package api

import (
	"sync"
	"time"
)

type limitEntry struct {
	failures    int
	lockedUntil time.Time
}

// loginLimiter locks a username+IP pair after repeated failures.
type loginLimiter struct {
	mu          sync.Mutex
	entries     map[string]*limitEntry
	maxFailures int
	lockout     time.Duration
	now         func() time.Time
}

func newLoginLimiter(maxFailures int, lockout time.Duration, now func() time.Time) *loginLimiter {
	return &loginLimiter{
		entries:     make(map[string]*limitEntry),
		maxFailures: maxFailures,
		lockout:     lockout,
		now:         now,
	}
}

func (l *loginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[key]
	if e == nil || e.lockedUntil.IsZero() {
		return true
	}
	return l.now().After(e.lockedUntil)
}

func (l *loginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[key]
	if e == nil {
		e = &limitEntry{}
		l.entries[key] = e
	}
	if !e.lockedUntil.IsZero() && l.now().After(e.lockedUntil) {
		e.failures = 0
		e.lockedUntil = time.Time{}
	}
	e.failures++
	if e.failures >= l.maxFailures {
		e.failures = 0
		e.lockedUntil = l.now().Add(l.lockout)
	}
}

func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
