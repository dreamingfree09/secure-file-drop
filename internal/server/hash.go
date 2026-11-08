package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
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

// runHashTool keeps the existing operator-supplied file-only helper contract.
func runHashTool(ctx context.Context, filePath string) (hashToolOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return runNativeHash(ctx, filePath, nil)
}

func runNativeHash(ctx context.Context, argument string, input io.Reader) (hashToolOutput, error) {
	toolPath := os.Getenv("SFD_HASH_TOOL")
	if toolPath == "" {
		toolPath = "/app/sfd-hash"
	}
	if !filepath.IsAbs(toolPath) {
		return hashToolOutput{}, errors.New("hash tool must be an absolute operator-configured path")
	}
	// #nosec G703 -- Read-only metadata check on the absolute operator-configured executable, never an HTTP path.
	info, err := os.Stat(toolPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 {
		return hashToolOutput{}, errors.New("hash tool must be a regular executable without group/other write access")
	}
	// #nosec G204 G702 -- Operator-only absolute regular executable; constant stream flag or generated local file argument; no shell.
	cmd := exec.CommandContext(ctx, toolPath, argument)
	cmd.Stdin = input
	cmd.WaitDelay = 2 * time.Second
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

// sha256FromMinioObject hashes the stored bytes using the native C utility.
// The bundled utility reads through a bounded pipe under the caller's context.
func sha256FromMinioObject(ctx context.Context, mc *minio.Client, bucket, objectKey string) (sha256Hex string, sha256Bytes []byte, size uint64, err error) {
	// Validate required parameters
	if mc == nil {
		return "", nil, 0, errors.New("minio client is nil")
	}
	if bucket == "" || objectKey == "" {
		return "", nil, 0, errors.New("bucket/objectKey missing")
	}

	obj, err := mc.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return "", nil, 0, fmt.Errorf("get object: %w", err)
	}
	defer func() { _ = obj.Close() }()

	var out hashToolOutput
	if os.Getenv("SFD_HASH_TOOL") == "" {
		out, err = hashStream(ctx, obj)
	} else {
		out, err = hashViaTemporaryFile(ctx, obj)
	}
	if err != nil {
		return "", nil, 0, err
	}
	raw, err := hex.DecodeString(out.Hash)
	if err != nil {
		return "", nil, 0, fmt.Errorf("decode sha256 hex: %w", err)
	}
	return out.Hash, raw, out.Bytes, nil
}

type hashInput struct {
	reader io.Reader
	bytes  int64
	eof    bool
}

func (input *hashInput) Read(buffer []byte) (int, error) {
	n, err := input.reader.Read(buffer)
	if n < 0 || input.bytes > math.MaxInt64-int64(n) {
		return 0, errors.New("invalid hash input size")
	}
	input.bytes += int64(n)
	input.eof = err == io.EOF
	return n, err
}

func hashStream(ctx context.Context, input io.Reader) (hashToolOutput, error) {
	source := &hashInput{reader: input}
	out, err := runNativeHash(ctx, "--stdin", source)
	if err != nil {
		return hashToolOutput{}, err
	}
	if !source.eof || out.Bytes > math.MaxInt64 || int64(out.Bytes) != source.bytes {
		return hashToolOutput{}, errors.New("hash tool did not verify the complete input")
	}
	return out, nil
}

func hashViaTemporaryFile(ctx context.Context, input io.Reader) (hashToolOutput, error) {
	tmp, err := os.CreateTemp("", "sfd-hash-*")
	if err != nil {
		return hashToolOutput{}, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	copied, copyErr := io.Copy(tmp, input)
	if copyErr != nil {
		return hashToolOutput{}, fmt.Errorf("copy object to temp: %w", copyErr)
	}
	if err := tmp.Sync(); err != nil {
		return hashToolOutput{}, fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return hashToolOutput{}, fmt.Errorf("close temp: %w", err)
	}

	out, err := runHashTool(ctx, tmp.Name())
	if err != nil {
		return hashToolOutput{}, err
	}

	if out.Bytes > math.MaxInt64 || int64(out.Bytes) != copied {
		return hashToolOutput{}, errors.New("hash tool byte count mismatch")
	}
	return out, nil
}

// Native readiness executes the empty-input vector on the active hashing path.
func hashToolReady(ctx context.Context) error {
	var out hashToolOutput
	var err error
	if os.Getenv("SFD_HASH_TOOL") == "" {
		out, err = hashStream(ctx, strings.NewReader(""))
	} else {
		out, err = hashViaTemporaryFile(ctx, strings.NewReader(""))
	}
	if err != nil {
		return err
	}
	if out.Bytes != 0 || out.Hash != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		return errors.New("native readiness vector mismatch")
	}
	return nil
}
