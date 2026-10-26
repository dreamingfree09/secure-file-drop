// Command sfd-acceptance-account is a local operator tool, not an HTTP service.
// Secrets arrive only on bounded stdin. It never inserts users or verifies email.
// Live use requires exact-revision independent review and fresh adoption gates.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const workID = "SFD-OWNER-TEST-ACCOUNT-20261026"
const quotaBytes = int64(1048576)

var accountName = regexp.MustCompile(`^sfd_acceptance_20261026(_[a-f0-9]{8})?$`)

type input struct {
	Mode                string
	DSN                 string
	AdminHash           string
	AdminPassword       string
	AdminName           string
	Session             string
	BackendOrigin       string
	ID                  string
	Username            string
	Email               string
	ExistingUsersSHA256 string
}
type result struct {
	OK                  bool   `json:"ok"`
	Stage               string `json:"stage"`
	ExistingUsersSHA256 string `json:"existing_users_sha256,omitempty"`
	ExistingUsers       int    `json:"existing_users,omitempty"`
	ID                  string `json:"id,omitempty"`
	Quota               int64  `json:"quota_bytes,omitempty"`
}

func usersFingerprint(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, except string) (string, int, error) {
	rows, err := q.QueryContext(ctx, `SELECT to_jsonb(u)::text FROM users u WHERE id::text<>$1 ORDER BY id`, except)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = rows.Close() }()
	h := sha256.New()
	count := 0
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			return "", 0, err
		}
		_, _ = fmt.Fprintln(h, s)
		count++
	}
	return hex.EncodeToString(h.Sum(nil)), count, rows.Err()
}
func authenticate(ctx context.Context, in input) error {
	// Reuse the actual adopted server's identity resolution; do not reimplement tokens.
	// This is the already existing private backend/DB network, not a public endpoint.
	if in.BackendOrigin != "http://backend:8080" || in.AdminName == "" || in.Session == "" || len(in.Session) > 4096 || strings.ContainsAny(in.Session, "\r\n;") {
		return errors.New("identity guard")
	}
	u, _ := url.Parse(in.BackendOrigin + "/me")
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}
	req.AddCookie(&http.Cookie{Name: "sfd_session", Value: in.Session, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return errors.New("administrator required")
	}
	var me struct {
		Status   string `json:"status"`
		Username string `json:"username"`
		Admin    bool   `json:"is_admin"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 16384))
	if dec.Decode(&me) != nil || me.Status != "ok" || me.Username != in.AdminName || !me.Admin {
		return errors.New("administrator required")
	}
	return nil
}
func run(ctx context.Context, in input) (result, error) { return runWithAuth(ctx, in, authenticate) }
func runWithAuth(ctx context.Context, in input, verify func(context.Context, input) error) (result, error) {
	out := result{Stage: in.Mode}
	if in.Mode == "check-admin-password" {
		if len(in.AdminPassword) == 0 || len(in.AdminPassword) > 72 {
			return out, errors.New("input guard")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(in.AdminHash), []byte(in.AdminPassword)); err != nil {
			return out, errors.New("credential mismatch")
		}
		out.OK = true
		return out, nil
	}
	if in.Mode != "snapshot" && in.Mode != "cap-new-account" {
		return out, errors.New("mode guard")
	}
	db, err := sql.Open("postgres", in.DSN)
	if err != nil {
		return out, err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	if in.Mode == "snapshot" {
		tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if e != nil {
			return out, e
		}
		defer func() { _ = tx.Rollback() }()
		if _, e = tx.ExecContext(ctx, "SET LOCAL TIME ZONE 'UTC'"); e != nil {
			return out, e
		}
		out.ExistingUsersSHA256, out.ExistingUsers, e = usersFingerprint(ctx, tx, "")
		if e != nil {
			return out, e
		}
		e = tx.Commit()
		out.OK = e == nil
		return out, e
	}
	if !accountName.MatchString(in.Username) || len(in.ExistingUsersSHA256) != 64 || in.Email == "" || in.ID == "" {
		return out, errors.New("account scope")
	}
	if err = verify(ctx, in); err != nil {
		return out, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='5s'; SET LOCAL TIME ZONE 'UTC'`); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(73520261026)`); err != nil {
		return out, err
	}
	var version int
	var dirty bool
	if err = tx.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty); err != nil || version != 12 || dirty {
		return out, errors.New("schema guard")
	}
	var prior int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE metadata->>'work_id'=$1`, workID).Scan(&prior); err != nil || prior != 0 {
		return out, errors.New("one-account guard")
	}
	var username, email, hash string
	var active, verified, admin bool
	var token sql.NullString
	var quota sql.NullInt64
	var created time.Time
	err = tx.QueryRowContext(ctx, `SELECT username,email,password_hash,is_active,email_verified,is_admin,verification_token,storage_quota_bytes,created_at FROM users WHERE id=$1 FOR UPDATE`, in.ID).Scan(&username, &email, &hash, &active, &verified, &admin, &token, &quota, &created)
	if err != nil {
		return out, errors.New("new-account guard")
	}
	cost, e := bcrypt.Cost([]byte(hash))
	age := time.Since(created.UTC())
	if username != in.Username || email != in.Email || !active || !verified || admin || token.Valid || quota.Valid || e != nil || cost != 12 || age < -time.Minute || age > 15*time.Minute {
		return out, errors.New("new-account guard")
	}
	fp, _, err := usersFingerprint(ctx, tx, in.ID)
	if err != nil || fp != in.ExistingUsersSHA256 {
		return out, errors.New("existing-account preservation")
	}
	var owned int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE user_id=$1", in.ID).Scan(&owned); err != nil || owned != 0 {
		return out, errors.New("unused-account guard")
	}
	changed, err := tx.ExecContext(ctx, `UPDATE users SET storage_quota_bytes=$1 WHERE id=$2 AND username=$3 AND email=$4 AND is_admin=false AND is_active=true AND email_verified=true AND verification_token IS NULL AND storage_quota_bytes IS NULL`, quotaBytes, in.ID, in.Username, in.Email)
	if err != nil {
		return out, err
	}
	n, err := changed.RowsAffected()
	if err != nil || n != 1 {
		return out, errors.New("one-row guard")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(timestamp,event_type,action,username,ip_address,resource_id,metadata,success) VALUES(CURRENT_TIMESTAMP,'admin_action','admin_action',$1,'local-operator',$2,jsonb_build_object('work_id',$3::text,'operation','cap-new-SMTP-verified-acceptance-account','quota_bytes',$4::bigint,'email_verification','normal application endpoint'),true)`, in.AdminName, in.ID, workID, quotaBytes)
	if err != nil {
		return out, err
	}
	after, _, err := usersFingerprint(ctx, tx, in.ID)
	if err != nil || after != fp {
		return out, errors.New("existing-account preservation")
	}
	// Recheck the live revocable administrator session immediately before COMMIT.
	if err = verify(ctx, in); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, errors.New("commit outcome requires read-only reconciliation")
	}
	out.OK = true
	out.ID = in.ID
	out.Quota = quotaBytes
	return out, nil
}
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	in := input{}
	dec := json.NewDecoder(io.LimitReader(os.Stdin, 32769))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil {
		fmt.Println(`{"ok":false,"stage":"bounded stdin"}`)
		os.Exit(1)
	}
	var tail any
	if dec.Decode(&tail) != io.EOF {
		fmt.Println(`{"ok":false,"stage":"bounded stdin"}`)
		os.Exit(1)
	}
	out, err := run(ctx, in)
	in.AdminPassword = ""
	in.Session = ""
	in.DSN = ""
	in.AdminHash = ""
	// Never return database errors, SQL arguments, passwords, hashes or cookies.
	if err != nil {
		out.OK = false
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
	if !out.OK {
		os.Exit(1)
	}
}
