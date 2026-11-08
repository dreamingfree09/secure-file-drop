package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"math"
	"time"

	"github.com/minio/minio-go/v7"
)

type r2StoredPart struct {
	receipt minio.CompletePart
	length  int64
	digest  string
}

// The bytes reaching the native hash also cross the durable per-part
// verification boundary. An ETag or an object size can never publish a file.
type r2VerifiedStream struct {
	source    io.Reader
	parts     []r2StoredPart
	hash      hash.Hash
	remaining int64
}

func (v *r2VerifiedStream) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if len(v.parts) == 0 {
		n, err := v.source.Read(buffer[:1])
		if n != 0 {
			return 0, errR2Integrity
		}
		return 0, err
	}
	if v.remaining == 0 {
		v.remaining = v.parts[0].length
		v.hash.Reset()
	}
	n, err := io.ReadFull(v.source, buffer[:min(int64(len(buffer)), v.remaining)])
	if err != nil {
		return n, io.ErrUnexpectedEOF
	}
	_, _ = v.hash.Write(buffer[:n])
	v.remaining -= int64(n)
	if v.remaining == 0 {
		var digest [sha256.Size]byte
		copy(digest[:], v.hash.Sum(nil))
		if r2ValidatePartDigest(v.parts[0].digest, digest) != nil {
			return n, errR2Integrity
		}
		v.parts = v.parts[1:]
	}
	return n, nil
}

func (s *r2Service) finish(ctx context.Context, tx *sql.Tx, u r2Upload) error {
	rows, err := tx.QueryContext(ctx, `SELECT number,start_byte,byte_count,digest,etag FROM r2_parts
		WHERE upload=$1 ORDER BY number`, u.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var parts []r2StoredPart
	var receipts []minio.CompletePart
	for rows.Next() {
		var part r2StoredPart
		var start int64
		if err := rows.Scan(&part.receipt.PartNumber, &start, &part.length, &part.digest, &part.receipt.ETag); err != nil {
			return err
		}
		plan, err := r2PlanPart(u.Total, start, part.length, r2MaximumBytes)
		if err != nil || plan.Number != len(parts)+1 || plan.Number != part.receipt.PartNumber {
			return errR2Integrity
		}
		parts = append(parts, part)
		receipts = append(receipts, part.receipt)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	info, err := s.store.complete(ctx, u.key, u.provider, u.Total, r2MaximumBytes, receipts)
	if err != nil {
		return err
	}
	body, err := s.store.read(ctx, u.key, info)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	hasher := &r2VerifiedStream{source: body, parts: parts, hash: sha256.New()}
	result, err := hashStream(ctx, hasher)
	if err != nil {
		return err
	}
	if result.Bytes > math.MaxInt64 || int64(result.Bytes) != u.Total || len(hasher.parts) != 0 {
		return errR2Integrity
	}
	sha, err := hex.DecodeString(result.Hash)
	if err != nil {
		return err
	}
	layout, err := hashMetadataLayout(ctx, tx)
	if err != nil {
		return err
	}
	var value any = u.Total
	if layout == "bytea" {
		value = sha
	}
	// Insert and reservation release share one transaction. On an uncertain
	// commit, a retry either sees complete or re-verifies the owned S3 object.
	if _, err := tx.ExecContext(ctx, `INSERT INTO files
		(id,object_key,orig_name,content_type,size_bytes,user_id,status,sha256_hex,sha256_bytes)
		VALUES ($1,$2,$3,$4,$5,$6,'hashed',$7,$8)`,
		u.ID, u.key, u.Name, u.ContentType, u.Total, u.owner, result.Hash, value); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE r2_uploads SET state='complete', digest=$2 WHERE id=$1", u.ID, result.Hash)
	return err
}

func (s *r2Service) cancelStoredParts(ctx context.Context, u r2Upload) error {
	_, err := s.store.client.StatObject(ctx, s.store.bucket, u.key, minio.StatObjectOptions{})
	if !r2ObjectMissing(err) {
		return errors.New("owned key is not confirmed absent")
	}
	ids, err := s.store.uploads(ctx, u.key)
	if err != nil {
		return err
	}
	if u.provider != "" {
		ids = append(ids, u.provider)
	}
	for _, id := range ids {
		if err := s.store.abort(ctx, u.key, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *r2Service) process(ctx context.Context) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	u, err := scanR2Upload(tx.QueryRowContext(ctx, "SELECT "+r2Fields+` FROM r2_uploads
		WHERE next_attempt <= now() AND (state IN ('new','finalizing','cancelling') OR (state='open' AND expires_at<=now()))
		ORDER BY next_attempt, id LIMIT 1 FOR UPDATE SKIP LOCKED`))
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if (u.State == "new" || u.State == "open") && !time.Now().Before(u.Expires) {
		// Record cancellation intent independently of any provider operation.
		_, err = tx.ExecContext(ctx, "UPDATE r2_uploads SET state='cancelling' WHERE id=$1", u.ID)
	} else {
		err = s.processState(ctx, tx, u)
	}
	if err != nil {
		// Storage failures retain the reservation. Integrity failure quarantines
		// the exact object without any deletion or downloadable file row.
		if u.State == "finalizing" && errors.Is(err, errR2Integrity) {
			_, err = tx.ExecContext(ctx, "UPDATE r2_uploads SET state='quarantined' WHERE id=$1", u.ID)
		} else {
			_, err = tx.ExecContext(ctx, "UPDATE r2_uploads SET next_attempt=now()+interval '30 seconds' WHERE id=$1", u.ID)
		}
	}
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *r2Service) processState(ctx context.Context, tx *sql.Tx, u r2Upload) error {
	if u.State == "finalizing" {
		return s.finish(ctx, tx, u)
	}
	storeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if u.State == "new" {
		id, err := s.store.begin(storeCtx, u.key)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE r2_uploads SET state='open',upload_id=$2 WHERE id=$1", u.ID, id)
		return err
	}
	if err := s.cancelStoredParts(storeCtx, u); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE r2_uploads SET state='cancelled' WHERE id=$1", u.ID)
	return err
}

func (s *r2Service) work(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for ctx.Err() == nil {
		op, cancel := context.WithTimeout(ctx, 8*time.Hour)
		again, err := s.process(op)
		cancel()
		if err != nil && ctx.Err() == nil {
			Warn("resumable worker will retry", nil)
		}
		if again && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
		case <-s.wake:
		case <-tick.C:
		}
	}
}
