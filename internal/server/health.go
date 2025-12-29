package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
)

// HealthStatus represents the overall health of the system
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentStatus represents the health of an individual component
type ComponentStatus string

const (
	ComponentStatusUp       ComponentStatus = "up"
	ComponentStatusDown     ComponentStatus = "down"
	ComponentStatusDegraded ComponentStatus = "degraded"
)

// Health represents the complete health check response
type Health struct {
	Status     HealthStatus           `json:"status"`
	Timestamp  time.Time              `json:"timestamp"`
	Version    string                 `json:"version,omitempty"`
	Components map[string]ComponentHealth `json:"components"`
}

// ComponentHealth represents the health of a single system component
type ComponentHealth struct {
	Status    ComponentStatus `json:"status"`
	Message   string          `json:"message,omitempty"`
	LatencyMs float64         `json:"latency_ms,omitempty"`
	Details   interface{}     `json:"details,omitempty"`
}

// StorageDetails provides additional storage health information
type StorageDetails struct {
	AvailableBytes int64   `json:"available_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	TotalBytes     int64   `json:"total_bytes"`
	PercentageUsed float64 `json:"percentage_used"`
}

// HandleHealth provides a detailed health check endpoint
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	health := s.checkHealth()

	// Set status code based on health
	statusCode := http.StatusOK
	if health.Status == HealthStatusDegraded {
		statusCode = http.StatusOK // Still return 200 for degraded
	} else if health.Status == HealthStatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(health)
}

// HandleReady provides a simple readiness probe for Kubernetes/load balancers
func (s *Server) HandleReady(w http.ResponseWriter, r *http.Request) {
	// Quick check: can we query the database?
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	var result int
	err := s.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)

	if err != nil {
		http.Error(w, `{"status":"not_ready","message":"database unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// HandleLive provides a liveness probe (is the process running?)
func (s *Server) HandleLive(w http.ResponseWriter, r *http.Request) {
	// Always returns OK if the process is running
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "alive",
	})
}

// checkHealth performs comprehensive health checks on all components
func (s *Server) checkHealth() Health {
	health := Health{
		Timestamp:  time.Now(),
		Version:    s.version, // Add version field to Server struct
		Components: make(map[string]ComponentHealth),
	}

	// Check database
	dbHealth := s.checkDatabaseHealth()
	health.Components["database"] = dbHealth

	// Check MinIO
	minioHealth := s.checkMinIOHealth()
	health.Components["minio"] = minioHealth

	// Check storage space
	storageHealth := s.checkStorageHealth()
	health.Components["storage"] = storageHealth

	// Determine overall health status
	health.Status = s.determineOverallHealth(health.Components)

	return health
}

// checkDatabaseHealth checks PostgreSQL connectivity and performance
func (s *Server) checkDatabaseHealth() ComponentHealth {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simple ping
	if err := s.db.PingContext(ctx); err != nil {
		return ComponentHealth{
			Status:  ComponentStatusDown,
			Message: "database ping failed: " + err.Error(),
		}
	}

	// Check if we can query
	var userCount int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return ComponentHealth{
			Status:  ComponentStatusDegraded,
			Message: "database query failed: " + err.Error(),
		}
	}

	latency := time.Since(start).Milliseconds()

	// Check connection pool stats
	stats := s.db.Stats()
	details := map[string]interface{}{
		"open_connections": stats.OpenConnections,
		"in_use":           stats.InUse,
		"idle":             stats.Idle,
		"wait_count":       stats.WaitCount,
		"wait_duration_ms": stats.WaitDuration.Milliseconds(),
	}

	status := ComponentStatusUp
	message := "database healthy"

	// Warn if latency is high
	if latency > 1000 {
		status = ComponentStatusDegraded
		message = "database latency high"
	}

	return ComponentHealth{
		Status:    status,
		Message:   message,
		LatencyMs: float64(latency),
		Details:   details,
	}
}

// checkMinIOHealth checks MinIO/S3 connectivity
func (s *Server) checkMinIOHealth() ComponentHealth {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if bucket exists and is accessible
	exists, err := s.minioClient.BucketExists(ctx, s.bucketName)
	if err != nil {
		return ComponentHealth{
			Status:  ComponentStatusDown,
			Message: "minio connection failed: " + err.Error(),
		}
	}

	if !exists {
		return ComponentHealth{
			Status:  ComponentStatusDown,
			Message: "bucket does not exist: " + s.bucketName,
		}
	}

	latency := time.Since(start).Milliseconds()

	status := ComponentStatusUp
	message := "minio healthy"

	if latency > 2000 {
		status = ComponentStatusDegraded
		message = "minio latency high"
	}

	return ComponentHealth{
		Status:    status,
		Message:   message,
		LatencyMs: float64(latency),
	}
}

// checkStorageHealth checks available disk space
func (s *Server) checkStorageHealth() ComponentHealth {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get total storage used from database
	var totalUsed int64
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(size_bytes), 0) FROM files WHERE status != 'failed'").Scan(&totalUsed)
	if err != nil {
		return ComponentHealth{
			Status:  ComponentStatusDegraded,
			Message: "could not query storage usage: " + err.Error(),
		}
	}

	// For demonstration, assume 1TB total capacity
	// In production, query actual disk space from the system
	const totalCapacity int64 = 1024 * 1024 * 1024 * 1024 // 1TB
	available := totalCapacity - totalUsed
	percentageUsed := float64(totalUsed) / float64(totalCapacity) * 100

	details := StorageDetails{
		AvailableBytes: available,
		UsedBytes:      totalUsed,
		TotalBytes:     totalCapacity,
		PercentageUsed: percentageUsed,
	}

	status := ComponentStatusUp
	message := "storage healthy"

	// Warn if storage is getting full
	if percentageUsed > 90 {
		status = ComponentStatusDegraded
		message = "storage critically low"
	} else if percentageUsed > 80 {
		status = ComponentStatusDegraded
		message = "storage running low"
	}

	return ComponentHealth{
		Status:  status,
		Message: message,
		Details: details,
	}
}

// determineOverallHealth calculates overall health from component statuses
func (s *Server) determineOverallHealth(components map[string]ComponentHealth) HealthStatus {
	var (
		downCount     int
		degradedCount int
	)

	for _, component := range components {
		switch component.Status {
		case ComponentStatusDown:
			downCount++
		case ComponentStatusDegraded:
			degradedCount++
		}
	}

	// If any critical component is down, system is unhealthy
	if downCount > 0 {
		return HealthStatusUnhealthy
	}

	// If any component is degraded, system is degraded
	if degradedCount > 0 {
		return HealthStatusDegraded
	}

	return HealthStatusHealthy
}
