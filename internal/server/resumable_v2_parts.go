package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// The standalone Rorschach multipart protocol uses one fixed-size S3 part per
// upload request. It must never inherit the whole-file HTTP upload envelope.
// For now the live /upload/resumable route remains intentionally HTTP 501.
const (
	r2ChunkBytes     int64 = 32 << 20
	r2MaximumBytes   int64 = 100 << 30
	r2MinS3PartBytes int64 = 5 << 20
	r2MaximumParts   int64 = 10000
)

type r2PartPlan struct {
	Number int
	Offset int64
	Length int64
	Last   bool
}

// r2PlanPart prevents arithmetic overflow, unbounded HTTP bodies, sparse
// offsets, partial non-final S3 parts and accidental acceptance of >100 GiB.
// configuredLimit comes from the deployment policy, not an untrusted header.
func r2PlanPart(total, offset, length, configuredLimit int64) (r2PartPlan, error) {
	if configuredLimit <= 0 || total <= 0 || total > configuredLimit || total > r2MaximumBytes {
		return r2PartPlan{}, errors.New("resumable object size not permitted")
	}
	if offset < 0 || offset >= total || offset%r2ChunkBytes != 0 {
		return r2PartPlan{}, errors.New("resumable offset invalid")
	}
	partNumber := offset/r2ChunkBytes + 1
	if partNumber > r2MaximumParts {
		return r2PartPlan{}, errors.New("resumable part count exceeded")
	}
	remaining := total - offset
	expected := min(remaining, r2ChunkBytes)
	if length != expected || length <= 0 {
		return r2PartPlan{}, errors.New("resumable part length mismatch")
	}
	last := remaining == expected
	if !last && length < r2MinS3PartBytes {
		return r2PartPlan{}, errors.New("resumable intermediate S3 part too small")
	}
	return r2PartPlan{Number: int(partNumber), Offset: offset, Length: length, Last: last}, nil
}

func r2PartCount(total, configuredLimit int64) (int, error) {
	if configuredLimit <= 0 || total <= 0 || total > configuredLimit || total > r2MaximumBytes {
		return 0, errors.New("resumable object size not permitted")
	}
	count := (total-1)/r2ChunkBytes + 1 // subtraction prevents overflow
	if count > r2MaximumParts {
		return 0, errors.New("resumable part count exceeded")
	}
	return int(count), nil
}

// r2ValidatePartDigest only accepts an unambiguous canonical SHA-256 hex
// string. Never put the digest in a signed URL, request log or exception.
func r2ValidatePartDigest(candidate string, actual [sha256.Size]byte) error {
	if !r2CanonicalDigest(candidate) {
		return errors.New("noncanonical part checksum")
	}
	b, _ := hex.DecodeString(candidate)
	if subtle.ConstantTimeCompare(b, actual[:]) != 1 {
		return errors.New("part checksum mismatch")
	}
	return nil
}

func r2CanonicalDigest(candidate string) bool {
	if len(candidate) != sha256.Size*2 || candidate != strings.ToLower(candidate) {
		return false
	}
	_, err := hex.DecodeString(candidate)
	return err == nil
}

// r2ValidateCompletion rejects an incomplete or duplicate multipart ledger.
// ETags are opaque storage validators, NOT SHA-256 checksums.
func r2ValidateCompletion(total, configuredLimit int64, partNumbers []int, etags []string) error {
	required, err := r2PartCount(total, configuredLimit)
	if err != nil {
		return err
	}
	if len(partNumbers) != required || len(etags) != required {
		return errors.New("missing multipart parts")
	}
	for i, number := range partNumbers {
		if number != i+1 || etags[i] == "" || len(etags[i]) > 128 {
			return fmt.Errorf("invalid multipart completion ledger at part %d", i+1)
		}
		for _, c := range etags[i] {
			if c < 0x21 || c > 0x7e || c == '"' || c == '\\' {
				return errors.New("invalid storage part validator")
			}
		}
	}
	return nil
}
