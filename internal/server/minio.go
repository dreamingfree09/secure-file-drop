package server

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

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
		if u.Host == "" {
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

func newMinioClient() (*minio.Client, string, error) {
	rawEndpoint := os.Getenv("SFD_S3_ENDPOINT")
	accessKey := os.Getenv("SFD_S3_ACCESS_KEY")
	secretKey := os.Getenv("SFD_S3_SECRET_KEY")
	bucket := os.Getenv("SFD_BUCKET")

	if rawEndpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, "", fmt.Errorf("minio configuration incomplete")
	}

	endpoint, secure, err := normaliseEndpoint(rawEndpoint)
	if err != nil {
		return nil, "", err
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, "", err
	}

	// Sanity check: bucket must exist.
	exists, err := client.BucketExists(context.Background(), bucket)
	if err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", fmt.Errorf("minio bucket does not exist: %s", bucket)
	}

	return client, bucket, nil
}
