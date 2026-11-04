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
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	railwayPrivacyMode        = "railway-positive-probe"
	railwayStorageEndpoint    = "https://t3.storageapi.dev"
	railwayStagingProject     = "d8d1c061-84be-4302-91a2-e24e8ec1326a"
	railwayStagingEnvironment = "85219452-c05b-487d-8cbe-7c3366697ca3"
)

// Unknown modes never silently fall back. The empty/default path is unchanged.
func railwayPrivacySelected() (bool, error) {
	switch os.Getenv("SFD_S3_PRIVACY_MODE") {
	case "", "bucket-policy":
		return false, nil
	case railwayPrivacyMode:
		return true, nil
	default:
		return false, privacyFailure("invalid_mode")
	}
}

func validateRailwayPrivacyConfig(endpoint, bucket string, lookup minio.BucketLookupType) error {
	if endpoint != railwayStorageEndpoint || lookup != minio.BucketLookupDNS ||
		os.Getenv("SFD_ENV") != "staging" || os.Getenv("RAILWAY_PROJECT_ID") != railwayStagingProject ||
		os.Getenv("RAILWAY_ENVIRONMENT_ID") != railwayStagingEnvironment {
		return privacyFailure("staging_scope")
	}
	// Exactly one DNS label; no dots, URL syntax, ports or alternate authority.
	if len(bucket) < 3 || len(bucket) > 63 || bucket[0] == '-' || bucket[len(bucket)-1] == '-' {
		return privacyFailure("bucket_identity")
	}
	for _, c := range bucket {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return privacyFailure("bucket_identity")
		}
	}
	return nil
}

// Stage names are code constants. Never wrap provider errors, URLs or payloads.
func privacyFailure(stage string) error {
	return fmt.Errorf("railway staging storage attestation failed: %s (fail-closed)", stage)
}

type privacyTransport struct {
	base     http.RoundTripper
	host     string
	unsigned bool
}

func (t privacyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != t.host || req.URL.User != nil ||
		req.URL.Opaque != "" || req.URL.Fragment != "" || req.Host != "" && req.Host != t.host {
		return nil, privacyFailure("request_identity")
	}
	if t.unsigned {
		if req.Method != http.MethodGet && req.Method != http.MethodPut && req.Method != http.MethodDelete || req.Header.Get("Authorization") != "" ||
			req.Header.Get("Proxy-Authorization") != "" || req.Header.Get("Cookie") != "" {
			return nil, privacyFailure("unsigned_credentials")
		}
		for key := range req.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-amz-") {
				return nil, privacyFailure("unsigned_credentials")
			}
		}
		if req.URL.RawPath != "" || req.URL.Opaque != "" {
			return nil, privacyFailure("unsigned_identity")
		}
		if req.URL.Path == "/" {
			if req.Method != http.MethodGet || req.URL.RawQuery != "max-keys=1" && req.URL.RawQuery != "list-type=2&max-keys=1" {
				return nil, privacyFailure("unsigned_query")
			}
		} else if !strings.HasPrefix(req.URL.Path, "/_sfd_privacy_attestation/") || req.URL.RawQuery != "" {
			return nil, privacyFailure("unsigned_query")
		}
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, privacyFailure("transport")
	}
	if resp == nil || resp.Body == nil {
		return nil, privacyFailure("empty_response")
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_ = resp.Body.Close()
		return nil, privacyFailure("redirect")
	}
	return resp, nil
}

func newRailwayStagingClient(endpoint, bucket, accessKey, secretKey string) (*minio.Client, string, error) {
	if err := validateRailwayPrivacyConfig(endpoint, bucket, minio.BucketLookupDNS); err != nil {
		return nil, "", err
	}
	signedTransport, err := minio.DefaultTransport(true)
	if err != nil {
		return nil, "", privacyFailure("transport_setup")
	}
	unsignedTransport := signedTransport.Clone()
	// Do not inherit proxy credentials/routes. Both transports keep normal TLS.
	signedTransport.Proxy = nil
	unsignedTransport.Proxy = nil
	defer unsignedTransport.CloseIdleConnections()
	host := bucket + ".t3.storageapi.dev"
	client, err := minio.New("t3.storageapi.dev", &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: true,
		Region: "auto", BucketLookup: minio.BucketLookupDNS, MaxRetries: 1,
		Transport: privacyTransport{base: signedTransport, host: host},
	})
	if err != nil {
		signedTransport.CloseIdleConnections()
		return nil, "", privacyFailure("client_setup")
	}
	unsigned := &http.Client{Transport: privacyTransport{base: unsignedTransport, host: host, unsigned: true},
		Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := attestRailwayStorage(ctx, client, bucket, unsigned, rand.Reader); err != nil {
		signedTransport.CloseIdleConnections()
		return nil, "", err
	}
	return client, bucket, nil
}

func readPrivacyObject(ctx context.Context, client *minio.Client, bucket, key string, size int) ([]byte, error) {
	object, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(object, int64(size)+1))
	closeErr := object.Close()
	if readErr != nil {
		return nil, readErr
	}
	return body, closeErr
}

// A failed PUT can have committed. Delete only this random key, and only when a
// successful PUT or a matching authenticated read proves it is our probe.
func cleanupPrivacyProbe(client *minio.Client, bucket, key string, payload []byte, created bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !created {
		body, err := readPrivacyObject(ctx, client, bucket, key, len(payload))
		missing := minio.ToErrorResponse(err)
		if err != nil && missing.Code == "NoSuchKey" && missing.StatusCode == 404 {
			return nil
		}
		if err != nil || !bytes.Equal(body, payload) {
			return privacyFailure("cleanup_identity")
		}
	}
	if err := client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return privacyFailure("cleanup_delete")
	}
	_, err := client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	missing := minio.ToErrorResponse(err)
	if err == nil || missing.Code != "NoSuchKey" || missing.StatusCode != 404 {
		return privacyFailure("cleanup_confirmation")
	}
	return nil
}

func attestRailwayStorage(ctx context.Context, client *minio.Client, bucket string, unsigned *http.Client, random io.Reader) (result error) {
	nonce := make([]byte, 32)
	content := make([]byte, 32)
	if _, err := io.ReadFull(random, nonce); err != nil {
		return privacyFailure("entropy")
	}
	if _, err := io.ReadFull(random, content); err != nil {
		return privacyFailure("entropy")
	}
	key := "_sfd_privacy_attestation/" + hex.EncodeToString(nonce)
	payload := []byte("Secure File Drop harmless staging probe: " + hex.EncodeToString(content))
	created, collision := false, false
	defer func() {
		if !collision {
			result = errors.Join(result, cleanupPrivacyProbe(client, bucket, key, payload, created))
		}
	}()
	opts := minio.PutObjectOptions{ContentType: "application/octet-stream", CacheControl: "no-store", DisableMultipart: true, SendContentMd5: true}
	opts.SetMatchETagExcept("*")
	info, err := client.PutObject(ctx, bucket, key, bytes.NewReader(payload), int64(len(payload)), opts)
	if err != nil {
		failure := minio.ToErrorResponse(err)
		collision = failure.Code == "PreconditionFailed" && failure.StatusCode == 412
		return privacyFailure("authenticated_put")
	}
	created = true
	if info.Key != key || info.Bucket != bucket || info.Size != int64(len(payload)) || info.ETag == "" {
		return privacyFailure("put_identity")
	}
	body, err := readPrivacyObject(ctx, client, bucket, key, len(payload))
	if err != nil || !bytes.Equal(body, payload) {
		return privacyFailure("authenticated_get")
	}
	// Prove the listing API is functional and names this exact new object.
	listCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	count := 0
	for item := range client.ListObjects(listCtx, bucket, minio.ListObjectsOptions{Prefix: key, Recursive: true, MaxKeys: 2}) {
		if item.Err != nil || item.Key != key || item.Size != int64(len(payload)) || count != 0 {
			return privacyFailure("authenticated_list")
		}
		count++
	}
	if count != 1 {
		return privacyFailure("authenticated_list")
	}
	objectURL := url.URL{Scheme: "https", Host: bucket + ".t3.storageapi.dev", Path: "/" + key}
	if err := requireAnonymousDenial(ctx, unsigned, objectURL, bucket, key, http.MethodGet); err != nil {
		return err
	}
	// Check both S3 listing forms, not merely a prefix-restricted view.
	for _, query := range []string{"max-keys=1", "list-type=2&max-keys=1"} {
		listingURL := url.URL{Scheme: "https", Host: objectURL.Host, Path: "/", RawQuery: query}
		if err := requireAnonymousDenial(ctx, unsigned, listingURL, bucket, "", http.MethodGet); err != nil {
			return err
		}
	}
	// Public write/delete grants are not compatible with private storage either.
	// These requests can touch only the newly created harmless probe.
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		if err := requireAnonymousDenial(ctx, unsigned, objectURL, bucket, key, method); err != nil {
			return err
		}
	}
	// A disappearing object cannot manufacture a passing anonymous result.
	body, err = readPrivacyObject(ctx, client, bucket, key, len(payload))
	if err != nil || !bytes.Equal(body, payload) {
		return privacyFailure("authenticated_recheck")
	}
	return nil
}

func requireAnonymousDenial(ctx context.Context, client *http.Client, target url.URL, bucket, key, method string) error {
	if target.Scheme != "https" || target.Host != bucket+".t3.storageapi.dev" || target.Path != "/"+key || target.User != nil || target.RawPath != "" || target.Opaque != "" || target.Fragment != "" {
		return privacyFailure("unsigned_identity")
	}
	var requestBody io.Reader
	if method == http.MethodPut {
		requestBody = strings.NewReader("unauthenticated probe write must be denied")
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), requestBody)
	if err != nil {
		return privacyFailure("unsigned_request")
	}
	req.Header.Set("Cache-Control", "no-store, no-cache")
	resp, err := client.Do(req)
	if err != nil {
		return privacyFailure("unsigned_transport")
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8193))
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil || len(body) > 8192 || resp.StatusCode != http.StatusForbidden {
		return privacyFailure("unsigned_response")
	}
	var denial struct {
		XMLName   xml.Name
		Codes     []string `xml:"Code"`
		Keys      []string `xml:"Key"`
		Buckets   []string `xml:"BucketName"`
		Resources []string `xml:"Resource"`
	}
	if !privacyDenialElements(body) {
		return privacyFailure("unsigned_denial")
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	root, err := privacyDenialRoot(decoder, body)
	if err != nil || decoder.DecodeElement(&denial, &root) != nil || denial.XMLName.Local != "Error" || len(denial.Codes) != 1 || denial.Codes[0] != "AccessDenied" || len(denial.Keys) > 1 || len(denial.Buckets) > 1 || len(denial.Resources) > 1 {
		return privacyFailure("unsigned_denial")
	}
	// Require one complete XML document; no ambiguous suffix or second response.
	for {
		before := decoder.InputOffset()
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return privacyFailure("unsigned_denial")
		}
		if _, ok := token.(xml.CharData); !ok || len(bytes.Trim(body[before:decoder.InputOffset()], " \t\r\n")) != 0 {
			return privacyFailure("unsigned_denial")
		}
	}
	if len(denial.Keys) == 1 && denial.Keys[0] != key || len(denial.Buckets) == 1 && denial.Buckets[0] != bucket {
		return privacyFailure("unsigned_identity")
	}
	resource := "/" + bucket
	if key != "" {
		resource += "/" + key
	}
	if len(denial.Resources) == 1 && denial.Resources[0] != resource && denial.Resources[0] != "/"+key {
		return privacyFailure("unsigned_identity")
	}
	return nil
}


// A denial needs no element attributes except an optional standard S3 default
// namespace on its root. Reject other attributes instead of trusting the XML
// decoder's permissive attribute-separator grammar. No directives or interior
// processing instructions are part of this accepted S3 error subset.
func privacyDenialElements(body []byte) bool {
	const namespace = "http://s3.amazonaws.com/doc/2006-03-01/"
	decoder := xml.NewDecoder(bytes.NewReader(body))
	depth := 0
	for {
		before := decoder.InputOffset()
		token, err := decoder.Token()
		if err == io.EOF {
			return depth == 0
		}
		if err != nil {
			return false
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			if value.Name.Space != "" && value.Name.Space != namespace {
				return false
			}
			if len(value.Attr) != 0 {
				if depth != 1 || len(value.Attr) != 1 || value.Attr[0].Name.Space != "" || value.Attr[0].Name.Local != "xmlns" || value.Attr[0].Value != namespace {
					return false
				}
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return false
		case xml.ProcInst:
			if before != 0 || value.Target != "xml" || !privacyXMLDeclaration(value.Inst) {
				return false
			}
		}
	}
}

// Decode skips leading character data. Validate every prolog token before
// DecodeElement, and inspect raw whitespace so character references and Unicode
// spaces outside the root cannot masquerade as legal XML document whitespace.
func privacyDenialRoot(decoder *xml.Decoder, body []byte) (xml.StartElement, error) {
	for {
		before := decoder.InputOffset()
		token, err := decoder.Token()
		if err != nil {
			return xml.StartElement{}, privacyFailure("unsigned_denial")
		}
		switch value := token.(type) {
		case xml.StartElement:
			return value, nil
		case xml.CharData:
			if len(bytes.Trim(body[before:decoder.InputOffset()], " \t\r\n")) != 0 {
				return xml.StartElement{}, privacyFailure("unsigned_denial")
			}
		case xml.Comment:
			// XML comments are legal before the document element.
		case xml.ProcInst:
			if before != 0 || value.Target != "xml" || !privacyXMLDeclaration(value.Inst) {
				return xml.StartElement{}, privacyFailure("unsigned_denial")
			}
		default:
			// No DTD or arbitrary processing instructions in an S3 denial.
			return xml.StartElement{}, privacyFailure("unsigned_denial")
		}
	}
}

// XML declarations require literal XML whitespace between attributes. The XML
// decoder also accepts adjacent attributes, so enforce this bounded, anchored
// XML1.0 UTF-8 subset against the original instruction, not decoded attributes.
var privacyXMLDeclarationPattern = regexp.MustCompile(`^[ \t\r\n]*version[ \t\r\n]*=[ \t\r\n]*(?:"1\.0"|'1\.0')(?:[ \t\r\n]+encoding[ \t\r\n]*=[ \t\r\n]*(?:"(?i:utf-8)"|'(?i:utf-8)'))?(?:[ \t\r\n]+standalone[ \t\r\n]*=[ \t\r\n]*(?:"(?:yes|no)"|'(?:yes|no)'))?[ \t\r\n]*$`)

func privacyXMLDeclaration(instruction []byte) bool {
	return privacyXMLDeclarationPattern.Match(instruction)
}
