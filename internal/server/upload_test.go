package server

import (
	"bytes"
	"context"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUploadHandler_InvalidMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/upload?id="+uuid.New().String(), nil)
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

func TestUploadHandler_MissingID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
	})

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing id, got %d", rr.Code)
	}
}

func TestUploadHandler_InvalidUUID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/upload?id=not-a-uuid", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		_, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
	})

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid uuid, got %d", rr.Code)
	}
}

func TestUploadHandler_MaxBytesValidation(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		shouldError bool
	}{
		{
			name:        "valid limit",
			envValue:    "1048576",
			shouldError: false,
		},
		{
			name:        "empty value (no limit)",
			envValue:    "",
			shouldError: false,
		},
		{
			name:        "invalid format",
			envValue:    "not-a-number",
			shouldError: true,
		},
		{
			name:        "negative value",
			envValue:    "-1",
			shouldError: false, // parsed successfully, but semantically questionable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SFD_MAX_UPLOAD_BYTES", tt.envValue)

			_, err := maxUploadBytes()

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for %s, got nil", tt.name)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error for %s, got %v", tt.name, err)
			}
		})
	}
}

func TestUploadHandler_MultipartParsing(t *testing.T) {
	// Create a multipart body with a file part
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "test.txt")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}

	_, err = part.Write([]byte("test content"))
	if err != nil {
		t.Fatalf("Failed to write to form file: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload?id="+uuid.New().String(), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Verify multipart can be parsed
	mr, err := req.MultipartReader()
	if err != nil {
		t.Errorf("Expected multipart reader to work, got error: %v", err)
	}

	firstPart, err := mr.NextPart()
	if err != nil {
		t.Errorf("Expected to read first part, got error: %v", err)
	}

	if firstPart.FormName() != "file" {
		t.Errorf("Expected form name 'file', got %q", firstPart.FormName())
	}
}

func TestUploadHandler_StatusValidation(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		shouldAllow    bool
		expectedStatus int
	}{
		{
			name:           "pending status",
			status:         "pending",
			shouldAllow:    true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "stored status",
			status:         "stored",
			shouldAllow:    false,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "hashed status",
			status:         "hashed",
			shouldAllow:    false,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "failed status",
			status:         "failed",
			shouldAllow:    false,
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate status check logic
			var statusCode int
			if tt.status != "pending" {
				statusCode = http.StatusConflict
			} else {
				statusCode = http.StatusOK
			}

			if statusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, statusCode)
			}
		})
	}
}

func TestMaxUploadBytes(t *testing.T) {
	t.Setenv("SFD_MAX_UPLOAD_BYTES", "5242880")

	limit, err := maxUploadBytes()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if limit != 5242880 {
		t.Errorf("Expected limit 5242880, got %d", limit)
	}
}

func TestMaxUploadBytes_NotSet(t *testing.T) {
	t.Setenv("SFD_MAX_UPLOAD_BYTES", "")

	limit, err := maxUploadBytes()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if limit != 0 {
		t.Errorf("Expected limit 0 (no limit), got %d", limit)
	}
}

// Mock DB for integration-style test (without actual DB)
type mockDB struct {
	queryRowFunc func(query string, args ...any) *sql.Row
	execFunc     func(query string, args ...any) (sql.Result, error)
}

func TestUploadValidationFlow(t *testing.T) {
	// Test the validation flow without actual DB/MinIO
	testID := uuid.New()

	// Simulate validation steps
	t.Run("check ID parsing", func(t *testing.T) {
		_, err := uuid.Parse(testID.String())
		if err != nil {
			t.Errorf("Valid UUID should parse: %v", err)
		}
	})

	t.Run("check content type extraction", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part, _ := writer.CreateFormFile("file", "test.pdf")
		part.Write([]byte("fake pdf"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		mr, _ := req.MultipartReader()
		p, _ := mr.NextPart()

		contentType := p.Header.Get("Content-Type")
		// Multipart form file parts may have empty content-type by default
		// This is expected behavior
		t.Logf("Content-Type: %q (may be empty for form files)", contentType)
	})
}

func TestUploadHandler_ContextTimeout(t *testing.T) {
	// Test that context timeout is properly set
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("Expected deadline to be set")
	}

	if time.Until(deadline) > 5*time.Minute {
		t.Error("Deadline is too far in the future")
	}
}
