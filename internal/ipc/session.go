package ipc

import (
	"net"
	"sync"
	"time"
)

// Session is the per-connection state of an authenticated IPC client. The
// server hands a *Session to every HandlerFunc.
type Session struct {
	mu          sync.Mutex
	conn        net.Conn
	tokenID     string
	tokenName   string
	authedAt    time.Time
	pid         int
}

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
