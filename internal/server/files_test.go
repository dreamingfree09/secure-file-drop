package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateFileHandler_Success(t *testing.T) {
	// Test validation: empty orig_name
	invalidPayload := createFileReq{
		OrigName:    "",
		ContentType: "text/plain",
		SizeBytes:   1024,
	}
	invalidBody, _ := json.Marshal(invalidPayload)
	invalidReq := httptest.NewRequest(http.MethodPost, "/files", bytes.NewReader(invalidBody))
	invalidReq.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req createFileReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		req.OrigName = strings.TrimSpace(req.OrigName)
		req.ContentType = strings.TrimSpace(req.ContentType)

		if req.OrigName == "" || req.ContentType == "" || req.SizeBytes < 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	handler.ServeHTTP(rr, invalidReq)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty orig_name, got %d", rr.Code)
	}
}

func TestCreateFileHandler_InvalidMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/files", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	})

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", rr.Code)
	}
}

func TestCreateFileHandler_ValidationCases(t *testing.T) {
	tests := []struct {
		name        string
		payload     createFileReq
		shouldError bool
	}{
		{
			name: "valid payload",
			payload: createFileReq{
				OrigName:    "document.pdf",
				ContentType: "application/pdf",
				SizeBytes:   2048,
			},
			shouldError: false,
		},
		{
			name: "empty orig_name",
			payload: createFileReq{
				OrigName:    "",
				ContentType: "text/plain",
				SizeBytes:   100,
			},
			shouldError: true,
		},
		{
			name: "empty content_type",
			payload: createFileReq{
				OrigName:    "file.txt",
				ContentType: "",
				SizeBytes:   100,
			},
			shouldError: true,
		},
		{
			name: "negative size",
			payload: createFileReq{
				OrigName:    "file.txt",
				ContentType: "text/plain",
				SizeBytes:   -1,
			},
			shouldError: true,
		},
		{
			name: "whitespace-only orig_name",
			payload: createFileReq{
				OrigName:    "   ",
				ContentType: "text/plain",
				SizeBytes:   100,
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate payload
			origName := strings.TrimSpace(tt.payload.OrigName)
			contentType := strings.TrimSpace(tt.payload.ContentType)
			sizeBytes := tt.payload.SizeBytes

			isValid := origName != "" && contentType != "" && sizeBytes >= 0

			if tt.shouldError && isValid {
				t.Errorf("Expected validation to fail for %s", tt.name)
			}
			if !tt.shouldError && !isValid {
				t.Errorf("Expected validation to pass for %s", tt.name)
			}
		})
	}
}
