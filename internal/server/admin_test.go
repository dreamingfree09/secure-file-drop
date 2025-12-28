package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminListFilesHandler_InvalidMethod(t *testing.T) {
	s := &Server{db: nil, minio: nil}

	req := httptest.NewRequest(http.MethodPost, "/admin/files", nil)
	w := httptest.NewRecorder()

	s.AdminListFilesHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestAdminDeleteFileHandler_InvalidMethod(t *testing.T) {
	s := &Server{db: nil, minio: nil}

	req := httptest.NewRequest(http.MethodGet, "/admin/files/test-id", nil)
	w := httptest.NewRecorder()

	s.AdminDeleteFileHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestAdminDeleteFileHandler_MissingID(t *testing.T) {
	s := &Server{db: nil, minio: nil}

	req := httptest.NewRequest(http.MethodDelete, "/admin/files/", nil)
	w := httptest.NewRecorder()

	s.AdminDeleteFileHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestAdminManualCleanupHandler_InvalidMethod(t *testing.T) {
	s := &Server{db: nil, minio: nil}

	req := httptest.NewRequest(http.MethodGet, "/admin/cleanup", nil)
	w := httptest.NewRecorder()

	s.AdminManualCleanupHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestCleanupResult_JSONSerialization(t *testing.T) {
	result := CleanupResult{DeletedCount: 42}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded CleanupResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.DeletedCount != 42 {
		t.Errorf("expected 42, got %d", decoded.DeletedCount)
	}
}

func TestFileInfo_JSONSerialization(t *testing.T) {
	info := FileInfo{
		ID:          "test-id",
		OrigName:    "test.txt",
		ContentType: "text/plain",
		SizeBytes:   1024,
		Status:      "ready",
		SHA256Hex:   "abc123",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded FileInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "test-id" {
		t.Errorf("expected 'test-id', got %s", decoded.ID)
	}
	if decoded.OrigName != "test.txt" {
		t.Errorf("expected 'test.txt', got %s", decoded.OrigName)
	}
}
