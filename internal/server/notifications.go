// notifications.go - Security event email notifications
package server

import (
	"fmt"
	"time"
)

// SecurityEvent represents different types of security events
type SecurityEvent string

const (
	EventFailedLogin       SecurityEvent = "failed_login"
	EventAccountLocked     SecurityEvent = "account_locked"
	EventPasswordChanged   SecurityEvent = "password_changed"
	EventNewDeviceLogin    SecurityEvent = "new_device_login"
	EventUnusualFileAccess SecurityEvent = "unusual_file_access"
	EventMultipleFailures  SecurityEvent = "multiple_failures"
)

// SecurityNotification represents a security event notification
type SecurityNotification struct {
	Event     SecurityEvent
	UserEmail string
	Username  string
	IP        string
	Timestamp time.Time
	Details   map[string]string
}

// SendSecurityNotification sends security event notifications via email
func (s *EmailService) SendSecurityNotification(notification SecurityNotification) error {
	if s == nil {
		// Log but don't fail if email service unavailable
		Info("security_event", map[string]interface{}{
			"event":    notification.Event,
			"username": notification.Username,
			"ip":       notification.IP,
		})
		return nil
	}

	subject, body := formatSecurityNotification(notification)

	return s.SendEmail(notification.UserEmail, subject, body)
}

// formatSecurityNotification formats the email subject and body
func formatSecurityNotification(n SecurityNotification) (subject, body string) {
	timeStr := n.Timestamp.Format("2006-01-02 15:04:05 MST")

	switch n.Event {
	case EventFailedLogin:
		subject = "Security Alert: Failed Login Attempt"
		body = fmt.Sprintf(`
<h2>Failed Login Attempt Detected</h2>
<p>We detected a failed login attempt on your Secure File Drop account.</p>

<h3>Details:</h3>
<ul>
  <li><strong>Username:</strong> %s</li>
  <li><strong>Time:</strong> %s</li>
  <li><strong>IP Address:</strong> %s</li>
  <li><strong>Attempts Remaining:</strong> %s</li>
</ul>

<p><strong>What should I do?</strong></p>
<ul>
  <li>If this was you, you can safely ignore this message.</li>
  <li>If this wasn't you, consider changing your password immediately.</li>
  <li>Review your recent account activity for any suspicious actions.</li>
</ul>

<p>If you continue to see failed login attempts, your account will be temporarily locked for security.</p>
`, n.Username, timeStr, n.IP, n.Details["attempts_remaining"])

	case EventAccountLocked:
		subject = "Security Alert: Account Temporarily Locked"
		body = fmt.Sprintf(`
<h2>⚠️ Account Locked Due to Multiple Failed Login Attempts</h2>
<p>Your Secure File Drop account has been temporarily locked due to multiple failed login attempts.</p>

<h3>Details:</h3>
<ul>
  <li><strong>Username:</strong> %s</li>
  <li><strong>Time:</strong> %s</li>
  <li><strong>IP Address:</strong> %s</li>
  <li><strong>Locked Until:</strong> %s</li>
</ul>

<p><strong>What happened?</strong></p>
<p>After %s failed login attempts, we've temporarily locked your account to protect it from unauthorized access.</p>

<p><strong>What should I do?</strong></p>
<ul>
  <li>Wait until the lockout period expires (typically 15 minutes)</li>
  <li>If this wasn't you, change your password immediately after the lockout expires</li>
  <li>Consider enabling two-factor authentication (if available)</li>
  <li>Contact support if you believe your account has been compromised</li>
</ul>

<p>Your account will automatically unlock at the time shown above.</p>
`, n.Username, timeStr, n.IP, n.Details["locked_until"], n.Details["attempt_count"])

	case EventPasswordChanged:
		subject = "Security Alert: Password Changed"
		body = fmt.Sprintf(`
<h2>Password Changed Successfully</h2>
<p>The password for your Secure File Drop account was recently changed.</p>

<h3>Details:</h3>
<ul>
  <li><strong>Username:</strong> %s</li>
  <li><strong>Time:</strong> %s</li>
  <li><strong>IP Address:</strong> %s</li>
</ul>

<p><strong>If this was you:</strong></p>
<p>No action needed. Your password has been updated successfully.</p>

<p><strong>If this wasn't you:</strong></p>
<ul>
  <li>Your account may have been compromised</li>
  <li>Reset your password immediately using the "Forgot Password" link</li>
  <li>Review your account activity for any unauthorized changes</li>
  <li>Contact support immediately</li>
</ul>
`, n.Username, timeStr, n.IP)

	case EventMultipleFailures:
		subject = "Security Alert: Multiple Failed Login Attempts"
		body = fmt.Sprintf(`
<h2>⚠️ Multiple Failed Login Attempts Detected</h2>
<p>We detected multiple failed login attempts on your Secure File Drop account within a short time period.</p>

<h3>Details:</h3>
<ul>
  <li><strong>Username:</strong> %s</li>
  <li><strong>Time Period:</strong> %s</li>
  <li><strong>Failed Attempts:</strong> %s</li>
  <li><strong>IP Addresses:</strong> %s</li>
</ul>

<p><strong>What does this mean?</strong></p>
<p>This could indicate:</p>
<ul>
  <li>You forgot your password and tried multiple times</li>
  <li>Someone is attempting to access your account without authorization</li>
  <li>An automated attack targeting your account</li>
</ul>

<p><strong>What should I do?</strong></p>
<ul>
  <li>Change your password if you suspect unauthorized access</li>
  <li>Use a strong, unique password</li>
  <li>Monitor your account for suspicious activity</li>
</ul>

<p>After a few more failed attempts, your account will be temporarily locked for security.</p>
`, n.Username, timeStr, n.Details["attempt_count"], n.Details["ip_addresses"])

	default:
		subject = "Security Alert"
		body = fmt.Sprintf(`
<h2>Security Event Notification</h2>
<p>A security event was detected on your Secure File Drop account.</p>

<h3>Details:</h3>
<ul>
  <li><strong>Event:</strong> %s</li>
  <li><strong>Username:</strong> %s</li>
  <li><strong>Time:</strong> %s</li>
  <li><strong>IP Address:</strong> %s</li>
</ul>

<p>If you have any concerns about your account security, please contact support.</p>
`, n.Event, n.Username, timeStr, n.IP)
	}

	return subject, body
}

// NotifyFailedLogin sends notification after failed login attempts
func NotifyFailedLogin(emailSvc *EmailService, username, email, ip string, attemptsRemaining int) {
	if emailSvc == nil {
		return
	}

	// Only notify after 3 failed attempts
	if attemptsRemaining > 2 {
		return
	}

	notification := SecurityNotification{
		Event:     EventFailedLogin,
		UserEmail: email,
		Username:  username,
		IP:        ip,
		Timestamp: time.Now(),
		Details: map[string]string{
			"attempts_remaining": fmt.Sprintf("%d", attemptsRemaining),
		},
	}

	go emailSvc.SendSecurityNotification(notification)
}

// NotifyAccountLocked sends notification when account is locked
func NotifyAccountLocked(emailSvc *EmailService, username, email, ip string, lockedUntil time.Time, attemptCount int) {
	if emailSvc == nil {
		return
	}

	notification := SecurityNotification{
		Event:     EventAccountLocked,
		UserEmail: email,
		Username:  username,
		IP:        ip,
		Timestamp: time.Now(),
		Details: map[string]string{
			"locked_until":  lockedUntil.Format("2006-01-02 15:04:05 MST"),
			"attempt_count": fmt.Sprintf("%d", attemptCount),
		},
	}

	go emailSvc.SendSecurityNotification(notification)
}

// NotifyPasswordChanged sends notification when password is changed
func NotifyPasswordChanged(emailSvc *EmailService, username, email, ip string) {
	if emailSvc == nil {
		return
	}

	notification := SecurityNotification{
		Event:     EventPasswordChanged,
		UserEmail: email,
		Username:  username,
		IP:        ip,
		Timestamp: time.Now(),
		Details:   make(map[string]string),
	}

	go emailSvc.SendSecurityNotification(notification)
}
