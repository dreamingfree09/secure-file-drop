// Fixture-only minimal S3 client, not an administration or bypass tool.
package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type alias struct {
	Endpoint string
	Key      string
	Secret   string
}

func run() error {
	args := os.Args[1:]
	path := "/tmp/sfd-stage1-alias.json"
	if len(args) == 6 && args[0] == "alias" && args[1] == "set" && args[2] == "fixture" {
		u, e := url.Parse(args[3])
		if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() != "9000" {
			return os.ErrPermission
		}
		c, e := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(args[4], args[5], ""), Secure: false})
		if e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, e = c.ListBuckets(ctx); e != nil {
			return e
		}
		raw, e := json.Marshal(alias{u.Host, args[4], args[5]})
		if e != nil {
			return e
		}
		return os.WriteFile(path, raw, 0600)
	}
	if len(args) != 2 || (args[0] != "mb" && args[0] != "stat") || !strings.HasPrefix(args[1], "fixture/sfd-auth-") {
		return os.ErrPermission
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	var a alias
	if e = json.Unmarshal(raw, &a); e != nil {
		return e
	}
	c, e := minio.New(a.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(a.Key, a.Secret, ""), Secure: false})
	if e != nil {
		return e
	}
	target := strings.TrimPrefix(args[1], "fixture/")
	parts := strings.SplitN(target, "/", 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if args[0] == "mb" {
		if len(parts) != 1 {
			return os.ErrPermission
		}
		return c.MakeBucket(ctx, parts[0], minio.MakeBucketOptions{})
	}
	if len(parts) != 2 {
		return os.ErrPermission
	}
	_, e = c.StatObject(ctx, parts[0], parts[1], minio.StatObjectOptions{})
	return e
}

func main() {
	if run() != nil {
		os.Exit(1)
	}
}
