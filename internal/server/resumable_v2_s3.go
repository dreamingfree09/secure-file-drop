package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"

	"github.com/minio/minio-go/v7"
)

var errR2Integrity = errors.New("stored object failed integrity verification")

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

// Only exact matches on a server-generated, session-owned key are eligible.
// A malformed or truncated response cannot authorize a new write or an abort.
func (s r2ObjectStore) uploads(ctx context.Context, key string) ([]string, error) {
	c, err := s.core()
	if err != nil || key == "" {
		return nil, errors.New("multipart list unavailable")
	}
	list, err := c.ListMultipartUploads(ctx, s.bucket, key, "", "", "", 100)
	if err != nil || list.IsTruncated {
		return nil, errors.New("multipart list unavailable")
	}
	var ids []string
	for _, upload := range list.Uploads {
		if upload.Key != key {
			continue
		}
		if upload.UploadID == "" || len(upload.UploadID) > 2048 {
			return nil, errors.New("invalid multipart identity")
		}
		ids = append(ids, upload.UploadID)
	}
	return ids, nil
}

func (s r2ObjectStore) begin(ctx context.Context, key string) (string, error) {
	ids, err := s.uploads(ctx, key)
	if err != nil {
		return "", err
	}
	if len(ids) > 0 {
		return ids[0], nil
	}
	c := minio.Core{Client: s.client}
	id, err := c.NewMultipartUpload(ctx, s.bucket, key, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil || id == "" || len(id) > 2048 {
		return "", errors.New("multipart initiation unavailable")
	}
	return id, nil
}

func (s r2ObjectStore) putPart(ctx context.Context, key, uploadID string, plan r2PartPlan, data []byte, digest string) (minio.CompletePart, error) {
	c, err := s.core()
	if err != nil {
		return minio.CompletePart{}, err
	}
	if key == "" || uploadID == "" || plan.Number <= 0 || plan.Number > int(r2MaximumParts) ||
		plan.Length <= 0 || plan.Length > r2ChunkBytes || int64(len(data)) != plan.Length ||
		plan.Offset < 0 || plan.Offset%r2ChunkBytes != 0 || int64(plan.Number) != plan.Offset/r2ChunkBytes+1 {
		return minio.CompletePart{}, errors.New("multipart part parameters invalid")
	}
	if err := r2ValidatePartDigest(digest, sha256.Sum256(data)); err != nil {
		return minio.CompletePart{}, err
	}
	result, err := c.PutObjectPart(ctx, s.bucket, key, uploadID, plan.Number, bytes.NewReader(data), plan.Length,
		minio.PutObjectPartOptions{Sha256Hex: digest})
	if err != nil {
		return minio.CompletePart{}, err
	}
	if result.PartNumber != plan.Number || downloadETag(result.ETag) == "" {
		return minio.CompletePart{}, errors.New("invalid storage part acknowledgement")
	}
	return minio.CompletePart{PartNumber: result.PartNumber, ETag: result.ETag}, nil
}

func r2ObjectMissing(err error) bool {
	return minio.ToErrorResponse(err).Code == "NoSuchKey"
}

// A missing completion receipt leaves finalizing state intact. An exact-key
// object readback is only a candidate until its bytes are verified.
func (s r2ObjectStore) complete(ctx context.Context, key, uploadID string, total, limit int64, parts []minio.CompletePart) (minio.ObjectInfo, error) {
	c, err := s.core()
	if err != nil || key == "" || uploadID == "" {
		return minio.ObjectInfo{}, errors.New("multipart completion unavailable")
	}
	numbers, etags := make([]int, len(parts)), make([]string, len(parts))
	for i, part := range parts {
		numbers[i], etags[i] = part.PartNumber, part.ETag
	}
	if err := r2ValidateCompletion(total, limit, numbers, etags); err != nil {
		return minio.ObjectInfo{}, err
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if r2ObjectMissing(err) {
		_, completeErr := c.CompleteMultipartUpload(ctx, s.bucket, key, uploadID, parts, minio.PutObjectOptions{})
		info, err = s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
		if err != nil {
			return minio.ObjectInfo{}, errors.Join(completeErr, err)
		}
	}
	if err != nil {
		return minio.ObjectInfo{}, err
	}
	if info.Size != total || downloadETag(info.ETag) == "" {
		return minio.ObjectInfo{}, errR2Integrity
	}
	return info, nil
}

func (s r2ObjectStore) read(ctx context.Context, key string, info minio.ObjectInfo) (io.ReadCloser, error) {
	opts := minio.GetObjectOptions{}
	if err := opts.SetMatchETag(info.ETag); err != nil {
		return nil, err
	}
	body, received, _, err := (minio.Core{Client: s.client}).GetObject(ctx, s.bucket, key, opts)
	if err != nil {
		return nil, err
	}
	if received.Size != info.Size || received.ETag != info.ETag {
		_ = body.Close()
		return nil, errR2Integrity
	}
	return body, nil
}

func (s r2ObjectStore) abort(ctx context.Context, key, uploadID string) error {
	c, err := s.core()
	if err != nil || key == "" || uploadID == "" {
		return errors.New("multipart abort unavailable")
	}
	err = c.AbortMultipartUpload(ctx, s.bucket, key, uploadID)
	if minio.ToErrorResponse(err).Code == "NoSuchUpload" {
		return nil
	}
	return err
}
