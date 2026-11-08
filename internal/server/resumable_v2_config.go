package server

import (
	"errors"
	"os"
	"strconv"
)

// ResumableConfig defines the opted-in private upload and reservation limits.
type ResumableConfig struct {
	MaxFileBytes     int64
	MaxReservedBytes int64
}

// LoadResumableConfig is off by default and never changes the legacy body limit.
func LoadResumableConfig() (ResumableConfig, error) {
	size, reservation := os.Getenv("SFD_RESUMABLE_MAX_BYTES"), os.Getenv("SFD_RESUMABLE_RESERVED_BYTES")
	if (size == "" || size == "0") && (reservation == "" || reservation == "0") {
		return ResumableConfig{}, nil
	}
	maxFile, errFile := strconv.ParseInt(size, 10, 64)
	maxReserved, errReserved := strconv.ParseInt(reservation, 10, 64)
	if errFile != nil || errReserved != nil || maxFile < 1 || maxFile > r2MaximumBytes ||
		maxReserved < maxFile || maxReserved > 1<<50 || os.Getenv("SFD_HASH_TOOL") != "" {
		return ResumableConfig{}, errors.New("invalid resumable upload capacity or non-bundled hash configuration")
	}
	if mode := os.Getenv("SFD_S3_PRIVACY_MODE"); mode != "" && mode != "bucket-policy" {
		return ResumableConfig{}, errors.New("resumable uploads require bucket-policy storage verification")
	}
	return ResumableConfig{MaxFileBytes: maxFile, MaxReservedBytes: maxReserved}, nil
}
