// config_validation.go - Comprehensive configuration validation for Secure File Drop.
//
// Validates all environment variables and configuration settings at startup
// to fail fast with clear error messages rather than runtime failures.
package server

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ConfigValidationError represents a configuration validation error.
type ConfigValidationError struct {
	Field   string
	Message string
}

func (e ConfigValidationError) Error() string {
	return fmt.Sprintf("config validation failed for %s: %s", e.Field, e.Message)
}

// ConfigValidator validates application configuration.
type ConfigValidator struct {
	errors []ConfigValidationError
}

// NewConfigValidator creates a new configuration validator.
func NewConfigValidator() *ConfigValidator {
	return &ConfigValidator{
		errors: make([]ConfigValidationError, 0),
	}
}

// AddError adds a validation error.
func (v *ConfigValidator) AddError(field, message string) {
	v.errors = append(v.errors, ConfigValidationError{
		Field:   field,
		Message: message,
	})
}

// HasErrors returns true if there are validation errors.
func (v *ConfigValidator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all validation errors.
func (v *ConfigValidator) Errors() []ConfigValidationError {
	return v.errors
}

// ErrorString returns a formatted string of all errors.
func (v *ConfigValidator) ErrorString() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Configuration validation failed with %d error(s):\n", len(v.errors)))
	for i, err := range v.errors {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return sb.String()
}

// ValidateRequired validates that a required environment variable is set.
func (v *ConfigValidator) ValidateRequired(key string) string {
	value := os.Getenv(key)
	if value == "" {
		v.AddError(key, "required environment variable not set")
	}
	return value
}

// ValidateURL validates that a value is a valid URL.
func (v *ConfigValidator) ValidateURL(key, value string) {
	if value == "" {
		return // Skip validation if empty (check with ValidateRequired first)
	}

	parsed, err := url.Parse(value)
	if err != nil {
		v.AddError(key, fmt.Sprintf("invalid URL format: %v", err))
		return
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		v.AddError(key, "URL must use http or https scheme")
	}
}

// ValidatePort validates that a value is a valid port number.
func (v *ConfigValidator) ValidatePort(key, value string) {
	if value == "" {
		return
	}

	// Handle ":port" format
	portStr := strings.TrimPrefix(value, ":")

	port, err := strconv.Atoi(portStr)
	if err != nil {
		v.AddError(key, "port must be a number")
		return
	}

	if port < 1 || port > 65535 {
		v.AddError(key, "port must be between 1 and 65535")
	}
}

// ValidateMinLength validates minimum string length.
func (v *ConfigValidator) ValidateMinLength(key, value string, minLen int) {
	if value == "" {
		return
	}

	if len(value) < minLen {
		v.AddError(key, fmt.Sprintf("must be at least %d characters long (got %d)", minLen, len(value)))
	}
}

// ValidateEnum validates that a value is one of allowed options.
func (v *ConfigValidator) ValidateEnum(key, value string, allowed []string) {
	if value == "" {
		return
	}

	for _, opt := range allowed {
		if value == opt {
			return
		}
	}

	v.AddError(key, fmt.Sprintf("must be one of: %s (got: %s)", strings.Join(allowed, ", "), value))
}

// ValidatePositiveInt validates that a value is a positive integer.
func (v *ConfigValidator) ValidatePositiveInt(key, value string) {
	if value == "" {
		return
	}

	num, err := strconv.Atoi(value)
	if err != nil {
		v.AddError(key, "must be a valid integer")
		return
	}

	if num <= 0 {
		v.AddError(key, "must be a positive integer")
	}
}

// ValidateEmailAddress validates basic email format.
func (v *ConfigValidator) ValidateEmailAddress(key, value string) {
	if value == "" {
		return
	}

	if !strings.Contains(value, "@") || !strings.Contains(value, ".") {
		v.AddError(key, "must be a valid email address")
	}
}

// ValidateBcryptHash validates that a value looks like a bcrypt hash.
func (v *ConfigValidator) ValidateBcryptHash(key, value string) {
	if value == "" {
		return
	}

	if !strings.HasPrefix(value, "$2a$") &&
		!strings.HasPrefix(value, "$2b$") &&
		!strings.HasPrefix(value, "$2y$") {
		v.AddError(key, "must be a valid bcrypt hash (starts with $2a$, $2b$, or $2y$)")
	}

	// Bcrypt hashes are 60 characters
	if len(value) != 60 {
		v.AddError(key, "bcrypt hash must be exactly 60 characters")
	}
}

// ValidateAllConfiguration performs comprehensive validation of all configuration.
func ValidateAllConfiguration() error {
	v := NewConfigValidator()

	// Required core configuration
	v.ValidateRequired("DATABASE_URL")
	v.ValidateRequired("SFD_SESSION_SECRET")
	v.ValidateRequired("SFD_ADMIN_PASS")

	// Database configuration
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		// Basic PostgreSQL URL validation
		if !strings.HasPrefix(dbURL, "postgres://") && !strings.HasPrefix(dbURL, "postgresql://") {
			v.AddError("DATABASE_URL", "must be a valid PostgreSQL connection string")
		}
	}

	// Session secret validation
	sessionSecret := os.Getenv("SFD_SESSION_SECRET")
	v.ValidateMinLength("SFD_SESSION_SECRET", sessionSecret, 32)

	// Admin password must be bcrypt hash
	adminPass := os.Getenv("SFD_ADMIN_PASS")
	v.ValidateBcryptHash("SFD_ADMIN_PASS", adminPass)

	// Optional but validated if present
	if addr := os.Getenv("SFD_ADDR"); addr != "" {
		v.ValidatePort("SFD_ADDR", addr)
	}

	if baseURL := os.Getenv("SFD_BASE_URL"); baseURL != "" {
		v.ValidateURL("SFD_BASE_URL", baseURL)
	}

	if publicURL := os.Getenv("SFD_PUBLIC_BASE_URL"); publicURL != "" {
		v.ValidateURL("SFD_PUBLIC_BASE_URL", publicURL)
	}

	// MinIO/S3 configuration
	if endpoint := os.Getenv("MINIO_ENDPOINT"); endpoint != "" {
		// Can be host:port or URL
		if strings.Contains(endpoint, "://") {
			v.ValidateURL("MINIO_ENDPOINT", endpoint)
		}
	}

	// Email configuration
	if smtpPort := os.Getenv("SMTP_PORT"); smtpPort != "" {
		v.ValidatePositiveInt("SMTP_PORT", smtpPort)
	}

	if smtpFrom := os.Getenv("SMTP_FROM"); smtpFrom != "" {
		v.ValidateEmailAddress("SMTP_FROM", smtpFrom)
	}

	// Log configuration
	v.ValidateEnum("SFD_LOG_FORMAT", os.Getenv("SFD_LOG_FORMAT"), []string{"", "json", "text"})
	v.ValidateEnum("SFD_LOG_LEVEL", os.Getenv("SFD_LOG_LEVEL"), []string{"", "debug", "info", "warn", "error"})
	v.ValidateEnum("SFD_ENV", os.Getenv("SFD_ENV"), []string{"", "development", "production", "staging"})

	// Backup configuration
	if backupInterval := os.Getenv("SFD_BACKUP_INTERVAL"); backupInterval != "" {
		// Must be a valid duration string
		if _, err := strconv.Atoi(strings.TrimSuffix(backupInterval, "h")); err != nil {
			if _, err := strconv.Atoi(strings.TrimSuffix(backupInterval, "m")); err != nil {
				v.AddError("SFD_BACKUP_INTERVAL", "must be a valid duration (e.g., 24h, 1440m)")
			}
		}
	}

	if retentionDays := os.Getenv("SFD_BACKUP_RETENTION_DAYS"); retentionDays != "" {
		v.ValidatePositiveInt("SFD_BACKUP_RETENTION_DAYS", retentionDays)
	}

	// Upload limits
	if maxUpload := os.Getenv("SFD_MAX_UPLOAD_BYTES"); maxUpload != "" {
		v.ValidatePositiveInt("SFD_MAX_UPLOAD_BYTES", maxUpload)
	}

	// Return errors if any
	if v.HasErrors() {
		return fmt.Errorf("%s", v.ErrorString())
	}

	return nil
}

// WarnOnOptionalMissingConfig logs warnings for optional but recommended config.
func WarnOnOptionalMissingConfig() {
	warnings := make([]string, 0)

	if os.Getenv("SFD_BASE_URL") == "" {
		warnings = append(warnings, "SFD_BASE_URL not set - using default http://localhost:8080")
	}

	if os.Getenv("SMTP_HOST") == "" {
		warnings = append(warnings, "SMTP_HOST not set - email notifications disabled")
	}

	if os.Getenv("SFD_BACKUP_ENABLED") != "true" {
		warnings = append(warnings, "SFD_BACKUP_ENABLED not set to 'true' - automated backups disabled")
	}

	if os.Getenv("SFD_LOG_FORMAT") == "" {
		warnings = append(warnings, "SFD_LOG_FORMAT not set - using text format (consider 'json' for production)")
	}

	if len(warnings) > 0 {
		Info("configuration warnings", map[string]any{
			"count":    len(warnings),
			"warnings": warnings,
		})
	}
}
