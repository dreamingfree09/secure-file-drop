// Package server provides storage privacy checks for Railway's private-only S3 service.
// This explicit staging-only alternative verifies a real owned canary and
// its denial to an independent anonymous request. Unknown results fail closed.
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/minio/minio-go/v7"
)

var railwayProbeBucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

func validateRailwayProbeTarget(endpoint string, secure bool, lookup minio.BucketLookupType, bucket string) error {
	if os.Getenv("SFD_ENV") != "staging" || endpoint != "t3.storageapi.dev" || !secure || lookup != minio.BucketLookupDNS ||
		!railwayProbeBucketName.MatchString(bucket) {
		return fmt.Errorf("railway privacy probe target invalid (fail-closed)")
	}
	return nil
}

// anonymousObjectReadDenied performs an unauthenticated GET, disallows redirects,
// and accepts only an explicit HTTP 403. This request has no S3 credentials,
// signed query, cookies or authorization header.
func anonymousObjectReadDenied(ctx context.Context, location string, transport http.RoundTripper) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil || request.URL.Scheme != "https" || request.URL.User != nil || request.URL.RawQuery != "" {
		return false
	}
	client := &http.Client{
		Timeout:   6 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode == http.StatusForbidden
}

// verifyRailwayPrivateBucket creates and removes only its own random canary.
// A failed operation cannot authorize startup, and cleanup failure always wins.
func verifyRailwayPrivateBucket(ctx context.Context, client *minio.Client, bucket, endpoint string) (result error) {
	var keyRandom [16]byte
	var payload [32]byte
	if _, err := rand.Read(keyRandom[:]); err != nil {
		return fmt.Errorf("railway privacy random key failed (fail-closed)")
	}
	if _, err := rand.Read(payload[:]); err != nil {
		return fmt.Errorf("railway privacy random payload failed (fail-closed)")
	}
	key := "sfd-privacy-" + hex.EncodeToString(keyRandom[:])
	// Always attempt to remove this exact canary even after ambiguous writes.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := client.RemoveObject(cleanupCtx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
			result = fmt.Errorf("railway privacy probe cleanup failed (fail-closed)")
			return
		}
		_, err := client.StatObject(cleanupCtx, bucket, key, minio.StatObjectOptions{})
		if err == nil || minio.ToErrorResponse(err).Code != "NoSuchKey" {
			result = fmt.Errorf("railway privacy probe deletion unverified (fail-closed)")
		}
	}()

	_, err := client.PutObject(ctx, bucket, key, bytes.NewReader(payload[:]), int64(len(payload)), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return fmt.Errorf("railway privacy probe authenticated write failed (fail-closed)")
	}
	object, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("railway privacy probe authenticated read failed (fail-closed)")
	}
	defer func() { _ = object.Close() }()
	metadata, err := object.Stat()
	if err != nil || metadata.Size != int64(len(payload)) {
		return fmt.Errorf("railway privacy probe signed object verification failed (fail-closed)")
	}
	actual, err := io.ReadAll(io.LimitReader(object, int64(len(payload)+1)))
	if err != nil || !bytes.Equal(actual, payload[:]) {
		return fmt.Errorf("railway privacy probe signed bytes mismatch (fail-closed)")
	}
	objectURL := (&url.URL{Scheme: "https", Host: bucket + "." + endpoint, Path: "/" + key}).String()
	if !anonymousObjectReadDenied(ctx, objectURL, nil) {
		return fmt.Errorf("railway privacy probe anonymous denial unverified (fail-closed)")
	}
	return nil
}
