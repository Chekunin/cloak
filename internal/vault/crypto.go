package vault

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"

	"github.com/Chekunin/cloak/internal/secrets"
)

// DeriveKEK derives a 32-byte KEK from password using Argon2id with the given
// params. The result is owned by the caller and should be zeroed when done.
func DeriveKEK(password *secrets.SecretBytes, k KDFParams) (*secrets.SecretBytes, error) {
	if password == nil {
		return nil, errors.New("vault: nil password")
	}
	salt, err := k.saltBytes()
	if err != nil {
		return nil, fmt.Errorf("vault: bad salt: %w", err)
	}
	if len(salt) == 0 {
		return nil, errors.New("vault: empty salt")
	}
	out := argon2.IDKey(password.Bytes(), salt, k.TimeCost, k.MemoryKiB, k.Parallelism, k.KeyLength)
	sb := secrets.NewFromBytes(out)
	// zero the intermediate slice
	for i := range out {
		out[i] = 0
	}
	return sb, nil
}

// seal returns nonce||ciphertext encrypted under key with XChaCha20-Poly1305.
// associatedData may be nil.
func seal(key, plaintext, associatedData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("vault: aead init: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("vault: nonce: %w", err)
	}
	out := aead.Seal(nil, nonce, plaintext, associatedData)
	return append(nonce, out...), nil
}

// open decrypts a nonce||ciphertext blob produced by seal.
func open(key, blob, associatedData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("vault: aead init: %w", err)
	}
	if len(blob) < aead.NonceSize() {
		return nil, errors.New("vault: ciphertext too short")
	}
	nonce := blob[:aead.NonceSize()]
	ct := blob[aead.NonceSize():]
	return aead.Open(nil, nonce, ct, associatedData)
}

// wrapDEK encrypts dek under kek and returns base64(nonce||ct).
func wrapDEK(kek, dek []byte) (string, error) {
	blob, err := seal(kek, dek, []byte("cloak.dek.v1"))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(blob), nil
}

func unwrapDEK(kek []byte, wrappedB64 string) (*secrets.SecretBytes, error) {
	blob, err := base64.StdEncoding.DecodeString(wrappedB64)
	if err != nil {
		return nil, fmt.Errorf("vault: bad wrapped DEK: %w", err)
	}
	plain, err := open(kek, blob, []byte("cloak.dek.v1"))
	if err != nil {
		return nil, errors.New("vault: unlock failed (wrong password?)")
	}
	sb := secrets.NewFromBytes(plain)
	for i := range plain {
		plain[i] = 0
	}
	return sb, nil
}
