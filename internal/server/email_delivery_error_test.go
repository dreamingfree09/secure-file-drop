package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendRejectionCategoriesAndPublicRedaction(t *testing.T) {
	const private = "re_fixture_PRIVATE recipient@example.test https://drop.example.test/receive?id=private"
	restriction, _ := json.Marshal(map[string]string{"name": "validation_error", "message": "You can only send testing emails to your own email address (" + private + "). Verify a domain."})
	cases := []struct {
		name, body, from, category, publicCode string
		status                                 int
	}{
		{"test restriction", string(restriction), "onboarding@resend.dev", "test_recipient_restricted", "email_test_recipient_restricted", 403},
		{"unknown forbidden", `{"name":"validation_error","message":"` + private + `"}`, "onboarding@resend.dev", "forbidden", "invitation_delivery_failed", 403},
		{"different sender", string(restriction), "files@example.test", "forbidden", "invitation_delivery_failed", 403},
		{"different name", strings.Replace(string(restriction), "validation_error", "unexpected", 1), "onboarding@resend.dev", "forbidden", "invitation_delivery_failed", 403},
		{"malformed forbidden", "{" + private, "onboarding@resend.dev", "forbidden", "invitation_delivery_failed", 403},
		{"invalid key", `{"name":"invalid_api_key","message":"` + private + `"}`, "onboarding@resend.dev", "credentials", "email_configuration", 403},
		{"restricted key", `{"name":"restricted_api_key","message":"` + private + `"}`, "onboarding@resend.dev", "credentials", "email_configuration", 403},
		{"unauthorized", private, "onboarding@resend.dev", "credentials", "email_configuration", 401},
		{"rate limit", private, "onboarding@resend.dev", "rate_limited", "email_rate_limited", 429},
		{"service unavailable", private, "onboarding@resend.dev", "provider_unavailable", "email_temporarily_unavailable", 503},
		{"invalid request", private, "onboarding@resend.dev", "rejected", "invitation_delivery_failed", 422},
		{"redirect", private, "onboarding@resend.dev", "redirect_refused", "invitation_delivery_failed", 307},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer provider.Close()
			client := provider.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			svc := NewEmailService(EmailConfig{Provider: "resend", ResendAPIKey: "re_fixture_PRIVATE", FromEmail: tc.from, Enabled: true})
			err := svc.sendResendContext(context.Background(), "recipient@example.test", "Subject", "Private body", provider.URL, client)
			var failure *emailDeliveryError
			if !errors.As(err, &failure) || failure.category != tc.category || failure.status != tc.status || requests != 1 {
				t.Fatalf("incorrect safe category or request count: %v requests=%d", err, requests)
			}
			w := httptest.NewRecorder()
			writeInvitationDeliveryError(w, err)
			var response struct{ Error, Message string }
			if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error != tc.publicCode || response.Message == "" ||
				w.Code != 503 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("unsafe or incompatible public error contract")
			}
			for _, text := range []string{err.Error(), emailFailureDiagnostic(err), w.Body.String()} {
				for _, secret := range strings.Fields(private) {
					if strings.Contains(text, secret) {
						t.Fatal("provider detail escaped the fixed diagnostic/public boundary")
					}
				}
			}
		})
	}
}

type mailFailureTransport func(*http.Request) (*http.Response, error)

func (f mailFailureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResendTransportAndMalformedAcknowledgementsFailClosed(t *testing.T) {
	svc := NewEmailService(EmailConfig{Provider: "resend", ResendAPIKey: "re_fixture_PRIVATE", FromEmail: "sender@example.test", Enabled: true})
	for _, tc := range []struct {
		name, body, category string
		err                  error
	}{
		{"network", "", "network", errors.New("private transport details re_fixture_PRIVATE")},
		{"deadline", "", "timeout", context.DeadlineExceeded},
		{"missing id", `{}`, "invalid_response", nil},
		{"blank id", `{"id":"  "}`, "invalid_response", nil},
		{"malformed json", `{"id":`, "invalid_response", nil},
		{"oversized", strings.Repeat("x", 4097), "invalid_response", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			client := &http.Client{Transport: mailFailureTransport(func(*http.Request) (*http.Response, error) {
				requests++
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: 201, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			err := svc.sendResendContext(context.Background(), "recipient@example.test", "Subject", "Private", "https://api.resend.com/emails", client)
			var failure *emailDeliveryError
			if !errors.As(err, &failure) || failure.category != tc.category || requests != 1 || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(emailFailureDiagnostic(err), "PRIVATE") {
				t.Fatal("transport/malformed response did not fail closed privately without retries")
			}
		})
	}
	w := httptest.NewRecorder()
	writeInvitationDeliveryError(w, errors.New("private unclassified failure"))
	if strings.Contains(w.Body.String(), "private") || !strings.Contains(w.Body.String(), `"error":"invitation_delivery_failed"`) {
		t.Fatal("unknown error exposed internal detail")
	}
}
