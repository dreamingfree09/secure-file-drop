// circuitbreaker.go - Circuit breaker pattern for Secure File Drop.
//
// Implements circuit breaker to prevent cascading failures when
// external dependencies (database, MinIO, SMTP) become unavailable.
package server

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
	// StateClosed: Circuit is closed, requests flow normally
	StateClosed CircuitState = iota
	// StateOpen: Circuit is open, requests fail fast
	StateOpen
	// StateHalfOpen: Circuit is testing if service recovered
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrCircuitOpen is returned when circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrTooManyRequests is returned when half-open circuit receives too many requests.
	ErrTooManyRequests = errors.New("too many requests while circuit is half-open")
)

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu sync.RWMutex

	// Configuration
	maxFailures uint32        // Failures before opening circuit
	timeout     time.Duration // Time to wait before attempting recovery
	maxHalfOpen uint32        // Max concurrent requests in half-open state

	// State
	state            CircuitState
	failures         uint32
	lastFailureTime  time.Time
	halfOpenRequests uint32

	// Statistics
	totalRequests    uint64
	successRequests  uint64
	failedRequests   uint64
	rejectedRequests uint64
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(maxFailures uint32, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures: maxFailures,
		timeout:     timeout,
		maxHalfOpen: 1, // Allow 1 request to test recovery
		state:       StateClosed,
	}
}

// Execute runs the given function with circuit breaker protection.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++

	// Check current state
	switch cb.state {
	case StateOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailureTime) > cb.timeout {
			// Transition to half-open
			cb.state = StateHalfOpen
			cb.halfOpenRequests = 0
			Info("circuit_breaker_half_open", map[string]any{
				"timeout_elapsed": cb.timeout.String(),
			})
		} else {
			// Still open, fail fast
			cb.rejectedRequests++
			return ErrCircuitOpen
		}

	case StateHalfOpen:
		// Limit concurrent requests in half-open state
		if cb.halfOpenRequests >= cb.maxHalfOpen {
			cb.rejectedRequests++
			return ErrTooManyRequests
		}
		cb.halfOpenRequests++
	}

	// Execute the function
	cb.mu.Unlock()
	err := fn()
	cb.mu.Lock()

	if err != nil {
		// Request failed
		cb.onFailure()
		return err
	}

	// Request succeeded
	cb.onSuccess()
	return nil
}

// onSuccess handles successful request.
func (cb *CircuitBreaker) onSuccess() {
	cb.successRequests++

	if cb.state == StateHalfOpen {
		// Recovery successful, close circuit
		cb.state = StateClosed
		cb.failures = 0
		Info("circuit_breaker_closed", map[string]any{
			"reason": "recovery_successful",
		})
	}
}

// onFailure handles failed request.
func (cb *CircuitBreaker) onFailure() {
	cb.failedRequests++
	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.maxFailures {
		// Open the circuit
		if cb.state != StateOpen {
			cb.state = StateOpen
			Warn("circuit_breaker_opened", map[string]any{
				"failures":     cb.failures,
				"max_failures": cb.maxFailures,
				"timeout":      cb.timeout.String(),
			})
		}
	}
}

// GetState returns the current circuit state (thread-safe).
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns circuit breaker statistics.
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:            cb.state,
		Failures:         cb.failures,
		TotalRequests:    cb.totalRequests,
		SuccessRequests:  cb.successRequests,
		FailedRequests:   cb.failedRequests,
		RejectedRequests: cb.rejectedRequests,
		LastFailureTime:  cb.lastFailureTime,
	}
}

// Reset manually resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.halfOpenRequests = 0

	Info("circuit_breaker_reset", map[string]any{
		"manual": true,
	})
}

// CircuitBreakerStats holds circuit breaker statistics.
type CircuitBreakerStats struct {
	State            CircuitState `json:"state"`
	Failures         uint32       `json:"failures"`
	TotalRequests    uint64       `json:"total_requests"`
	SuccessRequests  uint64       `json:"success_requests"`
	FailedRequests   uint64       `json:"failed_requests"`
	RejectedRequests uint64       `json:"rejected_requests"`
	LastFailureTime  time.Time    `json:"last_failure_time"`
}

// CircuitBreakerManager manages multiple circuit breakers for different services.
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerManager creates a manager for multiple circuit breakers.
func NewCircuitBreakerManager() *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// GetOrCreate returns existing circuit breaker or creates a new one.
func (cbm *CircuitBreakerManager) GetOrCreate(name string, maxFailures uint32, timeout time.Duration) *CircuitBreaker {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	if cb, exists := cbm.breakers[name]; exists {
		return cb
	}

	cb := NewCircuitBreaker(maxFailures, timeout)
	cbm.breakers[name] = cb

	Info("circuit_breaker_created", map[string]any{
		"name":         name,
		"max_failures": maxFailures,
		"timeout":      timeout.String(),
	})

	return cb
}

// Get returns an existing circuit breaker by name.
func (cbm *CircuitBreakerManager) Get(name string) (*CircuitBreaker, bool) {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	cb, exists := cbm.breakers[name]
	return cb, exists
}

// GetAllStats returns statistics for all circuit breakers.
func (cbm *CircuitBreakerManager) GetAllStats() map[string]CircuitBreakerStats {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats)
	for name, cb := range cbm.breakers {
		stats[name] = cb.GetStats()
	}

	return stats
}

// ResetAll resets all circuit breakers.
func (cbm *CircuitBreakerManager) ResetAll() {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	for _, cb := range cbm.breakers {
		cb.Reset()
	}
}
