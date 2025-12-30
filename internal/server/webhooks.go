package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// WebhookEvent represents an event that can trigger a webhook
type WebhookEvent string

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
		go s.sendWebhook(webhook, payload)
	}

	return nil
}

// sendWebhook sends a single webhook with retries
func (s *Server) sendWebhook(config WebhookConfig, payload WebhookPayload) {
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
			time.Sleep(backoff)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "POST", config.URL, bytes.NewReader(payloadBytes))
		if err != nil {
			log.Printf("WEBHOOK ERROR: Failed to create webhook request: %v", err)
			continue
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
			log.Printf("WEBHOOK ERROR: Request failed to %s (attempt %d): %v", config.URL, attempt+1, err)
			continue
		}

		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("WEBHOOK SENT: To %s, Event: %s", config.URL, payload.Event)
			// Log webhook delivery
			s.logWebhookDelivery(context.Background(), config.URL, payload, true, "")
			return
		}

		log.Printf("WEBHOOK ERROR: %s returned status %d (attempt %d): %s",
			config.URL, resp.StatusCode, attempt+1, string(body))
	}

	// All retries failed
	s.logWebhookDelivery(context.Background(), config.URL, payload, false, "max retries exceeded")
	log.Printf("WEBHOOK ERROR: Failed after max retries to %s for event %s", config.URL, payload.Event)
}

// generateWebhookSignature creates an HMAC signature for the payload
func (s *Server) generateWebhookSignature(payload []byte, secret string) string {
	// Use the same HMAC logic as signed download links
	// This would use crypto/hmac with SHA256
	return fmt.Sprintf("sha256=%x", payload) // Placeholder
}

// getWebhooksForEvent retrieves webhook configurations for a specific event
func (s *Server) getWebhooksForEvent(ctx context.Context, event WebhookEvent) ([]WebhookConfig, error) {
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
