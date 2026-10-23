package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// WebhookEvent represents an event that can trigger a webhook
type WebhookEvent string

// Webhook events identify delivery categories.
const (
	WebhookEventFileUploaded   WebhookEvent = "file.uploaded"
	WebhookEventFileDownloaded WebhookEvent = "file.downloaded"
	WebhookEventFileDeleted    WebhookEvent = "file.deleted"
	WebhookEventLinkCreated    WebhookEvent = "link.created"
	WebhookEventLinkExpired    WebhookEvent = "link.expired"
	WebhookEventQuotaExceeded  WebhookEvent = "quota.exceeded"
	WebhookEventUserRegistered WebhookEvent = "user.registered"
)

// WebhookPayload represents the data sent to webhook endpoints
type WebhookPayload struct {
	Event     WebhookEvent           `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// WebhookConfig stores webhook configuration
type WebhookConfig struct {
	URL        string         `json:"url"`
	Events     []WebhookEvent `json:"events"`
	Secret     string         `json:"secret,omitempty"`
	Enabled    bool           `json:"enabled"`
	RetryCount int            `json:"retry_count"`
}

// TriggerWebhook sends a webhook for the given event
func (s *Server) TriggerWebhook(ctx context.Context, event WebhookEvent, data map[string]interface{}) error {
	// Get webhook configurations for this event
	webhooks, err := s.getWebhooksForEvent(ctx, event)
	if err != nil {
		return err
	}

	for _, webhook := range webhooks {
		if !webhook.Enabled {
			continue
		}

		payload := WebhookPayload{
			Event:     event,
			Timestamp: time.Now(),
			Data:      data,
		}

		// Send webhook asynchronously
		go s.sendWebhook(ctx, webhook, payload)
	}

	return nil
}

// sendWebhook sends a single webhook with retries
func (s *Server) sendWebhook(parent context.Context, config WebhookConfig, payload WebhookPayload) {
	maxRetries := config.RetryCount
	if maxRetries == 0 {
		maxRetries = 3
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("WEBHOOK ERROR: Failed to marshal webhook payload: %v", err)
		return
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt*attempt) * time.Second
			select {
			case <-parent.Done():
				return
			case <-time.After(backoff):
			}
		}

		ctx, cancel := context.WithTimeout(parent, 10*time.Second)

		req, err := http.NewRequestWithContext(ctx, "POST", config.URL, bytes.NewReader(payloadBytes))
		if err != nil {
			cancel()
			log.Print("WEBHOOK ERROR: failed to create request")
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "SecureFileDrop-Webhook/1.0")
		req.Header.Set("X-Webhook-Event", string(payload.Event))
		req.Header.Set("X-Webhook-Timestamp", payload.Timestamp.Format(time.RFC3339))

		// Add HMAC signature if secret is configured
		if config.Secret != "" {
			signature := s.generateWebhookSignature(payloadBytes, config.Secret)
			req.Header.Set("X-Webhook-Signature", signature)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			if parent.Err() != nil {
				return
			}
			log.Printf("WEBHOOK ERROR: request failed (attempt %d)", attempt+1)
			continue
		}

		_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		closeErr := resp.Body.Close()
		cancel()
		if readErr != nil || closeErr != nil {
			log.Print("WEBHOOK ERROR: response read failed")
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Print("WEBHOOK SENT")
			// Log webhook delivery
			s.logWebhookDelivery(parent, config.URL, payload, true, "")
			return
		}

		log.Printf("WEBHOOK ERROR: status %d (attempt %d)", resp.StatusCode, attempt+1)
	}

	// All retries failed
	s.logWebhookDelivery(parent, config.URL, payload, false, "max retries exceeded")
	log.Print("WEBHOOK ERROR: maximum retries reached")
}

// generateWebhookSignature creates an HMAC-SHA256 signature for the payload
// Returns signature in format: sha256=<hex-encoded-hmac>
func (s *Server) generateWebhookSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return fmt.Sprintf("sha256=%x", h.Sum(nil))
}

// getWebhooksForEvent retrieves webhook configurations for a specific event
func (s *Server) getWebhooksForEvent(_ context.Context, _ WebhookEvent) ([]WebhookConfig, error) {
	// Query database for webhooks configured for this event
	// This is a placeholder - you'd implement actual DB query
	return []WebhookConfig{}, nil
}

// logWebhookDelivery records webhook delivery attempts
func (s *Server) logWebhookDelivery(ctx context.Context, url string, payload WebhookPayload, success bool, errorMsg string) {
	query := `
		INSERT INTO webhook_deliveries (url, event, payload, success, error_message, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	payloadJSON, _ := json.Marshal(payload)

	_, err := s.db.ExecContext(ctx, query,
		url,
		payload.Event,
		payloadJSON,
		success,
		errorMsg,
		time.Now(),
	)

	if err != nil {
		log.Printf("WEBHOOK ERROR: Failed to log webhook delivery: %v", err)
	}
}
