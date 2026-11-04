package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"strconv"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func railwayTestScope(t *testing.T) {
	t.Helper()
	t.Setenv("SFD_ENV", "staging")
	t.Setenv("RAILWAY_PROJECT_NAME", "secure-file-drop-staging")
	t.Setenv("RAILWAY_ENVIRONMENT_NAME", "staging")
}

func TestRailwayStoragePrivacyScope(t *testing.T) {
	railwayTestScope(t)
	const bucket = "sfd-private-staging-fixture"
	if err := validateRailwayProbeScope("t3.storageapi.dev", true, minio.BucketLookupDNS, bucket); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []struct{ endpoint string; secure bool; lookup minio.BucketLookupType; bucket string }{
		{"t3.storageapi.dev", false, minio.BucketLookupDNS, bucket},
		{"untrusted.example.test", true, minio.BucketLookupDNS, bucket},
		{"t3.storageapi.dev", true, minio.BucketLookupPath, bucket},
		{"t3.storageapi.dev", true, minio.BucketLookupDNS, "another-bucket"},
		{"t3.storageapi.dev", true, minio.BucketLookupDNS, "sfd-private-staging-invalid.bucket"},
	} {
		if validateRailwayProbeScope(wrong.endpoint, wrong.secure, wrong.lookup, wrong.bucket) == nil {
			t.Fatal("unsafe staging scope accepted")
		}
	}
	t.Setenv("RAILWAY_ENVIRONMENT_NAME", "production")
	if validateRailwayProbeScope("t3.storageapi.dev", true, minio.BucketLookupDNS, bucket) == nil {
		t.Fatal("production environment accepted")
	}
	if _, err := storagePrivacyMode("skip"); err == nil {
		t.Fatal("unknown privacy mode accepted")
	}
	if got, err := storagePrivacyMode(""); err != nil || got != "policy" {
		t.Fatal("default privacy enforcement changed")
	}
}

// Exercise the actual MinIO SDK requests and an entirely unsigned HTTP GET.
func TestRailwayStoragePrivacyAttestation(t *testing.T) {
	railwayTestScope(t)
	const bucket = "sfd-private-staging-fixture"
	const denial = "<Error><Code>AccessDenied</Code></Error>"
	modes := []struct {
		name string
		anonStatus int
		anonBody string
		publicPolicy bool
		badSignedRead bool
		deleteFailure bool
		wantSuccess bool
	}{
		{"private", 403, denial, false, false, false, true},
		{"public-200", 200, "public", false, false, false, false},
		{"redirect", 302, "", false, false, false, false},
		{"opaque-403", 403, "blocked", false, false, false, false},
		{"missing-404", 404, denial, false, false, false, false},
		{"explicit-public-policy", 403, denial, true, false, false, false},
		{"signed-byte-mismatch", 403, denial, false, true, false, false},
		{"cleanup-failure", 403, denial, false, false, true, false},
	}
	for _, tc := range modes {
		var stored []byte
		var objectPath string
		var uploaded, signedRead, anonRead, deletes int
		var signedGetRange string
		transport := policyFixtureTransport(func(req *http.Request) (*http.Response, error) {
			if req.URL.Scheme != "https" || req.URL.Host != bucket+".t3.storageapi.dev" {
				t.Errorf("unexpected S3 origin")
			}
			response := func(status int, content string) (*http.Response, error) {
				h := make(http.Header)
				h.Set("Content-Length", strconv.Itoa(len(content)))
				if status == http.StatusOK || status == http.StatusPartialContent {
					h.Set("ETag", "\"synthetic-etag\"")
					// MinIO requires a valid RFC7231 Last-Modified value on GET.
					h.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
				}
				return &http.Response{StatusCode: status, Header: h, ContentLength: int64(len(content)), Body: io.NopCloser(strings.NewReader(content)), Request: req}, nil
			}
			if req.URL.Query().Has("policy") {
				if tc.publicPolicy {
					return response(200, `{"Statement":[{"Effect":"Allow","Principal":"*"}]}`)
				}
				return response(403, denial)
			}
			if strings.Contains(req.URL.RawQuery, "location") {
				t.Errorf("unexpected S3 location lookup")
				return response(400, denial)
			}
			if req.Header.Get("Authorization") == "" {
				anonRead++
				if req.Method != http.MethodGet || objectPath == "" || req.URL.Path != objectPath || req.URL.RawQuery != "" {
					t.Errorf("unsigned request addressed wrong probe object")
				}
				return response(tc.anonStatus, tc.anonBody)
			}
			switch req.Method {
			case http.MethodPut:
				uploaded++
				objectPath = req.URL.Path
				var err error
				stored, err = io.ReadAll(io.LimitReader(req.Body, 65))
				if err != nil || len(stored) != 32 {t.Errorf("invalid signed probe upload")}
				return response(200, "")
			case http.MethodGet:
				signedRead++
				signedGetRange = req.Header.Get("Range")
				if req.URL.Path != objectPath {t.Errorf("signed read changed probe key")}
				data := string(stored)
				if tc.badSignedRead {data = "different"}
				if req.Header.Get("Range") != "" {
					reply, err := response(http.StatusPartialContent, data)
					reply.Header.Set("Content-Range", "bytes 0-"+strconv.Itoa(len(data)-1)+"/"+strconv.Itoa(len(stored)))
					reply.Header.Set("Accept-Ranges", "bytes")
					return reply, err
				}
				return response(200, data)
			case http.MethodHead:
				// GetObject internally requests HEAD to learn the original size.
				reply, err := response(200, "")
				reply.Header.Set("Content-Length", strconv.Itoa(len(stored)))
				reply.ContentLength = int64(len(stored))
				return reply, err
			case http.MethodDelete:
				deletes++
				if req.URL.Path != objectPath {t.Errorf("cleanup changed probe key")}
				if tc.deleteFailure {return nil, errors.New("synthetic cleanup error")}
				return response(204, "")
			default:
				t.Errorf("unexpected S3 method: %s", req.Method)
				return response(405, "")
			}
		})
		client, err := minio.New("t3.storageapi.dev", &minio.Options{
			Creds: credentials.NewStaticV4("fixture-access", "fixture-secret", ""),
			Secure: true, Region: "auto", BucketLookup: minio.BucketLookupDNS, Transport: transport,
		})
		if err != nil {t.Fatal(err)}
		anon := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		err = verifyStoragePrivacy(context.Background(), client, bucket, railwayStagingProbeMode, anon)
		if (err == nil) != tc.wantSuccess {
			t.Fatalf("%s: success=%v expected=%v: %v (uploaded=%d, signedReads=%d, anonymousReads=%d, deletes=%d, storedLength=%d, signedRange=%q)", tc.name, err == nil, tc.wantSuccess, err, uploaded, signedRead, anonRead, deletes, len(stored), signedGetRange)
		}
		if tc.publicPolicy {
			if uploaded != 0 || signedRead != 0 || anonRead != 0 || deletes != 0 {t.Fatal("explicit public policy must stop before probing")}
		} else if uploaded != 1 || (tc.deleteFailure && deletes < 1) || (!tc.deleteFailure && deletes != 1) {
			// The SDK retries failed DELETE operations; every attempt must
			// remain fail-closed rather than silently succeeding.
			t.Fatalf("%s: probe/cleanup accounting invalid (%d,%d)", tc.name, uploaded, deletes)
		}
		if tc.name == "private" && (signedRead == 0 || anonRead != 1 || len(stored) != 32) {
			t.Fatal("valid staging attestation missing a check")
		}
	}
}
