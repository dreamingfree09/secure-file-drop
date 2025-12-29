// sha256FromMinioObject reads an object from MinIO, computes its SHA-256
// and returns both hex and raw byte representations along with the number
// of bytes processed. Errors are returned when streaming fails.
// hash.go - SHA-256 computation via native helper for stored objects.
//
// Downloads MinIO objects to temp files and invokes sfd-hash to
// produce deterministic integrity metadata.
package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

// hashToolOutput represents the JSON output from the native C hash utility (sfd-hash).
// The tool calculates SHA-256 hashes using OpenSSL's libcrypto for performance and
// reliability. This provides file integrity verification independent of Go's crypto.
type hashToolOutput struct {
	Algorithm string `json:"algorithm"`
	Hash      string `json:"hash"`
	Bytes     uint64 `json:"bytes"`
}

// runHashTool executes the native C hash utility (sfd-hash) to calculate the SHA-256
// hash of a local file. The tool path is read from SFD_HASH_TOOL env var, defaulting
// to "/app/sfd-hash". The function validates the JSON output and ensures the hash is
// valid hex-encoded SHA-256 (64 characters).
//
// Returns hashToolOutput with algorithm ("sha256"), hash (hex string), and byte count.
// Times out after 2 minutes to prevent indefinite hangs on large files.
func runHashTool(ctx context.Context, filePath string) (hashToolOutput, error) {
	// Ensure we do not hang indefinitely on large files or slow storage
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	toolPath := os.Getenv("SFD_HASH_TOOL")
	if toolPath == "" {
		toolPath = "/app/sfd-hash"
	}
	cmd := exec.CommandContext(ctx, toolPath, filePath)
	out, err := cmd.Output()
	if err != nil {
		return hashToolOutput{}, fmt.Errorf("hash tool failed: %w", err)
	}

	var parsed hashToolOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return hashToolOutput{}, fmt.Errorf("hash tool output not json: %w", err)
	}

	parsed.Algorithm = strings.TrimSpace(parsed.Algorithm)
	parsed.Hash = strings.TrimSpace(strings.ToLower(parsed.Hash))

	if parsed.Algorithm != "sha256" {
		return hashToolOutput{}, fmt.Errorf("unexpected algorithm: %q", parsed.Algorithm)
	}
	if len(parsed.Hash) != 64 {
		return hashToolOutput{}, fmt.Errorf("unexpected hash length: %d", len(parsed.Hash))
	}

	// Validate hex.
	if _, err := hex.DecodeString(parsed.Hash); err != nil {
		return hashToolOutput{}, fmt.Errorf("hash is not valid hex: %w", err)
	}

	return parsed, nil
}

// sha256FromMinioObject downloads a file from MinIO to a temporary local file,
// runs the C hash utility on it, and returns the SHA-256 hash in both hex string
// and raw byte formats, plus the file size. This is used during the upload flow
// to verify file integrity after storage.
//
// The temporary file is automatically cleaned up after hashing.
// Returns error if MinIO stream fails or hash calculation fails.
func sha256FromMinioObject(ctx context.Context, mc *minio.Client, bucket, objectKey string) (sha256Hex string, sha256Bytes []byte, size uint64, err error) {
	// Validate required parameters
	if mc == nil {
		return "", nil, 0, errors.New("minio client is nil")
	}
	if bucket == "" || objectKey == "" {
		return "", nil, 0, errors.New("bucket/objectKey missing")
	}

	// Stream MinIO object to a temporary file, then hash locally via the C utility.
	tmp, err := os.CreateTemp("", "sfd-hash-*")
	if err != nil {
		return "", nil, 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	obj, err := mc.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return "", nil, 0, fmt.Errorf("get object: %w", err)
	}
	defer func() { _ = obj.Close() }()

	if _, err := io.Copy(tmp, obj); err != nil {
		return "", nil, 0, fmt.Errorf("copy object to temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", nil, 0, fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", nil, 0, fmt.Errorf("close temp: %w", err)
	}

	out, err := runHashTool(ctx, tmpPath)
	if err != nil {
		return "", nil, 0, err
	}

	raw, err := hex.DecodeString(out.Hash)
	if err != nil {
		return "", nil, 0, fmt.Errorf("decode sha256 hex: %w", err)
	}
	if len(raw) != 32 {
		return "", nil, 0, fmt.Errorf("decoded sha256 length unexpected: %d", len(raw))
	}

	return out.Hash, raw, out.Bytes, nil
}
