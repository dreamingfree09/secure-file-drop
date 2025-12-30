// lockout.go - Account lockout mechanism to prevent brute-force attacks
package server

import (
	"sync"
	"time"
)

// LoginAttempt tracks failed login attempts for an account
type LoginAttempt struct {
	Count       int
	LastAttempt time.Time
	LockedUntil time.Time
}

// AccountLockout manages account lockout after failed login attempts
type AccountLockout struct {
	mu              sync.RWMutex
	attempts        map[string]*LoginAttempt // username -> attempts
	maxAttempts     int
	lockoutDuration time.Duration
	windowDuration  time.Duration
}

// NewAccountLockout creates a new account lockout manager
// maxAttempts: number of failed attempts before lockout (e.g., 5)
// lockoutDuration: how long to lock the account (e.g., 15 minutes)
// windowDuration: time window to count attempts (e.g., 10 minutes)
func NewAccountLockout(maxAttempts int, lockoutDuration, windowDuration time.Duration) *AccountLockout {
	al := &AccountLockout{
		attempts:        make(map[string]*LoginAttempt),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		windowDuration:  windowDuration,
	}

	// Cleanup old entries every hour
	go al.cleanup()

	return al
}

// RecordFailedAttempt records a failed login attempt
// Returns true if account should be locked
func (al *AccountLockout) RecordFailedAttempt(username string) (locked bool, lockedUntil time.Time) {
	al.mu.Lock()
	defer al.mu.Unlock()

	now := time.Now()

	attempt, exists := al.attempts[username]
	if !exists {
		attempt = &LoginAttempt{}
		al.attempts[username] = attempt
	}

	// Reset count if outside window
	if now.Sub(attempt.LastAttempt) > al.windowDuration {
		attempt.Count = 0
	}

	attempt.Count++
	attempt.LastAttempt = now

	// Lock account if max attempts exceeded
	if attempt.Count >= al.maxAttempts {
		attempt.LockedUntil = now.Add(al.lockoutDuration)
		return true, attempt.LockedUntil
	}

	return false, time.Time{}
}

// RecordSuccessfulLogin resets failed attempts for a username
func (al *AccountLockout) RecordSuccessfulLogin(username string) {
	al.mu.Lock()
	defer al.mu.Unlock()

	delete(al.attempts, username)
}

// IsLocked checks if an account is currently locked
// Returns true if locked, and the unlock time
func (al *AccountLockout) IsLocked(username string) (locked bool, lockedUntil time.Time, attemptsRemaining int) {
	al.mu.RLock()
	defer al.mu.RUnlock()

	attempt, exists := al.attempts[username]
	if !exists {
		return false, time.Time{}, al.maxAttempts
	}

	now := time.Now()

	// Check if still locked
	if !attempt.LockedUntil.IsZero() && now.Before(attempt.LockedUntil) {
		return true, attempt.LockedUntil, 0
	}

	// Check if attempts are outside window
	if now.Sub(attempt.LastAttempt) > al.windowDuration {
		return false, time.Time{}, al.maxAttempts
	}

	remaining := al.maxAttempts - attempt.Count
	if remaining < 0 {
		remaining = 0
	}

	return false, time.Time{}, remaining
}

// GetAttemptCount returns the current failed attempt count for a username
func (al *AccountLockout) GetAttemptCount(username string) int {
	al.mu.RLock()
	defer al.mu.RUnlock()

	attempt, exists := al.attempts[username]
	if !exists {
		return 0
	}

	now := time.Now()

	// Reset if outside window
	if now.Sub(attempt.LastAttempt) > al.windowDuration {
		return 0
	}

	return attempt.Count
}

// cleanup removes expired lockout entries
func (al *AccountLockout) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		al.mu.Lock()
		now := time.Now()

		for username, attempt := range al.attempts {
			// Remove if:
			// 1. Lockout expired and no recent attempts
			// 2. Last attempt was more than 2x window duration ago
			if (attempt.LockedUntil.IsZero() || now.After(attempt.LockedUntil)) &&
				now.Sub(attempt.LastAttempt) > 2*al.windowDuration {
				delete(al.attempts, username)
			}
		}

		al.mu.Unlock()
	}
}
