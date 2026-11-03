package server

import (
	"crypto/hmac"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDeliveryCodeGenerationAndIndependentDigest(t *testing.T) {
	secret := []byte(strings.Repeat("S", 32))
	seen := make(map[string]bool)
	id1 := uuid.New()
	id2 := uuid.New()
	for i := 0; i < 200; i++ {
		code, err := generateDeliveryCode()
		if err != nil { t.Fatal(err) }
		normalised, ok := normaliseDeliveryCode(code)
		if !ok || len(code) != 19 || len(normalised) != 16 { t.Fatalf("invalid access code format: %q", code) }
		if seen[code] { t.Fatal("duplicate code from OS random source") }
		seen[code] = true
		d1 := deliveryCodeDigest(secret, id1, normalised)
		d2 := deliveryCodeDigest(secret, id2, normalised)
		if hmac.Equal(d1, d2) { t.Fatal("digest was not invitation-scoped") }
		if hmac.Equal(d1, deliveryCodeDigest(secret, id1, "AAAAAAAAAAAAAAAA")) && normalised != "AAAAAAAAAAAAAAAA" { t.Fatal("wrong code accepted") }
		decoded, err := hex.DecodeString(hex.EncodeToString(d1))
		if err != nil || !hmac.Equal(decoded, d1) { t.Fatal("digest not stable") }
	}
}

func TestDeliveryCodeAndDescriptionRejection(t *testing.T) {
	for _, code := range []string{"", "AAAAAAAAAAAAAAA", "AAAA-AAAA-AAAA-AAA!", "O000-OOOO-OOOO-OOOO", "AA", strings.Repeat("A", 100)} {
		if _, ok := normaliseDeliveryCode(code); ok { t.Fatalf("accepted malformed code: %q", code) }
	}
	if validDeliveryDescription(strings.Repeat("x", deliveryMaxDescriptionBytes + 1)) { t.Fatal("oversized description accepted") }
	if validDeliveryDescription("bad\rheader") { t.Fatal("CR control accepted") }
	if !validDeliveryDescription("Hello\nworld") { t.Fatal("legitimate description rejected") }
}

func TestDeliveryMailEscapesUntrustedTextAndOmitsCode(t *testing.T) {
	body := deliveryHTML("<sender@example.test>", "<script>file</script>", "hi <img src=x>", "https://drop.example.test/receive?id="+uuid.NewString(), "tomorrow")
	for _, bad := range []string{"<script>", "<img", "<sender@example.test>", "AAAA-BBBB-CCCC-DDDD"} {
		if strings.Contains(body, bad) { t.Fatalf("unsafe delivery mail HTML: %q", bad) }
	}
	if !strings.Contains(body, "&lt;script&gt;") { t.Fatal("filename was not escaped") }
}
