// Package server provides storage privacy checks for Railway's private-only S3 service.
// This explicit staging-only alternative verifies a real owned canary and
// its denial to an independent anonymous request. Unknown results fail closed.
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
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

// matchedSignedReadAndAnonymousDenied makes signed and unsigned GETs through
// one TLS transport and the same User-Agent. A generic 403 (e.g. from a WAF)
// is NOT privacy evidence: require a structured S3 AccessDenied response.
func matchedSignedReadAndAnonymousDenied(ctx context.Context, signedLocation, anonymousLocation string, expected []byte, transport http.RoundTripper) bool {
	signedURL, signedErr := url.Parse(signedLocation)
	anonymousURL, anonymousErr := url.Parse(anonymousLocation)
	if signedErr != nil || anonymousErr != nil ||
		signedURL.Scheme != "https" || anonymousURL.Scheme != "https" ||
		signedURL.User != nil || anonymousURL.User != nil ||
		signedURL.Host == "" || signedURL.Host != anonymousURL.Host ||
		signedURL.EscapedPath() != anonymousURL.EscapedPath() ||
		signedURL.RawQuery == "" || anonymousURL.RawQuery != "" {
		return false
	}
	client := &http.Client{
		Timeout:   6 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	read := func(location string) (*http.Response, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("User-Agent", "sfd-private-probe/1")
		request.Header.Set("Cache-Control", "no-store")
		request.Header.Set("Pragma", "no-cache")
		return client.Do(request)
	}
	signedResponse, err := read(signedLocation)
	if err != nil {
		return false
	}
	signedBytes, signedReadErr := io.ReadAll(io.LimitReader(signedResponse.Body, int64(len(expected)+1)))
	_ = signedResponse.Body.Close()
	if signedReadErr != nil || signedResponse.StatusCode != http.StatusOK ||
		signedResponse.Header.Get("Content-Encoding") != "" || !bytes.Equal(signedBytes, expected) {
		return false
	}
	anonymousResponse, err := read(anonymousLocation)
	if err != nil {
		return false
	}
	defer func() { _ = anonymousResponse.Body.Close() }()
	if anonymousResponse.StatusCode != http.StatusForbidden {
		return false
	}
	deniedBody, err := io.ReadAll(io.LimitReader(anonymousResponse.Body, 4097))
	if err != nil || len(deniedBody) > 4096 {
		return false
	}
	var denied struct {
		XMLName xml.Name `xml:"Error"`
		Code    string   `xml:"Code"`
	}
	return xml.Unmarshal(deniedBody, &denied) == nil && denied.Code == "AccessDenied"
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
	signedURL, err := client.PresignedGetObject(ctx, bucket, key, time.Minute, nil)
	if err != nil || !matchedSignedReadAndAnonymousDenied(ctx, signedURL.String(), objectURL, payload[:], nil) {
		return fmt.Errorf("railway privacy probe signed/anonymous control unverified (fail-closed)")
	}
	return nil
}
