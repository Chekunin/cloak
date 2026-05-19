// Package vault implements the master-key lifecycle and the lock state
// machine described in Section 3.1 of the spec.
//
// The vault holds:
//   - the on-disk meta (KDF params, wrapped DEK)
//   - the in-memory DEK once unlocked
//   - the lock state (Uninitialized / Locked / Unlocked)
//   - an idle auto-lock timer
//
// All encryption/decryption of secret material (in internal/store) goes
// through this package — there is no ad-hoc crypto elsewhere in Cloak.
package vault

import (
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/Chekunin/cloak/internal/errs"
	"github.com/Chekunin/cloak/internal/secrets"
)

// State enumerates the three lock-state values.
type State int

const (
	StateUninitialized State = iota
	StateLocked
	StateUnlocked
)

func (s State) String() string {
	switch s {
	case StateUninitialized:
		return "uninitialized"
	case StateLocked:
		return "locked"
	case StateUnlocked:
		return "unlocked"
	default:
		return "unknown"
	}
}

// LockHook is invoked from Lock / auto-lock, *before* the DEK is zeroed. The
// daemon registers a hook that tears down endpoints and connections. Multiple
// hooks run in registration order.
type LockHook func(reason LockReason)

// LockReason explains why a lock transition is happening.
type LockReason string

const (
	LockReasonExplicit LockReason = "explicit"
	LockReasonIdle     LockReason = "idle"
	LockReasonShutdown LockReason = "shutdown"
)

// Manager owns the vault state.
type Manager struct {
	mu          sync.RWMutex
	metaPath    string
	meta        *Meta
	state       State
	dek         *secrets.SecretBytes
	idleTimeout time.Duration
	lastActive  time.Time
	timer       *time.Timer
	hooks       []LockHook
	stopCh      chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
}

// New constructs a Manager bound to metaPath. It loads any existing meta and
// puts the manager in Locked or Uninitialized accordingly.
func New(metaPath string, idleTimeout time.Duration) (*Manager, error) {
	exists, err := MetaExists(metaPath)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		metaPath:    metaPath,
		idleTimeout: idleTimeout,
		stopCh:      make(chan struct{}),
	}
	if !exists {
		m.state = StateUninitialized
		return m, nil
	}
	meta, err := LoadMeta(metaPath)
	if err != nil {
		return nil, err
	}
	m.meta = meta
	m.state = StateLocked
	return m, nil
}

// State returns the current lock state.
func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// IdleTimeout returns the configured idle auto-lock interval.
func (m *Manager) IdleTimeout() time.Duration { return m.idleTimeout }

// ExpiresAt returns when the vault will auto-lock next, or the zero time if it
// is not currently unlocked.
func (m *Manager) ExpiresAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state != StateUnlocked {
		return time.Time{}
	}
	return m.lastActive.Add(m.idleTimeout)
}

// RegisterLockHook records hook for invocation on every lock transition.
func (m *Manager) RegisterLockHook(hook LockHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// Init creates a fresh vault: random DEK, random salt, wraps DEK under KEK
// derived from password, writes vault.meta.json. The DEK is not retained; the
// vault remains Locked until Unlock is called.
func (m *Manager) Init(password *secrets.SecretBytes) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != StateUninitialized {
		return errs.New(errs.CodeVaultAlreadyInitialized, "vault already initialized")
	}
	kdf := DefaultKDF()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	kdf = kdf.withSalt(salt)
	kek, err := DeriveKEK(password, kdf)
	if err != nil {
		return err
	}
	defer kek.Zero()
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return err
	}
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
	}()
	wrapped, err := wrapDEK(kek.Bytes(), dek)
	if err != nil {
		return err
	}
	meta := &Meta{
		FormatVersion: VaultFormatVersion,
		CreatedAt:     time.Now().UTC(),
		KDF:           kdf,
		WrappedDEK:    wrapped,
		UnlockMethods: []string{"password"},
	}
	if err := SaveMeta(m.metaPath, meta); err != nil {
		return err
	}
	m.meta = meta
	m.state = StateLocked
	return nil
}

// Unlock derives the KEK from password, unwraps the DEK, and transitions to
// Unlocked. Starts the idle auto-lock timer.
func (m *Manager) Unlock(password *secrets.SecretBytes) error {
	m.mu.Lock()
	switch m.state {
	case StateUninitialized:
		m.mu.Unlock()
		return errs.New(errs.CodeVaultNotInitialized, "vault is not initialized; run `cloak init`")
	case StateUnlocked:
		// idempotent
		m.lastActive = time.Now()
		m.resetTimerLocked()
		m.mu.Unlock()
		return nil
	}
	meta := m.meta
	m.mu.Unlock()

	kek, err := DeriveKEK(password, meta.KDF)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, err)
	}
	defer kek.Zero()
	dek, err := unwrapDEK(kek.Bytes(), meta.WrappedDEK)
	if err != nil {
		return errs.New(errs.CodeUnauthorized, "incorrect master password")
	}

	m.mu.Lock()
	m.dek = dek
	m.state = StateUnlocked
	m.lastActive = time.Now()
	m.resetTimerLocked()
	m.mu.Unlock()
	return nil
}

// Lock zeros the DEK, transitions to Locked, and fires all lock hooks.
// Passing reason=LockReasonShutdown is appropriate during daemon shutdown.
func (m *Manager) Lock(reason LockReason) {
	m.mu.Lock()
	if m.state != StateUnlocked {
		m.mu.Unlock()
		return
	}
	hooks := append([]LockHook(nil), m.hooks...)
	dek := m.dek
	m.dek = nil
	m.state = StateLocked
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.mu.Unlock()

	for _, h := range hooks {
		h(reason)
	}
	if dek != nil {
		dek.Zero()
	}
}

// Touch updates the last-active timestamp, deferring the next auto-lock.
// Adapters call this whenever they decrypt secret material or proxy traffic.
func (m *Manager) Touch() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != StateUnlocked {
		return
	}
	m.lastActive = time.Now()
	m.resetTimerLocked()
}

// Encrypt seals plaintext with the DEK. Caller must ensure Unlocked state.
// associatedData binds the ciphertext to a context string (e.g. "secret.payload")
// so it cannot be transplanted between fields.
func (m *Manager) Encrypt(plaintext, associatedData []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state != StateUnlocked || m.dek == nil {
		return nil, errs.New(errs.CodeVaultLocked, "vault is locked")
	}
	return seal(m.dek.Bytes(), plaintext, associatedData)
}

// Decrypt opens a blob previously produced by Encrypt with matching AAD.
func (m *Manager) Decrypt(ciphertext, associatedData []byte) (*secrets.SecretBytes, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state != StateUnlocked || m.dek == nil {
		return nil, errs.New(errs.CodeVaultLocked, "vault is locked")
	}
	plain, err := open(m.dek.Bytes(), ciphertext, associatedData)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, errors.New("vault: decryption failed"))
	}
	sb := secrets.NewFromBytes(plain)
	for i := range plain {
		plain[i] = 0
	}
	return sb, nil
}

// Shutdown locks the vault if unlocked, stops the timer, and waits for any
// background goroutines to exit. Safe to call multiple times.
func (m *Manager) Shutdown() {
	m.Lock(LockReasonShutdown)
	m.stopOnce.Do(func() { close(m.stopCh) })
	m.wg.Wait()
}

// resetTimerLocked schedules the next idle auto-lock. Caller holds m.mu.
func (m *Manager) resetTimerLocked() {
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(m.idleTimeout, func() {
		// Only auto-lock if no Touch arrived in the meantime.
		m.mu.RLock()
		idle := time.Since(m.lastActive)
		state := m.state
		m.mu.RUnlock()
		if state != StateUnlocked {
			return
		}
		if idle+50*time.Millisecond < m.idleTimeout {
			// Activity happened. Re-arm. Touch already did this; do nothing here.
			return
		}
		m.Lock(LockReasonIdle)
	})
}
