package server

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
)

// EmailConfig holds configuration for sending emails via SMTP
type EmailConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	FromEmail    string
	Enabled      bool
}

// LoadEmailConfig reads email configuration from environment variables
func LoadEmailConfig() EmailConfig {
	enabled := os.Getenv("SFD_EMAIL_ENABLED") == "true"

	cfg := EmailConfig{
		SMTPHost:     os.Getenv("SFD_SMTP_HOST"),
		SMTPPort:     os.Getenv("SFD_SMTP_PORT"),
		SMTPUser:     os.Getenv("SFD_SMTP_USER"),
		SMTPPassword: os.Getenv("SFD_SMTP_PASSWORD"),
		FromEmail:    os.Getenv("SFD_FROM_EMAIL"),
		Enabled:      enabled,
	}

	// Set defaults if not provided
	if cfg.SMTPPort == "" {
		cfg.SMTPPort = "587"
	}
	if cfg.FromEmail == "" {
		cfg.FromEmail = cfg.SMTPUser
	}

	return cfg
}

// EmailService handles sending emails
type EmailService struct {
	config EmailConfig
}

// NewEmailService creates a new email service
func NewEmailService(cfg EmailConfig) *EmailService {
	return &EmailService{config: cfg}
}

// SendEmail sends an email with the given subject and body
func (s *EmailService) SendEmail(to, subject, body string) error {
	if !s.config.Enabled {
		// Email disabled, just log
		log.Printf("EMAIL (disabled): To: %s, Subject: %s", to, subject)
		return nil
	}

	// Validate configuration
	if s.config.SMTPHost == "" || s.config.SMTPUser == "" || s.config.SMTPPassword == "" {
		log.Printf("EMAIL ERROR: SMTP not configured properly")
		return fmt.Errorf("SMTP not configured")
	}

	// Build email message
	message := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/html; charset=UTF-8\r\n"+
			"\r\n"+
			"%s\r\n",
		s.config.FromEmail, to, subject, body,
	))

	// Setup authentication
	auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPassword, s.config.SMTPHost)

	// Send email
	addr := s.config.SMTPHost + ":" + s.config.SMTPPort
	err := smtp.SendMail(addr, auth, s.config.FromEmail, []string{to}, message)
	if err != nil {
		log.Printf("EMAIL ERROR: Failed to send to %s: %v", to, err)
		return err
	}

	log.Printf("EMAIL SENT: To: %s, Subject: %s", to, subject)
	return nil
}

// SendVerificationEmail sends email verification link
func (s *EmailService) SendVerificationEmail(to, token, baseURL string) error {
	verifyURL := fmt.Sprintf("%s/verify?token=%s", baseURL, token)

	subject := "Verify Your Email - Secure File Drop"
	body := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
			<div style="max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; border-radius: 10px;">
				<h2 style="color: #4F46E5;">Email Verification</h2>
				<p>Thank you for registering with Secure File Drop!</p>
				<p>Please verify your email address by clicking the link below:</p>
				<p style="margin: 30px 0;">
					<a href="%s" style="background: #4F46E5; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">
						Verify Email
					</a>
				</p>
				<p style="color: #666; font-size: 0.9em;">
					Or copy and paste this link into your browser:<br>
					<code style="background: #eee; padding: 4px 8px; border-radius: 4px;">%s</code>
				</p>
				<p style="color: #666; font-size: 0.85em; margin-top: 30px;">
					If you didn't request this verification, please ignore this email.
				</p>
			</div>
		</body>
		</html>
	`, verifyURL, verifyURL)

	return s.SendEmail(to, subject, body)
}

// SendPasswordResetEmail sends password reset link
func (s *EmailService) SendPasswordResetEmail(to, token, baseURL string) error {
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)

	subject := "Password Reset Request - Secure File Drop"
	body := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
			<div style="max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; border-radius: 10px;">
				<h2 style="color: #4F46E5;">Password Reset</h2>
				<p>We received a request to reset your password for Secure File Drop.</p>
				<p>Click the link below to reset your password:</p>
				<p style="margin: 30px 0;">
					<a href="%s" style="background: #4F46E5; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">
						Reset Password
					</a>
				</p>
				<p style="color: #666; font-size: 0.9em;">
					Or copy and paste this link into your browser:<br>
					<code style="background: #eee; padding: 4px 8px; border-radius: 4px;">%s</code>
				</p>
				<p style="color: #666; font-size: 0.85em; margin-top: 30px;">
					This link will expire in 1 hour.<br>
					If you didn't request a password reset, please ignore this email.
				</p>
			</div>
		</body>
		</html>
	`, resetURL, resetURL)

	return s.SendEmail(to, subject, body)
}

// SendFileUploadNotification sends notification when file upload completes
func (s *EmailService) SendFileUploadNotification(to, filename, fileID, baseURL string) error {
	subject := "File Upload Complete - Secure File Drop"
	body := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
			<div style="max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; border-radius: 10px;">
				<h2 style="color: #10B981;">✓ Upload Complete</h2>
				<p>Your file has been successfully uploaded and is ready for sharing!</p>
				<div style="background: white; padding: 15px; border-radius: 6px; margin: 20px 0;">
					<p style="margin: 0;"><strong>Filename:</strong> %s</p>
					<p style="margin: 5px 0 0 0;"><strong>File ID:</strong> <code>%s</code></p>
				</div>
				<p>You can now create download links for this file from your dashboard.</p>
				<p style="margin: 30px 0;">
					<a href="%s" style="background: #4F46E5; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">
						View Dashboard
					</a>
				</p>
			</div>
		</body>
		</html>
	`, filename, fileID, baseURL)

	return s.SendEmail(to, subject, body)
}

// SendFileDownloadNotification sends notification when file is downloaded
func (s *EmailService) SendFileDownloadNotification(to, filename, downloaderIP string) error {
	subject := "File Downloaded - Secure File Drop"
	body := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
			<div style="max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; border-radius: 10px;">
				<h2 style="color: #4F46E5;">📥 File Downloaded</h2>
				<p>Someone has downloaded your file:</p>
				<div style="background: white; padding: 15px; border-radius: 6px; margin: 20px 0;">
					<p style="margin: 0;"><strong>Filename:</strong> %s</p>
					<p style="margin: 5px 0 0 0;"><strong>Downloaded from IP:</strong> %s</p>
				</div>
				<p style="color: #666; font-size: 0.9em;">
					This is an automated notification to keep you informed of file access.
				</p>
			</div>
		</body>
		</html>
	`, filename, downloaderIP)

	return s.SendEmail(to, subject, body)
}

// SendFileExpirationNotification sends notification when file is about to expire
func (s *EmailService) SendFileExpirationNotification(to, filename, expiresIn string) error {
	subject := "File Expiring Soon - Secure File Drop"
	body := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
			<div style="max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; border-radius: 10px;">
				<h2 style="color: #F59E0B;">⚠️ File Expiring Soon</h2>
				<p>Your file will expire and be automatically deleted soon:</p>
				<div style="background: white; padding: 15px; border-radius: 6px; margin: 20px 0;">
					<p style="margin: 0;"><strong>Filename:</strong> %s</p>
					<p style="margin: 5px 0 0 0;"><strong>Expires in:</strong> %s</p>
				</div>
				<p>If you need to keep this file longer, you may want to download it before it expires.</p>
				<p style="color: #666; font-size: 0.85em; margin-top: 30px;">
					This is an automated reminder. The file will be permanently deleted after expiration.
				</p>
			</div>
		</body>
		</html>
	`, filename, expiresIn)

	return s.SendEmail(to, subject, body)
}

// SendFileDeletedNotification sends notification when file is deleted
func (s *EmailService) SendFileDeletedNotification(to, filename, reason string) error {
	subject := "File Deleted - Secure File Drop"

	reasonText := "manually deleted"
	if strings.Contains(reason, "expired") {
		reasonText = "automatically deleted due to expiration"
	}

	body := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
			<div style="max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; border-radius: 10px;">
				<h2 style="color: #EF4444;">🗑️ File Deleted</h2>
				<p>Your file has been %s:</p>
				<div style="background: white; padding: 15px; border-radius: 6px; margin: 20px 0;">
					<p style="margin: 0;"><strong>Filename:</strong> %s</p>
				</div>
				<p style="color: #666; font-size: 0.9em;">
					This is an automated notification to confirm the file removal.
				</p>
			</div>
		</body>
		</html>
	`, reasonText, filename)

	return s.SendEmail(to, subject, body)
}
