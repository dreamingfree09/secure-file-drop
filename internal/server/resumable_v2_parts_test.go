package server

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// One bounded test event deliberately keeps the accepted portable runner's
// event accounting explicit. All cases execute even if an earlier one fails.
func TestRorschachMultipartPartGeometryAndIntegrity(t *testing.T) {
	const target = 100 << 30
	if r2ChunkBytes != 32<<20 || r2MaximumBytes != target || r2MaximumParts != 10000 {
		t.Error("public 100 GiB/32 MiB/S3 part ceiling has drifted")
	}
	if n, err := r2PartCount(target, target); err != nil || n != 3200 {
		t.Errorf("100 GiB must require exactly 3200 parts, got %d, %v", n, err)
	}
	if n, err := r2PartCount(1, 1); err != nil || n != 1 {
		t.Errorf("one-byte last S3 part must be legal, got %d, %v", n, err)
	}
	for _, v := range []int64{-1, 0, target + 1} {
		if _, err := r2PartCount(v, target); err == nil {
			t.Errorf("forbidden object size %d accepted", v)
		}
	}
	for _, tc := range []struct {
		total, offset, length, cap int64
		number                     int
		last                       bool
		ok                         bool
	}{
		{target, 0, r2ChunkBytes, target, 1, false, true},
		{target, target - r2ChunkBytes, r2ChunkBytes, target, 3200, true, true},
		{r2ChunkBytes + 1, r2ChunkBytes, 1, target, 2, true, true},
		{1, 0, 1, target, 1, true, true},
		{target + 1, 0, r2ChunkBytes, target + 1, 0, false, false},
		{target, 1, r2ChunkBytes, target, 0, false, false},
		{target, -r2ChunkBytes, r2ChunkBytes, target, 0, false, false},
		{target, target, r2ChunkBytes, target, 0, false, false},
		{target, 0, 1, target, 0, false, false},
		{target, 0, r2ChunkBytes + 1, target, 0, false, false},
		{target, 0, r2ChunkBytes, 10 << 20, 0, false, false},
	} {
		got, err := r2PlanPart(tc.total, tc.offset, tc.length, tc.cap)
		if (err == nil) != tc.ok {
			t.Errorf("part total=%d offset=%d length=%d cap=%d: %v", tc.total, tc.offset, tc.length, tc.cap, err)
		} else if tc.ok && (got.Number != tc.number || got.Last != tc.last || got.Length != tc.length || got.Offset != tc.offset) {
			t.Errorf("incorrect part geometry: %+v", got)
		}
	}
	whole := []byte("synthetic-part")
	digest := sha256.Sum256(whole)
	if err := r2ValidatePartDigest(hex.EncodeToString(digest[:]), digest); err != nil {
		t.Errorf("correct checksum rejected: %v", err)
	}
	for _, value := range []string{"", "BAD", "ABCD" + hex.EncodeToString(digest[:])[4:], hex.EncodeToString(digest[:])[:63]} {
		if err := r2ValidatePartDigest(value, digest); err == nil {
			t.Errorf("incorrect checksum variant accepted: %q", value[:min(8, len(value))])
		}
	}
	partNumbers := make([]int, 3200)
	etags := make([]string, 3200)
	for i := range partNumbers {
		partNumbers[i], etags[i] = i+1, "private-s3-etag"
	}
	if err := r2ValidateCompletion(target, target, partNumbers, etags); err != nil {
		t.Errorf("complete 3200-part ledger rejected: %v", err)
	}
	partNumbers[3199] = 3199
	if err := r2ValidateCompletion(target, target, partNumbers, etags); err == nil {
		t.Error("duplicate/omitted multipart number accepted")
	}
	partNumbers[3199] = 3200
	etags[3] = "opaque-etag\ninjection"
	if err := r2ValidateCompletion(target, target, partNumbers, etags); err == nil {
		t.Error("untrusted storage ETag accepted")
	}
	etags[3] = "private-s3-etag"
	if err := r2ValidateCompletion(target, target, partNumbers[:3199], etags[:3199]); err == nil {
		t.Error("incomplete multipart ledger accepted")
	}
	store := r2ObjectStore{}
	if _, err := store.begin(t.Context(), "key"); err == nil {
		t.Error("unconfigured private S3 multipart initiation must fail closed")
	}
	if _, err := store.putPart(t.Context(), "key", "upload-id", r2PartPlan{}, nil, ""); err == nil {
		t.Error("unconfigured private S3 part upload must fail closed")
	}
	if _, err := store.complete(t.Context(), "key", "upload-id", 1, 1, nil); err == nil {
		t.Error("unconfigured private S3 completion must fail closed")
	}
	if err := store.abort(t.Context(), "key", "upload-id"); err == nil {
		t.Error("unconfigured private S3 abort must fail closed")
	}
}
