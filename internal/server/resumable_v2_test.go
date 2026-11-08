package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestResumableVerifiedStream(t *testing.T) {
	a, b := []byte("one part"), []byte("another part")
	makePart := func(p []byte) r2StoredPart {
		digest := sha256.Sum256(p)
		return r2StoredPart{length: int64(len(p)), digest: hex.EncodeToString(digest[:])}
	}
	parts := []r2StoredPart{makePart(a), makePart(b)}
	expected := string(a) + string(b)
	for _, tc := range []struct {
		data string
		err  error
	}{
		{expected, nil},
		{"bad part" + string(b), errR2Integrity},
		{expected + "extra", errR2Integrity},
		{expected[:len(expected)-1], io.ErrUnexpectedEOF},
	} {
		reader := &r2VerifiedStream{source: strings.NewReader(tc.data), parts: parts, hash: sha256.New()}
		actual, err := io.ReadAll(reader)
		if !errors.Is(err, tc.err) || (tc.err == nil && string(actual) != expected) {
			t.Errorf("stream verification got %v, want %v", err, tc.err)
		}
	}
}

func TestResumableInvalidPartCannotMutateS3(t *testing.T) {
	var calls atomic.Int32
	httpStorage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer httpStorage.Close()
	client, err := minio.New(strings.TrimPrefix(httpStorage.URL, "http://"), &minio.Options{
		Creds: credentials.NewStaticV4("fixture", strings.Repeat("x", 32), ""), Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := r2ObjectStore{client: client, bucket: "fixture"}
	plan := r2PartPlan{Number: 1, Length: 3, Last: true}
	if _, err := store.putPart(t.Context(), "resumable/owned", "fixture-id", plan, bytes.Repeat([]byte("a"), 3), strings.Repeat("0", 64)); err == nil {
		t.Fatal("bad digest passed")
	}
	if calls.Load() != 0 {
		t.Fatal("unverified bytes reached S3")
	}
}
