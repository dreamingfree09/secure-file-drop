package server

import (
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
	if len(password) > 128 {
		return false, "Password must be less than 128 characters"
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

// sendVerificationEmail sends an email with verification link (stubbed for MVP)
func sendVerificationEmail(email, token string) error {
	// TODO: Implement actual email sending with SMTP
	// For MVP, just log the verification link
	log.Printf("EMAIL VERIFICATION: Send to %s - Token: %s", email, token)
	log.Printf("Verification URL: http://localhost:8080/verify?token=%s", token)
	return nil
}

// RegisterHandler handles POST /register requests for user registration
func (cfg Config) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Sanitize inputs
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

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

	// Check if user already exists
	var exists bool
	err := cfg.DB.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 OR username = $2)",
		req.Email, req.Username,
	).Scan(&exists)
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

	// Generate verification token
	verificationToken, err := generateVerificationToken()
	if err != nil {
		log.Printf("register: token generation failed: %v", err)
		http.Error(w, "Failed to generate verification token", http.StatusInternalServerError)
		return
	}

	// Create user with verification token
	userID := uuid.New()
	_, err = cfg.DB.Exec(`
		INSERT INTO users (id, email, username, password_hash, verification_token, verification_sent_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, req.Email, req.Username, passwordHash, verificationToken, time.Now().UTC())
	if err != nil {
		log.Printf("register: insert failed: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Send verification email
	if err := sendVerificationEmail(req.Email, verificationToken); err != nil {
		log.Printf("register: email send failed: %v", err)
		// Don't fail registration if email fails - user is already created
	}

	log.Printf("register: created user %s (%s) - verification email sent", req.Username, req.Email)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(RegisterResponse{
		ID:       userID.String(),
		Email:    req.Email,
		Username: req.Username,
	})
}

// VerifyEmailHandler handles GET /verify?token={token} requests for email verification
func (cfg Config) VerifyEmailHandler(w http.ResponseWriter, r *http.Request) {
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
	err := cfg.DB.QueryRow(`
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
		w.Write([]byte("<h2>Email already verified!</h2><p>You can now <a href='/'>log in</a>.</p>"))
		return
	}

	// Mark email as verified and clear token
	_, err = cfg.DB.Exec(`
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
	w.Write([]byte("<h2>Email verified successfully!</h2><p>You can now <a href='/'>log in</a>.</p>"))
}

// authenticateUser checks credentials against the database
func authenticateUser(db *sql.DB, username, password string) (string, bool) {
	var userID string
	var passwordHash string
	var emailVerified bool

	err := db.QueryRow(
		"SELECT id, password_hash, email_verified FROM users WHERE (username = $1 OR email = $1) AND is_active = TRUE",
		username,
	).Scan(&userID, &passwordHash, &emailVerified)

	if err != nil {
		if err == sql.ErrNoRows {
			return "", false
		}
		log.Printf("auth: db query failed: %v", err)
		return "", false
	}

	if !verifyPassword(password, passwordHash) {
		return "", false
	}

	// Check if email is verified
	if !emailVerified {
		log.Printf("auth: login blocked - email not verified for user %s", userID)
		return "", false
	}

	// Update last login
	_, _ = db.Exec("UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = $1", userID)

	return userID, true
}
