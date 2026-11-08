package server

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type r2Service struct {
	db     *sql.DB
	store  r2ObjectStore
	config ResumableConfig
	slots  chan struct{}
	wake   chan struct{}
}

type r2Upload struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	Total       int64     `json:"size_bytes"`
	Offset      int64     `json:"offset"`
	State       string    `json:"state"`
	Expires     time.Time `json:"expires_at"`
	Digest      string    `json:"sha256,omitempty"`
	owner       string
	key         string
	provider    string
}

const r2Fields = "id, orig_name, content_type, total_bytes, offset_bytes, state, expires_at, COALESCE(digest,''), owner_id, object_key, COALESCE(upload_id,'')"
const r2ReservedStates = "('new','open','finalizing','cancelling','quarantined')"

type rowScanner interface{ Scan(...any) error }
type sqlRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func scanR2Upload(row rowScanner) (r2Upload, error) {
	var u r2Upload
	err := row.Scan(&u.ID, &u.Name, &u.ContentType, &u.Total, &u.Offset, &u.State, &u.Expires, &u.Digest,
		&u.owner, &u.key, &u.provider)
	return u, err
}

func r2Find(ctx context.Context, db sqlRowQueryer, id, owner string) (r2Upload, error) {
	return scanR2Upload(db.QueryRowContext(ctx, "SELECT "+r2Fields+" FROM r2_uploads WHERE id=$1 AND owner_id=$2", id, owner))
}

func r2Lock(ctx context.Context, tx *sql.Tx, id, owner string) (r2Upload, error) {
	return scanR2Upload(tx.QueryRowContext(ctx, "SELECT "+r2Fields+" FROM r2_uploads WHERE id=$1 AND owner_id=$2 FOR UPDATE", id, owner))
}

var (
	errR2Quota    = errors.New("upload exceeds available quota")
	errR2Conflict = errors.New("upload state, offset or checksum conflict")
	errR2Expired  = errors.New("upload session expired")
	errR2Capacity = errors.New("resumable storage reservation is full")
	errR2Rate     = errors.New("too many upload sessions created recently")
)

func readStorageUsage(ctx context.Context, db sqlRowQueryer, owner string, resumable bool) (int64, error) {
	query := "SELECT COALESCE(SUM(size_bytes),0) FROM files WHERE user_id=$1 AND status IN ('pending','stored','hashed','ready')"
	if resumable {
		query = "SELECT (" + query + ") + (SELECT COALESCE(SUM(total_bytes),0) FROM r2_uploads WHERE owner_id=$1 AND state IN " + r2ReservedStates + ")"
	}
	var used int64
	err := db.QueryRowContext(ctx, query, owner).Scan(&used)
	return used, err
}

func (s *r2Service) reserve(ctx context.Context, owner string, requestID uuid.UUID, name, contentType string, total int64) (r2Upload, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return r2Upload{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// Owner row serializes legacy and resumable quota reservations. The fixed
	// advisory transaction lock serializes the global resumable capacity check.
	var quota sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT storage_quota_bytes FROM users WHERE id=$1 AND is_active FOR UPDATE", owner).Scan(&quota); err != nil {
		return r2Upload{}, err
	}
	existing, err := scanR2Upload(tx.QueryRowContext(ctx, "SELECT "+r2Fields+" FROM r2_uploads WHERE owner_id=$1 AND request_id=$2", owner, requestID))
	if err == nil {
		if existing.Name != name || existing.ContentType != contentType || existing.Total != total {
			return r2Upload{}, errR2Conflict
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return r2Upload{}, err
	}
	var recent int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM r2_uploads WHERE owner_id=$1 AND created_at > now() - interval '1 hour'", owner).Scan(&recent); err != nil {
		return r2Upload{}, err
	}
	if recent >= 20 {
		return r2Upload{}, errR2Rate
	}
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(1839289126)"); err != nil {
		return r2Upload{}, err
	}
	used, err := readStorageUsage(ctx, tx, owner, true)
	if err != nil {
		return r2Upload{}, err
	}
	if quota.Valid && (used > quota.Int64 || total > quota.Int64-used) {
		return r2Upload{}, errR2Quota
	}
	var active, reserved int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FILTER (WHERE owner_id=$1), COALESCE(SUM(total_bytes),0) FROM r2_uploads WHERE state IN "+r2ReservedStates, owner).Scan(&active, &reserved); err != nil {
		return r2Upload{}, err
	}
	if active >= 2 || reserved > s.config.MaxReservedBytes || total > s.config.MaxReservedBytes-reserved {
		return r2Upload{}, errR2Capacity
	}
	id := uuid.New()
	u, err := scanR2Upload(tx.QueryRowContext(ctx, `INSERT INTO r2_uploads
		(id,owner_id,request_id,object_key,orig_name,content_type,total_bytes)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+r2Fields,
		id, owner, requestID, "resumable/"+id.String(), name, contentType, total))
	if err != nil {
		return r2Upload{}, err
	}
	return u, tx.Commit()
}

func (s *r2Service) recordPartIntent(ctx context.Context, id, owner, digest string, part r2PartPlan) (r2Upload, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return r2Upload{}, err
	}
	defer func() { _ = tx.Rollback() }()
	u, err := r2Lock(ctx, tx, id, owner)
	if err != nil {
		return r2Upload{}, err
	}
	if !time.Now().Before(u.Expires) && u.State == "open" {
		return r2Upload{}, errR2Expired
	}
	if part.Offset > u.Offset || (part.Offset == u.Offset && u.State != "open") ||
		(u.State != "open" && u.State != "finalizing" && u.State != "complete") {
		return r2Upload{}, errR2Conflict
	}
	if part.Offset == u.Offset {
		if _, err := tx.ExecContext(ctx, `INSERT INTO r2_parts (upload,number,start_byte,byte_count,digest)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT (upload,number) DO NOTHING`,
			id, part.Number, part.Offset, part.Length, digest); err != nil {
			return r2Upload{}, err
		}
	}
	var stored string
	if err := tx.QueryRowContext(ctx, "SELECT digest FROM r2_parts WHERE upload=$1 AND number=$2", id, part.Number).Scan(&stored); err != nil {
		return r2Upload{}, err
	}
	if stored != digest {
		return r2Upload{}, errR2Conflict
	}
	return u, tx.Commit()
}

func (s *r2Service) writePart(ctx context.Context, id, owner, digest string, part r2PartPlan, data []byte) (r2Upload, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return r2Upload{}, err
	}
	defer func() { _ = tx.Rollback() }()
	u, err := r2Lock(ctx, tx, id, owner)
	if err != nil {
		return r2Upload{}, err
	}
	if u.State == "open" && !time.Now().Before(u.Expires) {
		return r2Upload{}, errR2Expired
	}
	if part.Offset < u.Offset && (u.State == "open" || u.State == "finalizing" || u.State == "complete") {
		return u, nil
	}
	if u.State != "open" || u.Offset != part.Offset {
		return r2Upload{}, errR2Conflict
	}
	// The intent was committed before the provider request. Every retry that
	// reaches S3 for this part can only replay the same verified digest.
	stored, err := s.store.putPart(ctx, u.key, u.provider, part, data, digest)
	if err != nil {
		return r2Upload{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE r2_parts SET etag=$3 WHERE upload=$1 AND number=$2", id, part.Number, stored.ETag); err != nil {
		return r2Upload{}, err
	}
	u.Offset += part.Length
	if _, err := tx.ExecContext(ctx, "UPDATE r2_uploads SET offset_bytes=$2 WHERE id=$1", id, u.Offset); err != nil {
		return r2Upload{}, err
	}
	return u, tx.Commit()
}

func (s *r2Service) transition(ctx context.Context, id, owner, target string) (r2Upload, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return r2Upload{}, err
	}
	defer func() { _ = tx.Rollback() }()
	u, err := r2Lock(ctx, tx, id, owner)
	if err != nil {
		return r2Upload{}, err
	}
	switch target {
	case "finalizing":
		if u.State == target || u.State == "complete" {
			return u, nil
		}
		if u.State != "open" || u.Offset != u.Total {
			return r2Upload{}, errR2Conflict
		}
		if !time.Now().Before(u.Expires) {
			return r2Upload{}, errR2Expired
		}
	case "cancelling":
		if u.State == target || u.State == "cancelled" {
			return u, nil
		}
		if u.State != "new" && u.State != "open" {
			return r2Upload{}, errR2Conflict
		}
	default:
		return r2Upload{}, errR2Conflict
	}
	u.State = target
	if _, err := tx.ExecContext(ctx, "UPDATE r2_uploads SET state=$2, next_attempt=now() WHERE id=$1", id, target); err != nil {
		return r2Upload{}, err
	}
	return u, tx.Commit()
}

func (s *r2Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
