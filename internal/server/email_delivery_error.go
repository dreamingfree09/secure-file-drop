package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// emailDeliveryError carries only fixed categories and an HTTP status. Never
// retain provider messages, addresses, request identifiers or transport errors.
type emailDeliveryError struct {
	category string
	status   int
}

func (e *emailDeliveryError) Error() string {
	return "email delivery failed (" + e.category + ")"
}

func mailDeliveryFailure(category string, status int) error {
	return &emailDeliveryError{category: category, status: status}
}

func classifyResendRejection(status int, data []byte, from string) error {
	if status == http.StatusTooManyRequests {
		return mailDeliveryFailure("rate_limited", status)
	}
	if status == http.StatusUnauthorized {
		return mailDeliveryFailure("credentials", status)
	}
	if status == http.StatusForbidden {
		var refusal struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &refusal) == nil {
			if refusal.Name == "invalid_api_key" || refusal.Name == "restricted_api_key" {
				return mailDeliveryFailure("credentials", status)
			}
			// Resend documents this exact prefix for its test-domain restriction.
			// A generic 403 must not be misdiagnosed as a rejected recipient.
			if strings.EqualFold(from, "onboarding@resend.dev") && refusal.Name == "validation_error" &&
				strings.HasPrefix(refusal.Message, "You can only send testing emails to your own email address (") {
				return mailDeliveryFailure("test_recipient_restricted", status)
			}
		}
		return mailDeliveryFailure("forbidden", status)
	}
	if status >= 500 {
		return mailDeliveryFailure("provider_unavailable", status)
	}
	if status >= 300 && status < 400 {
		return mailDeliveryFailure("redirect_refused", status)
	}
	return mailDeliveryFailure("rejected", status)
}

func writeInvitationDeliveryError(w http.ResponseWriter, err error) {
	code := "invitation_delivery_failed"
	message := "The email service did not confirm this invitation. Please contact the operator."
	var failure *emailDeliveryError
	if errors.As(err, &failure) {
		switch failure.category {
		case "test_recipient_restricted":
			code = "email_test_recipient_restricted"
			message = "This test sender can email only the inbox used to create the email-service account. Use that inbox, or ask the operator to verify a sending domain."
		case "credentials", "configuration":
			code = "email_configuration"
			message = "The email service needs attention from the operator. Your invitation was not confirmed."
		case "rate_limited":
			code = "email_rate_limited"
			message = "The email service has reached its sending limit. Please try again later."
		case "timeout", "network", "provider_unavailable":
			code = "email_temporarily_unavailable"
			message = "Email delivery could not be confirmed because the email service is temporarily unavailable. Please try again later."
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}{Error: code, Message: message})
}

// Keep the diagnostic representation limited to these trusted fields.
func emailFailureDiagnostic(err error) string {
	var failure *emailDeliveryError
	if !errors.As(err, &failure) {
		return "email_delivery_failed provider=resend category=unknown status=0"
	}
	return fmt.Sprintf("email_delivery_failed provider=resend category=%s status=%d", failure.category, failure.status)
}
