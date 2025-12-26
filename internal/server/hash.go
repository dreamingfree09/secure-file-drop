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

type hashToolOutput struct {
	Algorithm string `json:"algorithm"`
	Hash      string `json:"hash"`
	Bytes     uint64 `json:"bytes"`
}

func runHashTool(ctx context.Context, filePath string) (hashToolOutput, error) {
	// Ensure we do not hang indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/app/sfd-hash", filePath)
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

func sha256FromMinioObject(ctx context.Context, mc *minio.Client, bucket, objectKey string) (sha256Hex string, sha256Bytes []byte, size uint64, err error) {
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
	defer obj.Close()

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
