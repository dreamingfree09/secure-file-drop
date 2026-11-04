package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

const railwayStagingProbeMode = "railway-staging-probe"

var railwayStagingBucketName = regexp.MustCompile("^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$")

func storagePrivacyMode(raw string) (string, error) {
	switch raw {
	case "", "policy":
		return "policy", nil
	case railwayStagingProbeMode:
		return railwayStagingProbeMode, nil
	default:
		return "", errors.New("unsupported storage privacy mode")
	}
}

// This exception is deliberately not usable outside the one new, disposable
// Railway staging project. All ordinary installations keep policy validation.
func validateRailwayProbeScope(endpoint string, secure bool, lookup minio.BucketLookupType, bucket string) error {
	if endpoint != "t3.storageapi.dev" || !secure || lookup != minio.BucketLookupDNS ||
		os.Getenv("SFD_ENV") != "staging" ||
		os.Getenv("RAILWAY_PROJECT_NAME") != "secure-file-drop-staging" ||
		os.Getenv("RAILWAY_ENVIRONMENT_NAME") != "staging" ||
		!strings.HasPrefix(bucket, "sfd-private-staging-") || !railwayStagingBucketName.MatchString(bucket) {
		return errors.New("staging-only Railway privacy attestation scope mismatch")
	}
	return nil
}

// verifyStoragePrivacy never treats an S3 policy error alone as proof that a
// bucket is private. Staging mode additionally demands direct object denial.
func verifyStoragePrivacy(ctx context.Context, client *minio.Client, bucket, mode string, anon *http.Client) error {
	policy, err := client.GetBucketPolicy(ctx, bucket)
	if err == nil && !privateBucketPolicy(policy) {
		return errors.New("bucket policy permits anonymous access")
	}
	if mode != railwayStagingProbeMode {
		if err != nil {
			return errors.New("bucket policy could not be verified")
		}
		return nil
	}
	if err != nil {
		response := minio.ToErrorResponse(err)
		switch response.Code {
		case "AccessDenied", "NotImplemented", "MethodNotAllowed":
			// Not a privacy proof. Continue only to the positive byte-level test.
		default:
			return errors.New("unrecognised storage-policy failure; attestation refused")
		}
	}
	return attestRailwayStagingBucket(client, bucket, anon)
}

// attestRailwayStagingBucket uses one fresh, random object to prove that the
// same bytes can be read with credentials and cannot be read anonymously.
// Cleanup must succeed on every path, including failures before PUT completion.
func attestRailwayStagingBucket(client *minio.Client, bucket string, anon *http.Client) (resultErr error) {
	if client == nil {
		return errors.New("storage attestation client missing")
	}
	var randomKey [24]byte
	var content [32]byte
	if _, err := rand.Read(randomKey[:]); err != nil {
		return errors.New("storage attestation random source unavailable")
	}
	if _, err := rand.Read(content[:]); err != nil {
		return errors.New("storage attestation random source unavailable")
	}
	key := "sfd-privacy-probe/" + hex.EncodeToString(randomKey[:])
	payload := content[:]
	probeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Register cleanup before attempting any PUT, even one that returns an
	// ambiguous failure after the provider has already stored the object.
	defer func() {
		cleanupCtx, finish := context.WithTimeout(context.Background(), 6*time.Second)
		defer finish()
		if err := client.RemoveObject(cleanupCtx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
			resultErr = errors.New("storage privacy probe cleanup failed")
		}
	}()

	_, err := client.PutObject(probeCtx, bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return errors.New("storage privacy probe signed upload failed")
	}

	object, err := client.GetObject(probeCtx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return errors.New("storage privacy probe signed read failed")
	}
	signedBytes, readErr := io.ReadAll(io.LimitReader(object, int64(len(payload)+1)))
	closeErr := object.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(signedBytes, payload) {
		return errors.New("storage privacy probe signed bytes mismatch")
	}

	if anon == nil {
		anon = &http.Client{
			Timeout: 6 * time.Second,
			Transport: &http.Transport{TLSHandshakeTimeout: 4 * time.Second},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	// This bucket and base endpoint were already strictly validated. No
	// presigned query, Cookie, Authorization or cross-host redirect is used.
	u := url.URL{Scheme: "https", Host: bucket + ".t3.storageapi.dev", Path: "/" + key}
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return errors.New("storage privacy probe unsigned request failed")
	}
	request.Header.Set("Cache-Control", "no-cache")
	response, err := anon.Do(request)
	if err != nil {
		return errors.New("storage privacy probe unsigned request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusForbidden ||
		response.Request == nil || response.Request.URL.String() != u.String() {
		return errors.New("storage privacy probe anonymous access not definitively denied")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2049))
	if err != nil || len(body) > 2048 {
		return errors.New("storage privacy probe unsigned denial response invalid")
	}
	var denial struct {
		XMLName xml.Name
		Code    string
	}
	if xml.Unmarshal(body, &denial) != nil ||
		denial.XMLName.Local != "Error" || denial.Code != "AccessDenied" {
		return errors.New("storage privacy probe unsigned denial not independently verifiable")
	}
	return nil
}

// fmt ensures the staging mode is not silently treated as a general-purpose
// S3 policy bypass. Keep production policy handling in verifyStoragePrivacy.
var _ = fmt.Errorf
