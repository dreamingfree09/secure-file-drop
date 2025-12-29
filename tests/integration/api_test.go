//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestAPIWorkflow tests the complete file upload and download workflow
func TestAPIWorkflow(t *testing.T) {
	// Setup test server
	srv := setupTestServer(t)
	defer srv.Close()

	client := &http.Client{Timeout: 30 * time.Second}

	// Test 1: Health check
	t.Run("Health Check", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/ready")
		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode health response: %v", err)
		}

		if status, ok := result["status"].(string); !ok || status != "ok" {
			t.Errorf("Expected status 'ok', got %v", result["status"])
		}
	})

	// Test 2: User registration
	var verifyToken string
	t.Run("User Registration", func(t *testing.T) {
		payload := map[string]string{
			"email":    "test@example.com",
			"username": "testuser",
			"password": "TestPass123",
		}
		body, _ := json.Marshal(payload)

		resp, err := client.Post(srv.URL+"/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Registration failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 201, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		// In a real test with email, you'd extract the token from the email
		// For now, we'll skip verification and use admin login
	})

	// Test 3: Admin login
	var sessionCookie *http.Cookie
	t.Run("Admin Login", func(t *testing.T) {
		payload := map[string]string{
			"username": os.Getenv("SFD_ADMIN_USER"),
			"password": "admin", // This should match what's in test env
		}
		body, _ := json.Marshal(payload)

		resp, err := client.Post(srv.URL+"/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		// Extract session cookie
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "sfd_session" {
				sessionCookie = cookie
				break
			}
		}

		if sessionCookie == nil {
			t.Fatal("No session cookie received")
		}
	})

	// Test 4: Get user quota
	t.Run("Get Quota", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/quota", nil)
		req.AddCookie(sessionCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Quota request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var quota map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&quota); err != nil {
			t.Fatalf("Failed to decode quota response: %v", err)
		}

		if _, ok := quota["storage_used_bytes"]; !ok {
			t.Error("Missing storage_used_bytes in quota response")
		}
	})

	// Test 5: Create file metadata
	var fileID string
	t.Run("Create File Metadata", func(t *testing.T) {
		payload := map[string]interface{}{
			"orig_name":    "test.txt",
			"content_type": "text/plain",
			"size_bytes":   11,
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest("POST", srv.URL+"/files", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(sessionCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Create file failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode file response: %v", err)
		}

		fileID, _ = result["id"].(string)
		if fileID == "" {
			t.Fatal("No file ID returned")
		}
	})

	// Test 6: Upload file
	t.Run("Upload File", func(t *testing.T) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)

		// Create file part
		part, err := writer.CreateFormFile("file", "test.txt")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}

		_, err = part.Write([]byte("Hello World"))
		if err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}

		writer.Close()

		req, _ := http.NewRequest("POST", srv.URL+"/upload?id="+fileID, &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.AddCookie(sessionCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		// Wait for hashing to complete
		time.Sleep(2 * time.Second)
	})

	// Test 7: Create download link
	var downloadURL string
	t.Run("Create Download Link", func(t *testing.T) {
		payload := map[string]interface{}{
			"file_id":          fileID,
			"expires_in_hours": 24,
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest("POST", srv.URL+"/links", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(sessionCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Create link failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode link response: %v", err)
		}

		downloadURL, _ = result["url"].(string)
		if downloadURL == "" {
			t.Fatal("No download URL returned")
		}
	})

	// Test 8: Download file
	t.Run("Download File", func(t *testing.T) {
		resp, err := client.Get(downloadURL)
		if err != nil {
			t.Fatalf("Download failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read download content: %v", err)
		}

		if string(content) != "Hello World" {
			t.Errorf("Expected 'Hello World', got '%s'", string(content))
		}
	})

	// Test 9: Get metrics (admin only)
	t.Run("Get Metrics", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/metrics", nil)
		req.AddCookie(sessionCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Metrics request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var metrics map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
			t.Fatalf("Failed to decode metrics response: %v", err)
		}

		if _, ok := metrics["uploads_total"]; !ok {
			t.Error("Missing uploads_total in metrics")
		}
	})

	// Test 10: Delete file
	t.Run("Delete File", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", srv.URL+"/user/files/"+fileID, nil)
		req.AddCookie(sessionCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 204, got %d: %s", resp.StatusCode, string(bodyBytes))
		}
	})

	// Test 11: Logout
	t.Run("Logout", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/logout", nil)
		req.AddCookie(sessionCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Logout failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

// setupTestServer initializes a test server with all dependencies
func setupTestServer(t *testing.T) *httptest.Server {
	// This would ideally use the actual server setup
	// For now, return a mock that delegates to the real handler
	// In production, you'd configure a real server instance here

	// Set required environment variables if not already set
	if os.Getenv("SFD_SESSION_SECRET") == "" {
		os.Setenv("SFD_SESSION_SECRET", "test-session-secret-min-32-chars-long")
	}
	if os.Getenv("SFD_DOWNLOAD_SECRET") == "" {
		os.Setenv("SFD_DOWNLOAD_SECRET", "test-download-secret-min-32-chars")
	}
	if os.Getenv("SFD_ADMIN_USER") == "" {
		os.Setenv("SFD_ADMIN_USER", "admin")
	}

	// Create test server
	// Note: This requires refactoring server.go to expose a NewServer() function
	// For now, we'll create a placeholder
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprintf(w, "Test server not fully implemented yet")
	})

	return httptest.NewServer(handler)
}
