package ipc

import (
	"net"
	"sync"
	"time"
)

// Session is the per-connection state of an authenticated IPC client. The
// server hands a *Session to every HandlerFunc.
type Session struct {
	mu              sync.Mutex
	conn            net.Conn
	tokenID         string
	tokenName       string
	authedAt        time.Time
	pid             int
	revealFails     int
	revealLockUntil time.Time
}

// revealMaxFails consecutive failed master-password checks arm a temporary
// lockout of the reveal gate for revealLockWindow.
const (
	revealMaxFails   = 5
	revealLockWindow = 30 * time.Second
)

// IsAuthenticated reports whether the session has completed `hello`.
func (s *Session) IsAuthenticated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenID != ""
}

// Authenticate records a successful hello.
func (s *Session) Authenticate(tokenID, tokenName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokenID = tokenID
	s.tokenName = tokenName
	s.authedAt = time.Now()
}

// TokenID returns the authenticated token id (empty if not authenticated).
func (s *Session) TokenID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenID
}

// TokenName returns the friendly name supplied at token creation.
func (s *Session) TokenName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenName
}

// PID returns the peer process id when discoverable, or 0.
func (s *Session) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

// RevealLockRemaining reports how long the reveal gate is locked out for this
// session after repeated master-password failures, or 0 if it is not locked.
func (s *Session) RevealLockRemaining() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revealLockUntil.IsZero() {
		return 0
	}
	if d := time.Until(s.revealLockUntil); d > 0 {
		return d
	}
	return 0
}

// RecordRevealFailure counts one failed reveal attempt and arms a temporary
// lockout once revealMaxFails consecutive failures are reached.
func (s *Session) RecordRevealFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revealFails++
	if s.revealFails >= revealMaxFails {
		s.revealLockUntil = time.Now().Add(revealLockWindow)
		s.revealFails = 0
	}
}

// ResetRevealFailures clears the reveal failure counter and lockout after a
// successful master-password check.
func (s *Session) ResetRevealFailures() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revealFails = 0
	s.revealLockUntil = time.Time{}
}
