package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestManagedShareTokenIdentityAndOptIn(t *testing.T) {
	t.Setenv("SFD_DOWNLOAD_SECRET", strings.Repeat("s", 64))
	t.Setenv("SFD_MANAGED_LINKS_ENABLED", "")
	if enabled, err := LoadManagedLinksConfig(false); err != nil || enabled {
		t.Errorf("managed links must be disabled on legacy deployments, got %v, %v", enabled, err)
	}
	t.Setenv("SFD_MANAGED_LINKS_ENABLED", "true")
	if enabled, err := LoadManagedLinksConfig(false); err == nil || enabled {
		t.Errorf("managed links must not silently enable on schema13, got %v, %v", enabled, err)
	}
	if enabled, err := LoadManagedLinksConfig(true); err != nil || !enabled {
		t.Errorf("qualified opt-in rejected: %v, %v", enabled, err)
	}
	t.Setenv("SFD_MANAGED_LINKS_ENABLED", "yes")
	if _, err := LoadManagedLinksConfig(true); err == nil {
		t.Error("invalid activation value accepted")
	}
	file := uuid.New().String()
	link := uuid.New().String()
	expires := time.Now().UTC().Add(time.Hour)
	legacy, err := signDownloadToken(file, expires)
	if err != nil {
		t.Fatal(err)
	}
	legacyClaims, err := verifyDownloadToken(legacy, time.Now())
	if err != nil || legacyClaims.LinkID != "" || legacyClaims.FileID != file {
		t.Errorf("legacy bearer compatibility violated: %+v, %v", legacyClaims, err)
	}
	managed, err := signManagedDownloadToken(file, link, expires)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifyDownloadToken(managed, time.Now())
	if err != nil || claims.LinkID != link || claims.FileID != file || claims.Exp != expires.Unix() {
		t.Errorf("managed token identity incorrect: %+v, %v", claims, err)
	}
	if _, err = verifyDownloadToken(managed, expires.Add(time.Second)); !errors.Is(err, errTokenExpired) {
		t.Errorf("expired managed bearer accepted: %v", err)
	}
	if _, err = verifyDownloadToken(managed+"x", time.Now()); !errors.Is(err, errBadToken) {
		t.Errorf("tampered bearer accepted: %v", err)
	}
	if _, err = signManagedDownloadToken(file, "not-uuid", expires); !errors.Is(err, errBadToken) {
		t.Errorf("malformed revocation identity accepted: %v", err)
	}
}
