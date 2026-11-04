// Package server implements recipient invitations using an emailed locator and a separately delivered access code.
// The access code never enters email, URLs, logs or the database in plaintext.
package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

const (
	deliveryMaxAttempts         = 8
	deliveryMaxDownloads        = 3
	deliveryMaxDescriptionBytes = 1000
)

var deliveryCodePattern = regexp.MustCompile("^[A-Z2-7]{16}$")

type createInvitationRequest struct {
	FileID         string `json:"file_id"`
	RecipientEmail string `json:"recipient_email"`
	Description    string `json:"description"`
	TTLHours       int    `json:"ttl_hours"`
}

type invitationReceipt struct {
	RecipientEmail   string `json:"recipient_email"`
	AccessCode       string `json:"access_code"`
	ExpiresAt        string `json:"expires_at"`
	DownloadsAllowed int    `json:"downloads_allowed"`
}

func generateDeliveryCode() (string, error) {
	var random [10]byte // 80 bits; independent of the emailed UUID.
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random[:])
	return s[0:4] + "-" + s[4:8] + "-" + s[8:12] + "-" + s[12:16], nil
}

func normaliseDeliveryCode(input string) (string, bool) {
	if len(input) > 32 {
		return "", false
	}
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(input), "-", ""))
	return s, deliveryCodePattern.MatchString(s)
}

func deliveryCodeDigest(secret []byte, invitationID uuid.UUID, normalised string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("SFD-RECIPIENT-CODE-V1|"))
	_, _ = mac.Write([]byte(invitationID.String()))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(normalised))
	return mac.Sum(nil)
}

func deliverySecret() ([]byte, error) {
	secret, err := downloadSecret()
	if err != nil || len(secret) < 32 {
		return nil, errors.New("strong download secret required for recipient delivery")
	}
	return secret, nil
}

func validDeliveryDescription(description string) bool {
	if len(description) > deliveryMaxDescriptionBytes {
		return false
	}
	for _, ch := range description {
		if unicode.IsControl(ch) && ch != '\n' && ch != '\t' {
			return false
		}
	}
	return true
}

func deliveryHTML(sender, filename, description, link, expiry string) string {
	escape := html.EscapeString
	desc := strings.ReplaceAll(escape(description), "\n", "<br>")
	return fmt.Sprintf("<!doctype html><html><body><h2>Secure File Drop invitation</h2>"+
		"<p>%s has sent you a file: <strong>%s</strong>.</p>"+
		"<p>%s</p>"+
		"<p><a href=\"%s\">Open secure download</a></p>"+
		"<p>Ask the sender for the access code using a separate channel. "+
		"The code is never included in this email. This invitation expires %s (UTC) "+
		"and permits up to three downloads.</p></body></html>",
		escape(sender), escape(filename), desc, escape(link), escape(expiry))
}

func (s *EmailService) sendRecipientInvitation(ctx context.Context, to, sender, filename, description, link string, expires time.Time) error {
	if s == nil || !s.config.Enabled {
		return errors.New("SMTP delivery unavailable")
	}
	return s.sendEmailContext(ctx, to, "Secure File Drop: protected file invitation",
		deliveryHTML(sender, filename, description, link, expires.UTC().Format(time.RFC3339)))
}

// POST /invitations is available only to the verified owner of a fully hashed file.
// A successful 201 means the email provider accepted the message and the database committed the
// invitation. Only that one response contains the independently generated code.
func (cfg Config) createInvitationHandler(db *sql.DB) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID, err := cfg.Auth.getCurrentUser(r)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		var in createInvitationRequest
		if !decodeJSON(w, r, &in) {
			return
		}
		in.RecipientEmail = strings.ToLower(strings.TrimSpace(in.RecipientEmail))
		in.Description = strings.TrimSpace(in.Description)
		id, idErr := uuid.Parse(in.FileID)
		if idErr != nil || !validateEmail(in.RecipientEmail) || len(in.RecipientEmail) > 254 || !validDeliveryDescription(in.Description) || in.TTLHours < 0 || in.TTLHours > 72 {
			http.Error(w, "invalid invitation", http.StatusBadRequest)
			return
		}
		ttl := in.TTLHours
		if ttl == 0 {
			ttl = 24
		}
		origin, err := publicOrigin()
		if err != nil || !strings.HasPrefix(origin, "https://") {
			http.Error(w, "a configured HTTPS public origin is required", http.StatusServiceUnavailable)
			return
		}
		if cfg.EmailSvc == nil || !cfg.EmailSvc.config.Enabled {
			http.Error(w, "email delivery unavailable", http.StatusServiceUnavailable)
			return
		}
		secret, err := deliverySecret()
		if err != nil {
			http.Error(w, "server not configured for protected delivery", http.StatusServiceUnavailable)
			return
		}
		code, err := generateDeliveryCode()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		normalised, _ := normaliseDeliveryCode(code)
		invitationID := uuid.New()
		digest := hex.EncodeToString(deliveryCodeDigest(secret, invitationID, normalised))

		ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
		defer cancel()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		defer func() { _ = tx.Rollback() }()
		var filename, status, senderEmail string
		var fileExpiry sql.NullTime
		var autoDelete bool
		err = tx.QueryRowContext(ctx,
			"SELECT f.orig_name,f.status,f.expires_at,COALESCE(f.auto_delete,FALSE),u.email "+
				"FROM files f JOIN users u ON u.id=f.user_id "+
				"WHERE f.id=$1 AND f.user_id::text=$2 AND u.is_active=TRUE AND u.email_verified=TRUE FOR UPDATE OF f,u",
			id, userID).Scan(&filename, &status, &fileExpiry, &autoDelete, &senderEmail)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if status != "hashed" && status != "ready" {
			http.Error(w, "file not ready", http.StatusConflict)
			return
		}
		now := time.Now().UTC()
		expiresAt := now.Add(time.Duration(ttl) * time.Hour)
		if autoDelete && fileExpiry.Valid && fileExpiry.Time.Before(expiresAt) {
			expiresAt = fileExpiry.Time
		}
		if expiresAt.Sub(now) < time.Minute {
			http.Error(w, "file expiry is too near", http.StatusConflict)
			return
		}
		// The sender row is locked above, serializing invitations across
		// files and IP addresses. The independent ledger deliberately survives
		// file deletion, so revocation cannot reset a sender's email quota.
		// Only expired rate-limit metadata from this sender is pruned.
		_, err = tx.ExecContext(ctx,
			"DELETE FROM invitation_send_ledger WHERE sender_id::text=$1 AND created_at < CURRENT_TIMESTAMP - INTERVAL '7 days'", userID)
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		var recent int
		err = tx.QueryRowContext(ctx,
			"SELECT count(*) FROM invitation_send_ledger WHERE sender_id::text=$1 AND created_at > CURRENT_TIMESTAMP - INTERVAL '1 hour'", userID).Scan(&recent)
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if recent >= 12 {
			http.Error(w, "invitation limit reached", http.StatusTooManyRequests)
			return
		}
		_, err = tx.ExecContext(ctx,
			"INSERT INTO file_invitations (id,file_id,sender_id,recipient_email,description,code_digest,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)",
			invitationID, id, userID, in.RecipientEmail, in.Description, digest, expiresAt)
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		// One event for each committed email invitation, independent of the
		// file's lifetime. If delivery or COMMIT fails the transaction rolls back.
		_, err = tx.ExecContext(ctx,
			"INSERT INTO invitation_send_ledger (id,sender_id) VALUES ($1,$2)", invitationID, userID)
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		link := origin + "/receive?id=" + invitationID.String()
		if err = cfg.EmailSvc.sendRecipientInvitation(ctx, in.RecipientEmail, senderEmail, filename, in.Description, link, expiresAt); err != nil {
			writeInvitationDeliveryError(w, err)
			return
		}
		if err = tx.Commit(); err != nil {
			// The provider may have accepted the email, but no working invitation is
			// reported to the sender unless persistence also succeeds.
			http.Error(w, "invitation persistence failed", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(invitationReceipt{
			RecipientEmail:   in.RecipientEmail,
			AccessCode:       code,
			ExpiresAt:        expiresAt.Format(time.RFC3339),
			DownloadsAllowed: deliveryMaxDownloads,
		})
	}))
}

// GET /receive presents the code prompt, not an unrestricted download.
func receiveInvitationPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := uuid.Parse(r.URL.Query().Get("id")); err != nil {
		http.Error(w, "invalid invitation", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "/app/web/static/receive.html")
}

// POST form submission sends code in the request body, never in a URL.
// Failed attempts and download reservations are serialized in PostgreSQL.
func (cfg Config) recipientDownloadHandler(db *sql.DB, mc *minio.Client, bucket string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/x-www-form-urlencoded" {
			http.Error(w, "expected form submission", http.StatusUnsupportedMediaType)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if err := r.ParseForm(); err != nil || len(r.PostForm["id"]) != 1 || len(r.PostForm["code"]) != 1 || len(r.PostForm) != 2 {
			http.Error(w, "invalid submission", http.StatusBadRequest)
			return
		}
		id, err := uuid.Parse(r.PostForm.Get("id"))
		code, valid := normaliseDeliveryCode(r.PostForm.Get("code"))
		if err != nil || !valid {
			http.Error(w, "invalid submission", http.StatusBadRequest)
			return
		}
		secret, err := deliverySecret()
		if err != nil {
			http.Error(w, "server unavailable", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		defer func() { _ = tx.Rollback() }()
		var expectedHex, objectKey, filename, status string
		var attempts, downloads int
		var invitationExpires time.Time
		var size int64
		var fileExpires sql.NullTime
		var autoDelete bool
		err = tx.QueryRowContext(ctx,
			"SELECT i.code_digest,i.failed_attempts,i.downloads,i.expires_at,f.object_key,f.orig_name,f.status,f.size_bytes,f.expires_at,COALESCE(f.auto_delete,FALSE) "+
				"FROM file_invitations i JOIN files f ON f.id=i.file_id JOIN users u ON u.id=i.sender_id "+
				"WHERE i.id=$1 AND u.is_active=TRUE FOR UPDATE OF i",
			id).Scan(&expectedHex, &attempts, &downloads, &invitationExpires, &objectKey, &filename, &status, &size, &fileExpires, &autoDelete)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "invitation unavailable", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if !time.Now().UTC().Before(invitationExpires) || downloads >= deliveryMaxDownloads || attempts >= deliveryMaxAttempts || (autoDelete && fileExpires.Valid && !time.Now().Before(fileExpires.Time)) {
			http.Error(w, "invitation expired or unavailable", http.StatusGone)
			return
		}
		if status != "hashed" && status != "ready" {
			http.Error(w, "file unavailable", http.StatusGone)
			return
		}
		expected, err := hex.DecodeString(expectedHex)
		if err != nil || len(expected) != sha256.Size {
			http.Error(w, "invalid stored invitation", http.StatusServiceUnavailable)
			return
		}
		actual := deliveryCodeDigest(secret, id, code)
		if !hmac.Equal(actual, expected) {
			if _, err := tx.ExecContext(ctx, "UPDATE file_invitations SET failed_attempts=failed_attempts+1 WHERE id=$1", id); err != nil {
				http.Error(w, "database unavailable", http.StatusServiceUnavailable)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "database unavailable", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "incorrect access code", http.StatusUnauthorized)
			return
		}
		_, err = tx.ExecContext(ctx, "UPDATE file_invitations SET downloads=downloads+1 WHERE id=$1", id)
		if err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		// A reservation is consumed even if a connection is later interrupted;
		// three attempts allow bounded retry, never unlimited sharing.
		streamCtx, stop := context.WithTimeout(r.Context(), 30*time.Minute)
		defer stop()
		object, err := mc.GetObject(streamCtx, bucket, objectKey, minio.GetObjectOptions{})
		if err != nil {
			http.Error(w, "storage unavailable", http.StatusBadGateway)
			return
		}
		defer func() { _ = object.Close() }()
		info, err := object.Stat()
		if err != nil || info.Size != size || size < 0 {
			http.Error(w, "stored file unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": SanitizeFilename(filename)}))
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.WriteHeader(http.StatusOK)
		started := time.Now()
		n, copyErr := io.Copy(w, object)
		if copyErr != nil || n != size {
			Warn("recipient download interrupted", nil)
			return
		}
		GetMetrics().RecordDownload(n, time.Since(started))
	})
}
