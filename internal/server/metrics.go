package server

import (
	"sync"
	"time"
)

// Metrics holds application metrics
type Metrics struct {
	mu sync.RWMutex

	// Upload metrics
	uploadsTotal        int64
	uploadBytesTotal    int64
	uploadErrorsTotal   int64
	uploadDurationTotal time.Duration

	// Download metrics
	downloadsTotal        int64
	downloadBytesTotal    int64
	downloadErrorsTotal   int64
	downloadDurationTotal time.Duration

	// Auth metrics
	loginAttemptsTotal  int64
	loginSuccessTotal   int64
	loginFailuresTotal  int64
	activeSessionsTotal int64

	// File lifecycle metrics
	filesPendingTotal int64
	filesStoredTotal  int64
	filesHashedTotal  int64
	filesReadyTotal   int64
	filesFailedTotal  int64

	// System metrics
	requestsTotal    int64
	requestErrors5xx int64
	requestErrors4xx int64
}

var globalMetrics = &Metrics{}

// GetMetrics returns the global metrics instance
func GetMetrics() *Metrics {
	return globalMetrics
}

// RecordUpload records a successful upload
func (m *Metrics) RecordUpload(bytes int64, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploadsTotal++
	m.uploadBytesTotal += bytes
	m.uploadDurationTotal += duration
}

// RecordUploadError records an upload error
func (m *Metrics) RecordUploadError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploadErrorsTotal++
}

// RecordDownload records a successful download
func (m *Metrics) RecordDownload(bytes int64, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.downloadsTotal++
	m.downloadBytesTotal += bytes
	m.downloadDurationTotal += duration
}

// RecordDownloadError records a download error
func (m *Metrics) RecordDownloadError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.downloadErrorsTotal++
}

// RecordLoginAttempt records a login attempt
func (m *Metrics) RecordLoginAttempt(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loginAttemptsTotal++
	if success {
		m.loginSuccessTotal++
	} else {
		m.loginFailuresTotal++
	}
}

// SetActiveSessions sets the current active sessions count
func (m *Metrics) SetActiveSessions(count int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeSessionsTotal = count
}

// RecordFileStateTransition records a file state change
func (m *Metrics) RecordFileStateTransition(newState string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch newState {
	case "pending":
		m.filesPendingTotal++
	case "stored":
		m.filesStoredTotal++
	case "hashed":
		m.filesHashedTotal++
	case "ready":
		m.filesReadyTotal++
	case "failed":
		m.filesFailedTotal++
	}
}

// RecordRequest records an HTTP request
func (m *Metrics) RecordRequest(statusCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsTotal++

	if statusCode >= 500 {
		m.requestErrors5xx++
	} else if statusCode >= 400 {
		m.requestErrors4xx++
	}
}

// Snapshot returns a snapshot of current metrics
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MetricsSnapshot{
		UploadsTotal:          m.uploadsTotal,
		UploadBytesTotal:      m.uploadBytesTotal,
		UploadErrorsTotal:     m.uploadErrorsTotal,
		UploadAvgDurationMs:   avgDuration(m.uploadDurationTotal, m.uploadsTotal),
		DownloadsTotal:        m.downloadsTotal,
		DownloadBytesTotal:    m.downloadBytesTotal,
		DownloadErrorsTotal:   m.downloadErrorsTotal,
		DownloadAvgDurationMs: avgDuration(m.downloadDurationTotal, m.downloadsTotal),
		LoginAttemptsTotal:    m.loginAttemptsTotal,
		LoginSuccessTotal:     m.loginSuccessTotal,
		LoginFailuresTotal:    m.loginFailuresTotal,
		ActiveSessionsTotal:   m.activeSessionsTotal,
		FilesPendingTotal:     m.filesPendingTotal,
		FilesStoredTotal:      m.filesStoredTotal,
		FilesHashedTotal:      m.filesHashedTotal,
		FilesReadyTotal:       m.filesReadyTotal,
		FilesFailedTotal:      m.filesFailedTotal,
		RequestsTotal:         m.requestsTotal,
		RequestErrors5xx:      m.requestErrors5xx,
		RequestErrors4xx:      m.requestErrors4xx,
	}
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	// Upload metrics
	UploadsTotal        int64   `json:"uploads_total"`
	UploadBytesTotal    int64   `json:"upload_bytes_total"`
	UploadErrorsTotal   int64   `json:"upload_errors_total"`
	UploadAvgDurationMs float64 `json:"upload_avg_duration_ms"`

	// Download metrics
	DownloadsTotal        int64   `json:"downloads_total"`
	DownloadBytesTotal    int64   `json:"download_bytes_total"`
	DownloadErrorsTotal   int64   `json:"download_errors_total"`
	DownloadAvgDurationMs float64 `json:"download_avg_duration_ms"`

	// Auth metrics
	LoginAttemptsTotal  int64 `json:"login_attempts_total"`
	LoginSuccessTotal   int64 `json:"login_success_total"`
	LoginFailuresTotal  int64 `json:"login_failures_total"`
	ActiveSessionsTotal int64 `json:"active_sessions_total"`

	// File lifecycle metrics
	FilesPendingTotal int64 `json:"files_pending_total"`
	FilesStoredTotal  int64 `json:"files_stored_total"`
	FilesHashedTotal  int64 `json:"files_hashed_total"`
	FilesReadyTotal   int64 `json:"files_ready_total"`
	FilesFailedTotal  int64 `json:"files_failed_total"`

	// System metrics
	RequestsTotal    int64 `json:"requests_total"`
	RequestErrors5xx int64 `json:"request_errors_5xx"`
	RequestErrors4xx int64 `json:"request_errors_4xx"`
}

func avgDuration(total time.Duration, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(total.Milliseconds()) / float64(count)
}
