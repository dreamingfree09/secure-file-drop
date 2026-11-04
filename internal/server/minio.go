package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// normaliseEndpoint parses a MinIO endpoint string and determines whether to use TLS.
// Accepts formats: "minio:9000", "http://minio:9000", or "https://minio:9000".
// Returns the host:port endpoint, secure flag (true for HTTPS), or an error.
func normaliseEndpoint(raw string) (endpoint string, secure bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, fmt.Errorf("empty endpoint")
	}

	// Accept either "minio:9000" or "http://minio:9000" / "https://minio:9000".
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false, err
		}
		if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", false, fmt.Errorf("invalid endpoint")
		}
		if u.Path != "" && u.Path != "/" {
			return "", false, fmt.Errorf("endpoint must not contain a path")
		}
		secure = (u.Scheme == "https")
		return u.Host, secure, nil
	}

	// No scheme provided, treat as host:port (insecure by default for local MinIO).
	return raw, false, nil
}

// storageBucketLookup is explicit for non-AWS S3 services. Auto preserves the
// historical client default, while newer Railway buckets require virtual DNS.
func storageBucketLookup(mode string) (minio.BucketLookupType, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return minio.BucketLookupAuto, nil
	case "virtual":
		return minio.BucketLookupDNS, nil
	case "path":
		return minio.BucketLookupPath, nil
	default:
		return minio.BucketLookupAuto, fmt.Errorf("unsupported S3 bucket addressing style")
	}
}

func newMinioClient() (*minio.Client, string, error) {
	rawEndpoint := os.Getenv("SFD_S3_ENDPOINT")
	accessKey := os.Getenv("SFD_S3_ACCESS_KEY")
	secretKey := os.Getenv("SFD_S3_SECRET_KEY")
	bucket := os.Getenv("SFD_BUCKET")

	if rawEndpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, "", fmt.Errorf("minio configuration incomplete")
	}

	// Validate the opt-in scope before parsing untrusted endpoint text. URL
	// parser errors can echo their input, which must not reach startup logs.
	railway, err := railwayPrivacySelected()
	if err != nil {
		return nil, "", err
	}
	if railway {
		lookup, err := storageBucketLookup(os.Getenv("SFD_S3_BUCKET_LOOKUP"))
		if err != nil {
			return nil, "", err
		}
		if err := validateRailwayPrivacyConfig(rawEndpoint, bucket, lookup); err != nil {
			return nil, "", err
		}
		return newRailwayStagingClient(rawEndpoint, bucket, accessKey, secretKey)
	}

	endpoint, secure, err := normaliseEndpoint(rawEndpoint)
	if err != nil {
		return nil, "", err
	}
	lookup, err := storageBucketLookup(os.Getenv("SFD_S3_BUCKET_LOOKUP"))
	if err != nil {
		return nil, "", err
	}
	if lookup == minio.BucketLookupDNS && !secure {
		return nil, "", fmt.Errorf("virtual-hosted S3 addressing requires HTTPS")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       secure,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, "", err
	}

	// Sanity check: bucket must exist.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", fmt.Errorf("minio bucket does not exist: %s", bucket)
	}

	policy, err := client.GetBucketPolicy(ctx, bucket)
	if err != nil {
		return nil, "", classifyBucketPolicyError(err)
	}
	if !privateBucketPolicy(policy) {
		return nil, "", fmt.Errorf("bucket policy check failed: unverified privacy")
	}
	return client, bucket, nil
}

// classifyBucketPolicyError exposes only allowlisted failure categories.
// Do not include provider error text: it can contain hosts, buckets or signed URLs.
// No category authorizes acceptance or changes the privacy decision.
func classifyBucketPolicyError(err error) error {
	response := minio.ToErrorResponse(err)
	category := "unclassified"
	switch response.Code {
	case "AccessDenied":
		category = "access_denied"
	case "NotImplemented", "NotSupported", "UnsupportedOperation":
		category = "operation_unsupported"
	case "InvalidAccessKeyId", "SignatureDoesNotMatch", "AuthorizationHeaderMalformed":
		category = "credential_or_signature"
	case "NoSuchBucket":
		category = "bucket_missing"
	case "NoSuchBucketPolicy":
		category = "bucket_policy_unavailable"
	}
	return fmt.Errorf("bucket policy verification unavailable: %s (fail-closed)", category)
}

// privateBucketPolicy permits policies restricted to named principals, but
// rejects anonymous grants, unknown principal forms and NotPrincipal allows.
func privateBucketPolicy(policy string) bool {
	if strings.TrimSpace(policy) == "" {
		return true
	}
	type statement struct {
		Effect       string
		Principal    json.RawMessage
		NotPrincipal json.RawMessage
	}
	var document struct{ Statement json.RawMessage }
	if json.Unmarshal([]byte(policy), &document) != nil {
		return false
	}
	var statements []statement
	if json.Unmarshal(document.Statement, &statements) != nil {
		var single statement
		if json.Unmarshal(document.Statement, &single) != nil {
			return false
		}
		statements = []statement{single}
	}
	if len(statements) == 0 {
		return false
	}
	var named func(any) bool
	named = func(value any) bool {
		switch v := value.(type) {
		case string:
			return v != "" && !strings.Contains(v, "*")
		case []any:
			if len(v) == 0 {
				return false
			}
			for _, item := range v {
				if !named(item) {
					return false
				}
			}
			return true
		case map[string]any:
			if len(v) == 0 {
				return false
			}
			for key, item := range v {
				if key != "AWS" && key != "Service" && key != "CanonicalUser" {
					return false
				}
				if !named(item) {
					return false
				}
			}
			return true
		default:
			return false
		}
	}
	for _, s := range statements {
		if s.Effect == "Deny" {
			continue
		}
		if s.Effect != "Allow" || len(s.NotPrincipal) > 0 {
			return false
		}
		var p any
		if json.Unmarshal(s.Principal, &p) != nil || !named(p) {
			return false
		}
	}
	return true
}
