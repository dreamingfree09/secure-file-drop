// prometheus.go - Prometheus metrics exporter

package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// PrometheusExporter converts internal metrics to Prometheus format
type PrometheusExporter struct{}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter() *PrometheusExporter {
	return &PrometheusExporter{}
}

// Handler returns an HTTP handler for the /metrics endpoint
func (p *PrometheusExporter) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get current metrics snapshot
		snapshot := GetMetrics().Snapshot()

		// Build Prometheus format output
		var output strings.Builder

		// Write header
		output.WriteString("# HELP sfd_info Application version info\n")
		output.WriteString("# TYPE sfd_info gauge\n")
		output.WriteString("sfd_info{version=\"dev\"} 1\n\n")

		// Request metrics
		output.WriteString("# HELP sfd_requests_total Total number of HTTP requests\n")
		output.WriteString("# TYPE sfd_requests_total counter\n")
		output.Write(fmt.Appendf(nil, "sfd_requests_total %d\n\n", snapshot.RequestsTotal))

		// Upload metrics
		output.WriteString("# HELP sfd_uploads_total Total number of file uploads\n")
		output.WriteString("# TYPE sfd_uploads_total counter\n")
		output.Write(fmt.Appendf(nil, "sfd_uploads_total %d\n\n", snapshot.UploadsTotal))

		// Download metrics
		output.WriteString("# HELP sfd_downloads_total Total number of file downloads\n")
		output.WriteString("# TYPE sfd_downloads_total counter\n")
		output.Write(fmt.Appendf(nil, "sfd_downloads_total %d\n\n", snapshot.DownloadsTotal))

		output.WriteString("# HELP sfd_download_bytes_total Bytes in completed downloads\n# TYPE sfd_download_bytes_total counter\n")
		output.Write(fmt.Appendf(nil, "sfd_download_bytes_total %d\n\n", snapshot.DownloadBytesTotal))
		output.WriteString("# HELP sfd_download_errors_total Failed authorized download attempts\n# TYPE sfd_download_errors_total counter\n")
		output.Write(fmt.Appendf(nil, "sfd_download_errors_total %d\n\n", snapshot.DownloadErrorsTotal))

		// Storage metrics
		output.WriteString("# HELP sfd_storage_bytes Total storage used in bytes\n")
		output.WriteString("# TYPE sfd_storage_bytes gauge\n")
		output.Write(fmt.Appendf(nil, "sfd_storage_bytes %d\n\n", snapshot.StorageTotalBytes))

		output.WriteString("# HELP sfd_storage_files Total number of stored files\n")
		output.WriteString("# TYPE sfd_storage_files gauge\n")
		output.Write(fmt.Appendf(nil, "sfd_storage_files %d\n\n", snapshot.StorageTotalFiles))

		// Login metrics
		output.WriteString("# HELP sfd_login_success_total Total number of successful logins\n")
		output.WriteString("# TYPE sfd_login_success_total counter\n")
		output.Write(fmt.Appendf(nil, "sfd_login_success_total %d\n\n", snapshot.LoginSuccessTotal))

		output.WriteString("# HELP sfd_login_failures_total Total number of failed logins\n")
		output.WriteString("# TYPE sfd_login_failures_total counter\n")
		output.Write(fmt.Appendf(nil, "sfd_login_failures_total %d\n\n", snapshot.LoginFailuresTotal))

		// File status metrics
		output.WriteString("# HELP sfd_files_by_status Files grouped by status\n")
		output.WriteString("# TYPE sfd_files_by_status gauge\n")
		output.Write(fmt.Appendf(nil, "sfd_files_by_status{status=\"ready\"} %d\n", snapshot.FilesReadyTotal))
		output.Write(fmt.Appendf(nil, "sfd_files_by_status{status=\"pending\"} %d\n", snapshot.FilesPendingTotal))
		output.Write(fmt.Appendf(nil, "sfd_files_by_status{status=\"failed\"} %d\n\n", snapshot.FilesFailedTotal))

		// Response time histogram (if we add this in the future)
		output.WriteString("# HELP sfd_uptime_seconds Application uptime in seconds\n")
		output.WriteString("# TYPE sfd_uptime_seconds counter\n")
		uptime := time.Since(serverStartTime).Seconds()
		output.Write(fmt.Appendf(nil, "sfd_uptime_seconds %.0f\n\n", uptime))

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(output.String())); err != nil {
			Warn("metrics response write failed", nil)
		}
	}
}

// PrometheusMetricsHandler creates a handler that exports metrics in Prometheus format
func PrometheusMetricsHandler() http.Handler {
	exporter := NewPrometheusExporter()
	return exporter.Handler()
}

// MetricsSummary provides additional metrics for Prometheus
type MetricsSummary struct {
	mu                   sync.RWMutex
	requestDurations     map[string][]float64 // endpoint -> durations in ms
	errorRatesByEndpoint map[string]int
}

var (
	metricsSummary = &MetricsSummary{
		requestDurations:     make(map[string][]float64),
		errorRatesByEndpoint: make(map[string]int),
	}
	serverStartTime = time.Now()
)

// RecordRequestDuration records the duration of a request for histogram metrics
func RecordRequestDuration(endpoint string, durationMs float64) {
	metricsSummary.mu.Lock()
	defer metricsSummary.mu.Unlock()

	if metricsSummary.requestDurations[endpoint] == nil {
		metricsSummary.requestDurations[endpoint] = make([]float64, 0, 1000)
	}

	durations := metricsSummary.requestDurations[endpoint]
	durations = append(durations, durationMs)

	// Keep only last 1000 samples per endpoint
	if len(durations) > 1000 {
		durations = durations[len(durations)-1000:]
	}

	metricsSummary.requestDurations[endpoint] = durations
}

// GetRequestDurationPercentiles returns percentile data for request durations
func GetRequestDurationPercentiles(endpoint string) (p50, p95, p99 float64) {
	metricsSummary.mu.RLock()
	defer metricsSummary.mu.RUnlock()

	durations := metricsSummary.requestDurations[endpoint]
	if len(durations) == 0 {
		return 0, 0, 0
	}

	// Sort durations
	sorted := make([]float64, len(durations))
	copy(sorted, durations)
	sort.Float64s(sorted)

	// Calculate percentiles
	p50 = sorted[len(sorted)*50/100]
	p95 = sorted[len(sorted)*95/100]
	p99 = sorted[len(sorted)*99/100]

	return
}
