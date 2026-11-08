package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/minio/minio-go/v7"
)

// r2ObjectStore is the storage-side primitive for a future durable upload
// session. It is deliberately not wired to HTTP until transactional ownership,
// part-record persistence, crash reconciliation and signed-byte acceptance pass.
type r2ObjectStore struct {
	client *minio.Client
	bucket string
}

func (s r2ObjectStore) core() (minio.Core, error) {
	if s.client == nil || s.bucket == "" {
		return minio.Core{}, errors.New("private object storage unavailable")
	}
	return minio.Core{Client: s.client}, nil
}

func (s r2ObjectStore) begin(ctx context.Context, key string) (string, error) {
	c, err := s.core()
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", errors.New("object key missing")
	}
	return c.NewMultipartUpload(ctx, s.bucket, key, minio.PutObjectOptions{ContentType: "application/octet-stream"})
}

// putPart streams one exact-sized, already validated part to private S3 and
// returns the opaque ETag for durable PostgreSQL recording. The caller must
// hold the upload-session lease and enforce owner, offset, reservation, request
// deadline and at-most-32 MiB Content-Length before calling this method.
func (s r2ObjectStore) putPart(ctx context.Context, key, uploadID string, plan r2PartPlan, input io.Reader, expectedSHA256 string) (minio.CompletePart, error) {
	c, err := s.core()
	if err != nil {
		return minio.CompletePart{}, err
	}
	if key == "" || uploadID == "" || input == nil || plan.Number <= 0 ||
		plan.Number > int(r2MaximumParts) || plan.Length <= 0 || plan.Length > r2ChunkBytes ||
		plan.Offset < 0 || plan.Offset%r2ChunkBytes != 0 ||
		int64(plan.Number) != plan.Offset/r2ChunkBytes+1 {
		return minio.CompletePart{}, errors.New("multipart part parameters invalid")
	}
	if len(expectedSHA256) != 64 {
		return minio.CompletePart{}, errors.New("part checksum required")
	}
	if _, err := hex.DecodeString(expectedSHA256); err != nil {
		return minio.CompletePart{}, errors.New("part checksum invalid")
	}
	limited := &io.LimitedReader{R: input, N: plan.Length}
	hasher := sha256.New()
	stream := io.TeeReader(limited, hasher)
	received, err := c.PutObjectPart(ctx, s.bucket, key, uploadID, plan.Number, stream, plan.Length, minio.PutObjectPartOptions{})
	if err != nil {
		return minio.CompletePart{}, err
	}
	if limited.N != 0 || received.PartNumber != plan.Number || received.ETag == "" {
		return minio.CompletePart{}, errors.New("storage did not acknowledge complete part")
	}
	var extra [1]byte
	n, endErr := input.Read(extra[:])
	if n != 0 || endErr != io.EOF {
		return minio.CompletePart{}, errors.New("extra bytes or incomplete part body")
	}
	var actual [sha256.Size]byte
	copy(actual[:], hasher.Sum(nil))
	if err := r2ValidatePartDigest(expectedSHA256, actual); err != nil {
		return minio.CompletePart{}, err
	}
	return minio.CompletePart{PartNumber: received.PartNumber, ETag: received.ETag}, nil
}

// complete only addresses the exact session-owned key; its caller must first
// prove durable per-part ETags, exact offsets, and a locked, owned file record.
// An ambiguous S3 COMMIT must be reconciled against persisted state and hashes,
// never compensated by blind object deletion.
func (s r2ObjectStore) complete(ctx context.Context, key, uploadID string, total, configuredLimit int64, parts []minio.CompletePart) error {
	c, err := s.core()
	if err != nil {
		return err
	}
	if key == "" || uploadID == "" {
		return errors.New("incomplete multipart identity")
	}
	numbers, etags := make([]int, len(parts)), make([]string, len(parts))
	for i, part := range parts {
		numbers[i], etags[i] = part.PartNumber, part.ETag
	}
	if err := r2ValidateCompletion(total, configuredLimit, numbers, etags); err != nil {
		return err
	}
	if _, err := c.CompleteMultipartUpload(ctx, s.bucket, key, uploadID, parts, minio.PutObjectOptions{}); err != nil {
		return err
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return err
	}
	if info.Size != total {
		return errors.New("completed object size differs from reserved file")
	}
	return nil
}

func (s r2ObjectStore) abort(ctx context.Context, key, uploadID string) error {
	c, err := s.core()
	if err != nil {
		return err
	}
	if key == "" || uploadID == "" {
		return errors.New("incomplete multipart identity")
	}
	// Only caller-verified-owned incomplete sessions may reach abort.
	return c.AbortMultipartUpload(ctx, s.bucket, key, uploadID)
}
