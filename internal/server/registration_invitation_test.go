package server

import (
	"strings"
	"testing"
)

func TestRegistrationInvitationGuards(t *testing.T) {
	t.Setenv("SFD_REGISTRATION_INVITATIONS_ENABLED", "")
	if enabled, err := LoadRegistrationInvitationsConfig(false); err != nil || enabled {
		t.Fatal("default registration changed")
	}
	t.Setenv("SFD_REGISTRATION_INVITATIONS_ENABLED", "true")
	t.Setenv("SFD_PUBLIC_BASE_URL", "https://fixture.example.invalid")
	if enabled, err := LoadRegistrationInvitationsConfig(false); err == nil || enabled {
		t.Fatal("schema15 prerequisite bypassed")
	}
	if enabled, err := LoadRegistrationInvitationsConfig(true); err != nil || !enabled {
		t.Fatal("valid registration gate rejected")
	}
	t.Setenv("SFD_PUBLIC_BASE_URL", "http://fixture.example.invalid")
	if _, err := LoadRegistrationInvitationsConfig(true); err == nil {
		t.Fatal("insecure registration origin accepted")
	}
	t.Setenv("SFD_REGISTRATION_INVITATIONS_ENABLED", "yes")
	if _, err := LoadRegistrationInvitationsConfig(true); err == nil {
		t.Fatal("ambiguous activation accepted")
	}
	for _, invalid := range []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)} {
		if registrationTokenPattern.MatchString(invalid) {
			t.Fatal("invalid capability shape accepted")
		}
	}
	token := strings.Repeat("a", 64)
	if !registrationTokenPattern.MatchString(token) || registrationDigest(token) == token || len(registrationDigest(token)) != 64 {
		t.Fatal("invalid digest contract")
	}
	if registrationQuotaBytes != 10<<30 {
		t.Fatal("ordinary registration quota changed")
	}
}
