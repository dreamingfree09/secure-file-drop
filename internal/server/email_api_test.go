package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendHTTPSAcceptanceAndNoCodeLeak(t *testing.T) {
	secret := strings.Join([]string{"re", strings.Repeat("s", 32)}, "_")
	const to = "recipient@example.test"
	var observed bool
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = true
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			t.Error("unexpected API route")
		}
		if r.Header.Get("Authorization") != "Bearer "+secret ||
			r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing authenticated JSON request")
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			t.Fatal(err)
		}
		var sent struct {
			From    string   `json:"from"`
			To      []string `json:"to"`
			Subject string   `json:"subject"`
			HTML    string   `json:"html"`
		}
		if json.Unmarshal(body, &sent) != nil || sent.From != "sender@example.test" ||
			len(sent.To) != 1 || sent.To[0] != to || sent.Subject != "Protected delivery" ||
			sent.HTML != "<p>Private invitation without access code</p>" {
			t.Error("provider payload mismatch")
		}
		if strings.Contains(string(body), secret) {
			t.Error("API key leaked into JSON")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"synthetic-accepted-id"}`))
	}))
	defer testServer.Close()
	s := NewEmailService(EmailConfig{Provider: "resend", ResendAPIKey: secret, FromEmail: "sender@example.test", Enabled: true})
	if err := s.sendResendContext(context.Background(), to, "Protected delivery",
		"<p>Private invitation without access code</p>", testServer.URL+"/emails", testServer.Client()); err != nil {
		t.Fatal(err)
	}
	if !observed {
		t.Fatal("HTTPS API was never reached")
	}
}

func TestResendFailClosedAndNoTokenInError(t *testing.T) {
	secret := strings.Join([]string{"re", strings.Repeat("t", 32)}, "_")
	var sent int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sent++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(secret))
	}))
	defer server.Close()
	svc := NewEmailService(EmailConfig{Provider: "resend", ResendAPIKey: secret, FromEmail: "sender@example.test", Enabled: true})
	err := svc.sendResendContext(context.Background(), "recipient@example.test", "Subject", "<p>Body</p>", server.URL, server.Client())
	if err == nil || strings.Contains(err.Error(), secret) || sent != 1 {
		t.Fatal("provider refusal was not contained safely")
	}
	// Production's default Go TLS verifier must reject an unknown provider certificate.
	err = svc.sendResendContext(context.Background(), "recipient@example.test", "Subject", "<p>Body</p>", server.URL, http.DefaultClient)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("unexpected acceptance with untrusted TLS")
	}
}

func TestResendRedirectAndMissingAcknowledgementFailClosed(t *testing.T) {
	var redirected int
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { redirected++; w.WriteHeader(200) }))
	defer target.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	svc := NewEmailService(EmailConfig{Provider: "resend", ResendAPIKey: "re_fixture", FromEmail: "sender@example.test", Enabled: true})
	client := redirector.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	err := svc.sendResendContext(context.Background(), "recipient@example.test", "subject", "<p>hi</p>", redirector.URL, client)
	if err == nil || redirected != 0 {
		t.Fatal("provider redirect allowed")
	}
	noAck := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer noAck.Close()
	if svc.sendResendContext(context.Background(), "recipient@example.test", "subject", "<p>hi</p>", noAck.URL, noAck.Client()) == nil {
		t.Fatal("missing response id treated as delivered")
	}
}

func TestLoadEmailConfigSelectsHTTPSWithoutSMTPFallback(t *testing.T) {
	t.Setenv("SFD_EMAIL_PROVIDER", "resend")
	t.Setenv("SFD_RESEND_API_KEY", "re_synthetic")
	t.Setenv("SFD_EMAIL_FROM", "sender@example.test")
	t.Setenv("SFD_SMTP_HOST", "smtp.example.test")
	t.Setenv("SFD_SMTP_USER", "user")
	t.Setenv("SFD_SMTP_PASS", "synthetic")
	c := LoadEmailConfig()
	if !c.Enabled || c.Provider != "resend" || c.FromEmail != "sender@example.test" {
		t.Fatal("HTTPS provider selection failed")
	}
	t.Setenv("SFD_RESEND_API_KEY", "")
	if LoadEmailConfig().Enabled {
		t.Fatal("missing HTTPS key accepted using SMTP fallback")
	}
}
