// Command probe performs bounded, credentialed, read-only private storage checks.
// The production command contains no bucket/object write or delete operations.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const maxObject = int64(64 * 1024 * 1024)
const maxTotal = int64(1024 * 1024 * 1024)

type config struct{ Endpoint, AccessKey, SecretKey, Bucket string }
type entry struct {
	ID       string `json:"id"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Modified string `json:"modified"`
}
type snapshot struct {
	PolicySHA256 string  `json:"policy_sha256"`
	OK           bool    `json:"ok"`
	Objects      []entry `json:"objects"`
	Total        int64   `json:"total_bytes"`
	Stage        string  `json:"stage,omitempty"`
}

func hashName(bucket, key string) string {
	h := sha256.Sum256([]byte(bucket + "\x00" + key))
	return hex.EncodeToString(h[:])
}
func readHash(r io.Reader, expected int64) (string, error) {
	if expected < 0 || expected > maxObject {
		return "", errors.New("object-bound")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(r, expected+1))
	if err != nil || n != expected {
		return "", errors.New("object-integrity")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func inspect(ctx context.Context, c config) (snapshot, error) {
	out := snapshot{Stage: "endpoint", Objects: []entry{}}
	endpoint, secure, err := normaliseEndpoint(c.Endpoint)
	if err != nil {
		return out, err
	}
	if c.AccessKey == "" || c.SecretKey == "" || c.Bucket == "" {
		return out, errors.New("configuration")
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Secure: secure})
	if err != nil {
		return out, err
	}
	out.Stage = "bucket"
	exists, err := client.BucketExists(ctx, c.Bucket)
	if err != nil || !exists {
		return out, errors.New("bucket")
	}
	policy, err := client.GetBucketPolicy(ctx, c.Bucket)
	if err != nil || !privateBucketPolicy(policy) {
		return out, errors.New("private-policy")
	}
	pol := sha256.Sum256([]byte(policy))
	out.PolicySHA256 = hex.EncodeToString(pol[:])
	out.Stage = "objects"
	seen := map[string]bool{}
	for obj := range client.ListObjects(ctx, c.Bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil || len(out.Objects) >= 512 || obj.Size < 0 || obj.Size > maxObject || out.Total+obj.Size > maxTotal {
			return out, errors.New("snapshot-bound")
		}
		id := hashName(c.Bucket, obj.Key)
		if seen[id] {
			return out, errors.New("duplicate-object")
		}
		seen[id] = true
		r, err := client.GetObject(ctx, c.Bucket, obj.Key, minio.GetObjectOptions{})
		if err != nil {
			return out, err
		}
		hash, err := readHash(r, obj.Size)
		closeErr := r.Close()
		if err != nil || closeErr != nil {
			return out, errors.New("object-read")
		}
		out.Objects = append(out.Objects, entry{ID: id, Bytes: obj.Size, SHA256: hash, Modified: obj.LastModified.UTC().Format(time.RFC3339Nano)})
		out.Total += obj.Size
	}
	sort.Slice(out.Objects, func(i, j int) bool { return out.Objects[i].ID < out.Objects[j].ID })
	out.OK = true
	out.Stage = "complete"
	return out, nil
}
func main() {
	var c config
	d := json.NewDecoder(io.LimitReader(os.Stdin, 16385))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		_, _ = os.Stdout.WriteString("{\"ok\":false,\"stage\":\"stdin\"}\n")
		os.Exit(1)
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		_, _ = os.Stdout.WriteString("{\"ok\":false,\"stage\":\"stdin\"}\n")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, err := inspect(ctx, c)
	c.SecretKey = ""
	c.AccessKey = ""
	if err != nil {
		out.OK = false
		out.Objects = nil
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
	if !out.OK {
		os.Exit(1)
	}
}
