package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func normaliseEndpoint(raw string) (endpoint string, secure bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, fmt.Errorf("empty endpoint")
	}

	// Accept either "minio:9000" or "http://minio:9000" / "https://minio:9000".
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false, err
		}
		if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", false, fmt.Errorf("invalid endpoint")
		}
		if u.Path != "" && u.Path != "/" {
			return "", false, fmt.Errorf("endpoint must not contain a path")
		}
		secure = (u.Scheme == "https")
		return u.Host, secure, nil
	}

	// No scheme provided, treat as host:port (insecure by default for local MinIO).
	return raw, false, nil
}

func privateBucketPolicy(policy string) bool {
	if strings.TrimSpace(policy) == "" {
		return true
	}
	type statement struct {
		Effect       string
		Principal    json.RawMessage
		NotPrincipal json.RawMessage
	}
	var document struct{ Statement json.RawMessage }
	if json.Unmarshal([]byte(policy), &document) != nil {
		return false
	}
	var statements []statement
	if json.Unmarshal(document.Statement, &statements) != nil {
		var single statement
		if json.Unmarshal(document.Statement, &single) != nil {
			return false
		}
		statements = []statement{single}
	}
	if len(statements) == 0 {
		return false
	}
	var named func(any) bool
	named = func(value any) bool {
		switch v := value.(type) {
		case string:
			return v != "" && !strings.Contains(v, "*")
		case []any:
			if len(v) == 0 {
				return false
			}
			for _, item := range v {
				if !named(item) {
					return false
				}
			}
			return true
		case map[string]any:
			if len(v) == 0 {
				return false
			}
			for key, item := range v {
				if key != "AWS" && key != "Service" && key != "CanonicalUser" {
					return false
				}
				if !named(item) {
					return false
				}
			}
			return true
		default:
			return false
		}
	}
	for _, s := range statements {
		if s.Effect == "Deny" {
			continue
		}
		if s.Effect != "Allow" || len(s.NotPrincipal) > 0 {
			return false
		}
		var p any
		if json.Unmarshal(s.Principal, &p) != nil || !named(p) {
			return false
		}
	}
	return true
}
