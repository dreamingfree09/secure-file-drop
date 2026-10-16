// Disposable signed-administration fixture; never points at production.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/minio/madmin-go/v3"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	if err := setup(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func setup() error {
	if len(os.Args) != 4 {
		return fmt.Errorf("expected owned loopback endpoint, bucket and run")
	}
	addr, bucket, run := os.Args[1], os.Args[2], os.Args[3]
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" || !strings.HasPrefix(run, "sfd-quality-") || bucket != run+"-private" {
		return fmt.Errorf("unexpected disposable target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	admin, err := madmin.New(addr, "sfdtest", "SyntheticPass123456", false)
	if err != nil {
		return err
	}
	for _, user := range []string{"sfdapp", "sfdreader", "sfdoutsider"} {
		target := bucket
		actions := []string{"s3:GetObject", "s3:GetBucketLocation", "s3:ListBucket"}
		if user != "sfdreader" {
			actions = append(actions, "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts", "s3:ListBucketMultipartUploads", "s3:GetBucketPolicy")
		}
		if user == "sfdoutsider" {
			target = run + "-other"
		}
		policy, err := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::" + target, "arn:aws:s3:::" + target + "/*"}}}})
		if err != nil {
			return err
		}
		if err := admin.AddUser(ctx, user, "SyntheticPass123456"); err != nil {
			return fmt.Errorf("create %s: %w", user, err)
		}
		if err := admin.AddCannedPolicy(ctx, run+"-"+user, policy); err != nil {
			return fmt.Errorf("policy %s: %w", user, err)
		}
		if err := admin.SetPolicy(ctx, run+"-"+user, user, false); err != nil {
			return fmt.Errorf("attach %s: %w", user, err)
		}
	}
	return nil
}
