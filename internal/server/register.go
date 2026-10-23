// RegisterHandler handles POST /register to create new users and issue
// an email verification. Email must be verified before login succeeds.
// Password complexity is enforced; responses avoid leaking user existence.
// VerifyEmailHandler handles GET /verify?token=... and marks the user
// email as verified, enabling login. Tokens are single-use and time-bound.
// register.go - User registration and verification flow handlers.
//
// Handles account creation with email verification and basic validation.

package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// RegisterRequest represents the JSON payload for user registration
type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterResponse is the JSON response after successful registration
type RegisterResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// validateEmail checks if an email address is valid
func validateEmail(email string) bool {
	return emailRegex.MatchString(email)
}

// validatePassword checks password strength requirements
func validatePassword(password string) (bool, string) {
	if len(password) < 8 {
		return false, "Password must be at least 8 characters long"
	}
	if len(password) > 72 {
		return false, "Password must be at most 72 bytes"
	}
	// Check for at least one number
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	// Check for at least one letter
	hasLetter := regexp.MustCompile(`[a-zA-Z]`).MatchString(password)

	if !hasNumber || !hasLetter {
		return false, "Password must contain both letters and numbers"
	}

	return true, ""
}

// validateUsername checks username requirements
func validateUsername(username string) (bool, string) {
	if len(username) < 3 {
		return false, "Username must be at least 3 characters long"
	}
	if len(username) > 50 {
		return false, "Username must be less than 50 characters"
	}
	// Only allow alphanumeric and underscore
	validUsername := regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(username)
	if !validUsername {
		return false, "Username can only contain letters, numbers, and underscores"
	}
	return true, ""
}

// hashPassword generates a bcrypt hash of the password
func hashPassword(password string) (string, error) {
	// bcrypt cost of 12 is a good balance of security and performance
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// verifyPassword compares a password with its hash
func verifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// generateVerificationToken creates a random hex token for email verification
func generateVerificationToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// generateResetToken creates a random hex token for password reset
func generateResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// sendPasswordResetEmail sends an email with password reset link
func sendPasswordResetEmail(emailSvc *EmailService, baseURL, email, token string) error {
	if emailSvc == nil {
		// Fallback logging without exposing the token (security risk)
		log.Printf("PASSWORD RESET: Email service not configured, reset email for %s not sent", email)
		log.Printf("WARNING: Password reset requested but no email service available")
		return nil
	}
	return emailSvc.SendPasswordResetEmail(email, token, baseURL)
}

// RegisterHandler handles POST /register requests for user registration
func (cfg Config) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	// Sanitize inputs
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)

	// Validate email
	if !validateEmail(req.Email) {
		http.Error(w, "Invalid email address", http.StatusBadRequest)
		return
	}

	// Validate username
	if valid, msg := validateUsername(req.Username); !valid {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Validate password
	if valid, msg := validatePassword(req.Password); !valid {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	if r.Context().Err() != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if cfg.EmailSvc == nil || !cfg.EmailSvc.config.Enabled {
		http.Error(w, "registration unavailable: email delivery is not configured", http.StatusServiceUnavailable)
		return
	}

	// Check if user already exists
	var exists bool
	checkCtx, checkCancel := sqlContext(r.Context())
	err := cfg.DB.QueryRowContext(checkCtx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 OR username = $2)",
		req.Email, req.Username,
	).Scan(&exists)
	checkCancel()
	if err != nil {
		log.Printf("register: db check failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "Email or username already registered", http.StatusConflict)
		return
	}

	// Hash password
	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		log.Printf("register: hash failed: %v", err)
		http.Error(w, "Failed to process password", http.StatusInternalServerError)
		return
	}

	// Generate verification token for email verification
	verificationToken, err := generateVerificationToken()
	if err != nil {
		log.Printf("register: token generation failed: %v", err)
		http.Error(w, "Failed to generate verification token", http.StatusInternalServerError)
		return
	}

	// Hashing and validation do not consume the transaction budget. Keep SQL
	// operations short, but reserve delivery (15s) and commit headroom (5s).
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// Create user
	userID := uuid.New()
	tx, err := cfg.DB.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "registration unavailable", http.StatusServiceUnavailable)
		return
	}
	defer func() { _ = tx.Rollback() }()
	insertCtx, insertCancel := sqlContext(ctx)
	_, err = tx.ExecContext(insertCtx, `
		INSERT INTO users (id, email, username, password_hash, verification_token, verification_sent_at, email_verified, is_admin)
		VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE)
	`, userID, req.Email, req.Username, passwordHash, verificationToken, time.Now().UTC(), false)
	insertCancel()
	if err != nil {
		log.Printf("register: insert failed: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Never begin delivery without the full SMTP budget plus commit headroom.
	deadline, _ := ctx.Deadline()
	if ctx.Err() != nil || time.Until(deadline) < 20*time.Second {
		http.Error(w, "registration unavailable", http.StatusServiceUnavailable)
		return
	}
	// SMTP and SQL cannot form a distributed transaction. Roll back on delivery
	// failure; a post-delivery database failure is still reported as failure.
	// Do not commit an account whose verification message could not be delivered.
	if err := cfg.EmailSvc.sendVerificationEmailContext(ctx, req.Email, verificationToken, cfg.BaseURL); err != nil {
		Warn("registration delivery failed", nil)
		http.Error(w, "registration unavailable: email delivery failed", http.StatusServiceUnavailable)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "registration failed", http.StatusInternalServerError)
		return
	}

	log.Printf("register: created user - verification email sent")

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(RegisterResponse{
		ID:       userID.String(),
		Email:    req.Email,
		Username: req.Username,
	}); err != nil {
		Warn("response write failed", nil)
	}
}

// VerifyEmailHandler handles GET /verify?token={token} requests for email verification
func (cfg Config) VerifyEmailHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing verification token", http.StatusBadRequest)
		return
	}

	// Find user with this token
	var userID, email string
	var alreadyVerified bool
	err := cfg.DB.QueryRowContext(ctx, `
		SELECT id, email, email_verified
		FROM users
		WHERE verification_token = $1
	`, token).Scan(&userID, &email, &alreadyVerified)

	if err == sql.ErrNoRows {
		http.Error(w, "Invalid or expired verification token", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("verify: db query failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if alreadyVerified {
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte("<h2>Email already verified!</h2><p>You can now <a href='/'>log in</a>.</p>")); err != nil {
			Warn("verification response write failed", nil)
		}
		return
	}

	// Mark email as verified and clear token
	_, err = cfg.DB.ExecContext(ctx, `
		UPDATE users
		SET email_verified = true,
		    verification_token = NULL
		WHERE id = $1
	`, userID)

	if err != nil {
		log.Printf("verify: update failed: %v", err)
		http.Error(w, "Failed to verify email", http.StatusInternalServerError)
		return
	}

	log.Printf("verify: email verified for user %s", email)

	w.Header().Set("Content-Type", "text/html")
	if _, err := w.Write([]byte("<h2>Email verified successfully!</h2><p>You can now <a href='/'>log in</a>.</p>")); err != nil {
		Warn("response write failed", nil)
	}
}

// RequestPasswordResetHandler handles POST /reset-password-request for initiating password reset
func (cfg Config) RequestPasswordResetHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !validateEmail(req.Email) {
		http.Error(w, "Invalid email address", http.StatusBadRequest)
		return
	}

	// Check if user exists
	var userID string
	err := cfg.DB.QueryRowContext(ctx, "SELECT id FROM users WHERE email = $1 AND is_active = TRUE", req.Email).Scan(&userID)
	if err == sql.ErrNoRows {
		// Don't reveal if email exists or not for security
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "If the email exists, a reset link has been sent"}); err != nil {
			Warn("response write failed", nil)
		}
		return
	}
	if err != nil {
		log.Printf("reset-request: db query failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Generate reset token (expires in 1 hour)
	resetToken, err := generateResetToken()
	if err != nil {
		log.Printf("reset-request: token generation failed: %v", err)
		http.Error(w, "Failed to generate reset token", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().UTC().Add(1 * time.Hour)

	// Store reset token
	_, err = cfg.DB.ExecContext(ctx, `
		UPDATE users
		SET reset_token = $1,
		    reset_token_expires = $2
		WHERE id = $3
	`, resetToken, expiresAt, userID)

	if err != nil {
		log.Printf("reset-request: update failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Send reset email
	if err := sendPasswordResetEmail(cfg.EmailSvc, cfg.BaseURL, req.Email, resetToken); err != nil {
		log.Printf("reset-request: email send failed: %v", err)
	}

	log.Printf("reset-request: password reset requested for %s", req.Email)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "If the email exists, a reset link has been sent"}); err != nil {
		Warn("response write failed", nil)
	}
}

// ResetPasswordHandler handles POST /reset-password for completing password reset
func (cfg Config) ResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	req.Token = strings.TrimSpace(req.Token)

	if req.Token == "" {
		http.Error(w, "Missing reset token", http.StatusBadRequest)
		return
	}

	// Validate new password
	if valid, msg := validatePassword(req.NewPassword); !valid {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Find user with valid token
	var userID string
	var expiresAt time.Time
	err := cfg.DB.QueryRowContext(ctx, `
		SELECT id, reset_token_expires
		FROM users
		WHERE reset_token = $1 AND is_active = TRUE
	`, req.Token).Scan(&userID, &expiresAt)

	if err == sql.ErrNoRows {
		http.Error(w, "Invalid or expired reset token", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("reset-password: db query failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Check if token is expired
	if time.Now().UTC().After(expiresAt) {
		http.Error(w, "Reset token has expired", http.StatusGone)
		return
	}

	// Hash new password
	passwordHash, err := hashPassword(req.NewPassword)
	if err != nil {
		log.Printf("reset-password: hash failed: %v", err)
		http.Error(w, "Failed to process password", http.StatusInternalServerError)
		return
	}

	// Update password and clear reset token
	_, err = cfg.DB.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $1,
		    reset_token = NULL,
		    reset_token_expires = NULL
		WHERE id = $2
	`, passwordHash, userID)

	if err != nil {
		log.Printf("reset-password: update failed: %v", err)
		http.Error(w, "Failed to reset password", http.StatusInternalServerError)
		return
	}

	log.Printf("reset-password: password reset successful for user %s", userID)

	// Send password change notification
	var username, email string
	if err := cfg.DB.QueryRowContext(ctx, "SELECT username, email FROM users WHERE id = $1", userID).Scan(&username, &email); err != nil {
		Warn("password change notification identity unavailable", nil)
		return
	}
	if cfg.EmailSvc != nil && email != "" {
		NotifyPasswordChanged(cfg.EmailSvc, username, email, getClientIP(r))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Password reset successfully"}); err != nil {
		Warn("response write failed", nil)
	}
}

// authenticateUser checks credentials against the database
func authenticateUser(parent context.Context, db *sql.DB, username, password string) (string, bool, error) {
	ctx, cancel := sqlContext(parent)
	defer cancel()
	var userID string
	var passwordHash string
	var emailVerified bool

	log.Printf("auth: attempting login (username: %s)", username)

	err := db.QueryRowContext(ctx,
		"SELECT id, password_hash, email_verified FROM users WHERE (username = $1 OR email = $1) AND is_active = TRUE",
		username,
	).Scan(&userID, &passwordHash, &emailVerified)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("auth: user not found or not active")
			return "", false, nil
		}
		log.Printf("auth: db query failed")
		return "", false, err
	}

	log.Printf("auth: found user (id: %s, verified: %v)", userID, emailVerified)

	if !verifyPassword(password, passwordHash) {
		log.Printf("auth: password verification failed")
		return "", false, nil
	}

	log.Printf("auth: password verified")

	// Check if email is verified
	if !emailVerified {
		log.Printf("auth: login blocked - email not verified for user %s", userID)
		return "", false, nil
	}

	log.Printf("auth: login successful")

	// Update last login
	_, _ = db.ExecContext(ctx, "UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = $1", userID)

	return userID, true, nil
}

// passwordResetUnavailable does not issue tokens or email links to an unimplemented UI.
func passwordResetUnavailable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Error(w, "email password reset is not accepted in this baseline", http.StatusNotImplemented)
}
