package server

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const registrationQuotaBytes = int64(10 << 30)

var registrationTokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func registrationDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func registrationHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func registrationError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func registrationOrigin(w http.ResponseWriter, r *http.Request) bool {
	expected, err := publicOrigin()
	actual, actualErr := canonicalOrigin(r.Header.Get("Origin"))
	if err != nil || actualErr != nil || !strings.HasPrefix(expected, "https://") || expected != actual {
		registrationError(w, http.StatusForbidden, "origin_required")
		return false
	}
	return true
}

func registrationJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	var body json.RawMessage
	if !decodeJSON(w, r, &body) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(dst) != nil {
		registrationError(w, http.StatusBadRequest, "invalid_registration_request")
		return false
	}
	return true
}

func (cfg Config) registrationAdministrator(w http.ResponseWriter, r *http.Request) (identity, bool) {
	principal, err := cfg.Auth.resolveIdentity(r)
	if err != nil {
		writeAuthError(w, err)
		return principal, false
	}
	if !cfg.RegistrationInvitations || !principal.EnvironmentAdmin {
		registrationError(w, http.StatusForbidden, "environment_administrator_required")
		return principal, false
	}
	return principal, true
}

func (cfg Config) registrationInvitationsHandler(w http.ResponseWriter, r *http.Request) {
	registrationHeaders(w)
	principal, ok := cfg.registrationAdministrator(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		cfg.listRegistrationInvitations(w, r, principal.Username)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		registrationError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if !registrationOrigin(w, r) {
		return
	}
	var input struct {
		TTLSeconds int `json:"ttl_seconds"`
	}
	if !registrationJSON(w, r, &input) {
		return
	}
	if input.TTLSeconds == 0 {
		input.TTLSeconds = 3600
	}
	if input.TTLSeconds < 60 || input.TTLSeconds > 86400 {
		registrationError(w, http.StatusBadRequest, "invalid_expiry")
		return
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		registrationError(w, http.StatusServiceUnavailable, "invitation_unavailable")
		return
	}
	token := hex.EncodeToString(secret[:])
	id := uuid.New()
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	tx, err := cfg.DB.BeginTx(ctx, nil)
	if err != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,16))", "registration-issuer:"+principal.Username); err != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	var recent, outstanding int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE created_at>now()-interval '1 hour'),
	 count(*) FILTER(WHERE expires_at>now() AND revoked_at IS NULL AND redeemed_at IS NULL)
	 FROM registration_invitations WHERE issuer=$1`, principal.Username).Scan(&recent, &outstanding)
	if err != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	if recent >= 20 || outstanding >= 100 {
		w.Header().Set("Retry-After", "3600")
		registrationError(w, http.StatusTooManyRequests, "invitation_limit")
		return
	}
	var expires time.Time
	err = tx.QueryRowContext(ctx, `INSERT INTO registration_invitations(id,token_sha256,issuer,expires_at)
	 VALUES($1,$2,$3,clock_timestamp()+make_interval(secs=>$4)) RETURNING expires_at`,
		id, registrationDigest(token), principal.Username, input.TTLSeconds).Scan(&expires)
	if err == nil {
		_, err = tx.ExecContext(ctx, "INSERT INTO registration_invitation_audit(invitation_id,event,issuer) VALUES($1,'issued',$2)", id, principal.Username)
	}
	if err != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	if tx.Commit() != nil {
		registrationError(w, 503, "outcome_unconfirmed")
		return
	}
	origin, _ := publicOrigin()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id.String(), "url": origin + "/join#token=" + token, "expires_at": expires})
}

type registrationInvitationPublic struct {
	ID         string     `json:"id"`
	State      string     `json:"state"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	RedeemedAt *time.Time `json:"redeemed_at,omitempty"`
}

func (cfg Config) listRegistrationInvitations(w http.ResponseWriter, r *http.Request, issuer string) {
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	query := `SELECT id::text,CASE WHEN redeemed_at IS NOT NULL THEN 'redeemed' WHEN revoked_at IS NOT NULL THEN 'revoked'
	 WHEN expires_at<=now() THEN 'expired' ELSE 'active' END,created_at,expires_at,revoked_at,redeemed_at
	 FROM registration_invitations WHERE issuer=$1`
	args := []any{issuer}
	if cursor := r.URL.Query().Get("before"); cursor != "" {
		id, err := uuid.Parse(cursor)
		if err != nil || id == uuid.Nil || id.String() != cursor {
			registrationError(w, 400, "invalid_cursor")
			return
		}
		query += ` AND (created_at,id)<(SELECT created_at,id FROM registration_invitations WHERE id=$2 AND issuer=$1)`
		args = append(args, id)
	}
	query += " ORDER BY created_at DESC,id DESC LIMIT 51"
	rows, err := cfg.DB.QueryContext(ctx, query, args...)
	if err != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	defer func() { _ = rows.Close() }()
	items := make([]registrationInvitationPublic, 0)
	for rows.Next() {
		var item registrationInvitationPublic
		var revoked, redeemed sql.NullTime
		if rows.Scan(&item.ID, &item.State, &item.CreatedAt, &item.ExpiresAt, &revoked, &redeemed) != nil {
			registrationError(w, 503, "invitation_unavailable")
			return
		}
		if revoked.Valid {
			item.RevokedAt = &revoked.Time
		}
		if redeemed.Valid {
			item.RedeemedAt = &redeemed.Time
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	if len(items) > 50 {
		items = items[:50]
		w.Header().Set("X-Next-Cursor", items[49].ID)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

func (cfg Config) revokeRegistrationInvitation(w http.ResponseWriter, r *http.Request) {
	registrationHeaders(w)
	principal, ok := cfg.registrationAdministrator(w, r)
	if !ok || !registrationOrigin(w, r) {
		return
	}
	idText := r.PathValue("id")
	id, err := uuid.Parse(idText)
	if err != nil || id == uuid.Nil || id.String() != idText {
		registrationError(w, 404, "invitation_not_found")
		return
	}
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	tx, err := cfg.DB.BeginTx(ctx, nil)
	if err != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	var used bool
	err = tx.QueryRowContext(ctx, "SELECT redeemed_at IS NOT NULL FROM registration_invitations WHERE id=$1 AND issuer=$2 FOR UPDATE", id, principal.Username).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		registrationError(w, 404, "invitation_not_found")
		return
	}
	if err != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	if used {
		registrationError(w, 409, "invitation_already_used")
		return
	}
	_, err = tx.ExecContext(ctx, "UPDATE registration_invitations SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE id=$1", id)
	if err == nil {
		_, err = tx.ExecContext(ctx, "INSERT INTO registration_invitation_audit(invitation_id,event,issuer) VALUES($1,'revoked',$2) ON CONFLICT DO NOTHING", id, principal.Username)
	}
	if err != nil {
		registrationError(w, 503, "invitation_unavailable")
		return
	}
	if tx.Commit() != nil {
		registrationError(w, 503, "outcome_unconfirmed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RedeemRegistrationInvitation creates exactly one ordinary account and consumes
// its authority in the same transaction. It never creates an authenticated session.
func (cfg Config) RedeemRegistrationInvitation(w http.ResponseWriter, r *http.Request) {
	registrationHeaders(w)
	if !cfg.RegistrationInvitations {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		registrationError(w, 405, "method_not_allowed")
		return
	}
	if !registrationOrigin(w, r) {
		return
	}
	var input struct {
		Token    string `json:"token"`
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !registrationJSON(w, r, &input) {
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	usernameOK, _ := validateUsername(input.Username)
	passwordOK, _ := validatePassword(input.Password)
	if !registrationTokenPattern.MatchString(input.Token) {
		registrationError(w, 410, "invitation_unavailable")
		return
	}
	if !usernameOK || !passwordOK || !validateEmail(input.Email) || len(input.Email) > 254 ||
		strings.EqualFold(input.Username, cfg.Auth.AdminUser) || strings.EqualFold(input.Email, cfg.Auth.AdminUser) {
		registrationError(w, 400, "invalid_credentials")
		return
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		registrationError(w, 503, "registration_unavailable")
		return
	}
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	tx, err := cfg.DB.BeginTx(ctx, nil)
	if err != nil {
		registrationError(w, 503, "registration_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	var invitationID, issuer string
	err = tx.QueryRowContext(ctx, `SELECT id::text,issuer FROM registration_invitations WHERE token_sha256=$1
	 AND revoked_at IS NULL AND redeemed_at IS NULL AND expires_at>clock_timestamp() FOR UPDATE`, registrationDigest(input.Token)).Scan(&invitationID, &issuer)
	if errors.Is(err, sql.ErrNoRows) {
		registrationError(w, 410, "invitation_unavailable")
		return
	}
	if err != nil {
		registrationError(w, 503, "registration_unavailable")
		return
	}
	accountID := uuid.New()
	_, err = tx.ExecContext(ctx, `INSERT INTO users(id,email,username,password_hash,email_verified,is_admin,is_active,storage_quota_bytes,registration_method,registration_invitation_id)
	 VALUES($1,$2,$3,$4,FALSE,FALSE,TRUE,$5,'administrator_invitation',$6)`, accountID, input.Email, input.Username, hash, registrationQuotaBytes, invitationID)
	if err != nil {
		var constraint *pq.Error
		if errors.As(err, &constraint) && constraint.Code == "23505" {
			registrationError(w, 409, "registration_conflict")
			return
		}
		registrationError(w, 503, "registration_unavailable")
		return
	}
	var consumed string
	err = tx.QueryRowContext(ctx, `UPDATE registration_invitations SET redeemed_at=clock_timestamp(),redeemed_by=$2
	 WHERE id=$1 AND revoked_at IS NULL AND redeemed_at IS NULL AND expires_at>clock_timestamp() RETURNING id::text`, invitationID, accountID).Scan(&consumed)
	if errors.Is(err, sql.ErrNoRows) {
		registrationError(w, 410, "invitation_unavailable")
		return
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, "INSERT INTO registration_invitation_audit(invitation_id,event,issuer,account_id) VALUES($1,'redeemed',$2,$3)", invitationID, issuer, accountID)
	}
	if err != nil {
		registrationError(w, 503, "registration_unavailable")
		return
	}
	// Unknown COMMIT is not proof of rollback. Preserve its possible account and
	// consumed invitation; no compensation, retry, session or false success.
	if tx.Commit() != nil {
		registrationError(w, 503, "outcome_unconfirmed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "registered", "username": input.Username, "registration_method": "administrator_invitation", "email_verified": false})
}

func registrationPage(w http.ResponseWriter, r *http.Request) {
	registrationHeaders(w)
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || r.URL.RawQuery != "" {
		http.Error(w, "invalid registration page request", 400)
		return
	}
	http.ServeFile(w, r, "/app/web/static/register-invitation.html")
}
