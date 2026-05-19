package vault

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// VaultFormatVersion is the on-disk meta format. Bump when meta layout changes.
const VaultFormatVersion = 1

// Meta is the unencrypted vault metadata persisted to vault.meta.json.
// It contains *no* secret material: just KDF parameters, the wrapped DEK, and
// bookkeeping fields.
type Meta struct {
	FormatVersion int            `json:"format_version"`
	CreatedAt     time.Time      `json:"created_at"`
	KDF           KDFParams      `json:"kdf"`
	WrappedDEK    string         `json:"wrapped_dek"` // base64 of nonce||ciphertext (XChaCha20-Poly1305)
	UnlockMethods []string       `json:"unlock_methods"`
	Extra         map[string]any `json:"extra,omitempty"`
}

// KDFParams describes the Argon2id parameters and the salt for KEK derivation.
// Stored alongside the wrapped DEK so changes in defaults do not break old vaults.
type KDFParams struct {
	Algorithm   string `json:"algorithm"` // always "argon2id" in v1
	TimeCost    uint32 `json:"time"`
	MemoryKiB   uint32 `json:"memory_kib"`
	Parallelism uint8  `json:"parallelism"`
	KeyLength   uint32 `json:"key_length"`
	SaltB64     string `json:"salt"`
}

// DefaultKDF returns the v1 parameters.
func DefaultKDF() KDFParams {
	return KDFParams{
		Algorithm:   "argon2id",
		TimeCost:    3,
		MemoryKiB:   64 * 1024,
		Parallelism: 4,
		KeyLength:   32,
	}
}

// LoadMeta reads vault.meta.json. Returns os.ErrNotExist if absent.
func LoadMeta(path string) (*Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("vault: parse meta: %w", err)
	}
	if m.FormatVersion != VaultFormatVersion {
		return nil, fmt.Errorf("vault: unsupported format version %d", m.FormatVersion)
	}
	if m.KDF.Algorithm != "argon2id" {
		return nil, fmt.Errorf("vault: unsupported KDF %q", m.KDF.Algorithm)
	}
	return &m, nil
}

// SaveMeta atomically writes meta to path with 0600 permissions.
func SaveMeta(path string, m *Meta) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

// MetaExists reports whether vault meta is present on disk.
func MetaExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (k KDFParams) saltBytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(k.SaltB64)
}

func (k KDFParams) withSalt(salt []byte) KDFParams {
	k.SaltB64 = base64.StdEncoding.EncodeToString(salt)
	return k
}
