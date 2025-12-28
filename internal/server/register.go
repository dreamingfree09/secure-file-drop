package server

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

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

	// Create user
	userID := uuid.New()
	_, err = cfg.DB.Exec(`
		INSERT INTO users (id, email, username, password_hash)
		VALUES ($1, $2, $3, $4)
	`, userID, req.Email, req.Username, passwordHash)
	if err != nil {
		log.Printf("register: insert failed: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	log.Printf("register: created user %s (%s)", req.Username, req.Email)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(RegisterResponse{
		ID:       userID.String(),
		Email:    req.Email,
		Username: req.Username,
	})
}

// authenticateUser checks credentials against the database
func authenticateUser(db *sql.DB, username, password string) (string, bool) {
	var userID string
	var passwordHash string

	err := db.QueryRow(
		"SELECT id, password_hash FROM users WHERE (username = $1 OR email = $1) AND is_active = TRUE",
		username,
	).Scan(&userID, &passwordHash)

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

	// Update last login
	_, _ = db.Exec("UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = $1", userID)

	return userID, true
}
