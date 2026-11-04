package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type policyFixtureTransport func(*http.Request) (*http.Response, error)

func (f policyFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Models provider responses through the actual SDK. This is not evidence that
// any real Railway bucket implements GetBucketPolicy.
func TestStoragePolicyResponses(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		private bool
	}{
		{200, `{"Statement":{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::123:user/backend"}}}`, true},
		{200, `{"Statement":{"Effect":"Allow","Principal":"*"}}`, false},
		{200, `invalid`, false},
		{403, `<Error><Code>AccessDenied</Code></Error>`, false},
		{501, `<Error><Code>NotImplemented</Code></Error>`, false},
		{404, `<Error><Code>NoSuchBucketPolicy</Code></Error>`, true},
	} {
		requests := 0
		transport := policyFixtureTransport(func(req *http.Request) (*http.Response, error) {
			requests++
			if req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != "owned-fixture.objects.example.invalid" || !req.URL.Query().Has("policy") {
				t.Errorf("unexpected policy request addressing")
			}
			return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{"application/xml"}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
		})
		client, err := minio.New("objects.example.invalid", &minio.Options{
			Creds:  credentials.NewStaticV4("synthetic-access", "synthetic-secret", ""),
			Secure: true, Region: "auto", BucketLookup: minio.BucketLookupDNS, Transport: transport,
		})
		if err != nil {
			t.Fatal(err)
		}
		policy, err := client.GetBucketPolicy(context.Background(), "owned-fixture")
		if requests == 0 || (err == nil && privateBucketPolicy(policy)) != tc.private {
			t.Fatalf("policy status=%d accepted=%v want=%v", tc.status, err == nil && privateBucketPolicy(policy), tc.private)
		}
	}
}
