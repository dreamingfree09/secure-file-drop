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
	if len(os.Args) != 4 && (len(os.Args) != 5 || os.Args[4] != "generated") {
		return fmt.Errorf("expected owned loopback endpoint, bucket and run")
	}
	addr, bucket, run := os.Args[1], os.Args[2], os.Args[3]
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" || !strings.HasPrefix(run, "sfd-quality-") || bucket != run+"-private" {
		return fmt.Errorf("unexpected disposable target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	rootKey, rootSecret := "sfdtest", "SyntheticPass123456"
	keys := []string{"sfdapp", "sfdreader", "sfdoutsider"}
	secrets := []string{rootSecret, rootSecret, rootSecret}
	generated := len(os.Args) == 5
	if generated {
		if os.Getenv("SFD_IAM_RUN") != run {
			return fmt.Errorf("missing synthetic ownership token")
		}
		rootKey, rootSecret = os.Getenv("SFD_IAM_ROOT_KEY"), os.Getenv("SFD_IAM_ROOT_SECRET")
		if !strings.HasPrefix(rootKey, "root_") || len(rootSecret) < 32 {
			return fmt.Errorf("invalid generated root identity")
		}
		seen := map[string]bool{rootSecret: true}
		for i, role := range []string{"APP", "READER", "OUTSIDER"} {
			keys[i], secrets[i] = os.Getenv("SFD_IAM_"+role+"_KEY"), os.Getenv("SFD_IAM_"+role+"_SECRET")
			if !strings.HasPrefix(keys[i], strings.ToLower(role)+"_") || len(secrets[i]) < 32 || seen[secrets[i]] || keys[i] == rootKey {
				return fmt.Errorf("invalid or shared synthetic identity")
			}
			seen[secrets[i]] = true
		}
	}
	admin, err := madmin.New(addr, rootKey, rootSecret, false)
	if err != nil {
		return err
	}
	for i, user := range keys {
		target := bucket
		actions := []string{"s3:GetObject", "s3:GetBucketLocation", "s3:ListBucket"}
		if i != 1 {
			actions = append(actions, "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts", "s3:ListBucketMultipartUploads", "s3:GetBucketPolicy")
		}
		if i == 2 {
			target = run + "-other"
		}
		policy, err := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::" + target, "arn:aws:s3:::" + target + "/*"}}}})
		if err != nil {
			return err
		}
		if generated && i == 0 {
			raw, e := os.ReadFile("../rustfs-app-policy.json")
			if e != nil {
				return e
			}
			policy = []byte(strings.ReplaceAll(string(raw), "${BUCKET}", bucket))
			if !json.Valid(policy) || strings.Contains(string(policy), "${") {
				return fmt.Errorf("invalid scoped fixture policy")
			}
		}
		if err := admin.AddUser(ctx, user, secrets[i]); err != nil {
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
