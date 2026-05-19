package sshadapter

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

// LoadOrGenerateHostKeys ensures dir contains an ed25519 and rsa host key,
// generating them if missing, and returns ssh.Signers for use as host keys.
// Directory and file permissions are set to 0700 / 0600 respectively.
func LoadOrGenerateHostKeys(dir string) ([]gossh.Signer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ssh: host_keys mkdir: %w", err)
	}
	var signers []gossh.Signer

	edPath := filepath.Join(dir, "ssh_host_ed25519_key")
	edSigner, err := loadOrGenerateEd25519(edPath)
	if err != nil {
		return nil, err
	}
	signers = append(signers, edSigner)

	rsaPath := filepath.Join(dir, "ssh_host_rsa_key")
	rsaSigner, err := loadOrGenerateRSA(rsaPath, 3072)
	if err != nil {
		return nil, err
	}
	signers = append(signers, rsaSigner)

	return signers, nil
}

func loadOrGenerateEd25519(path string) (gossh.Signer, error) {
	if data, err := os.ReadFile(path); err == nil {
		return gossh.ParsePrivateKey(data)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	pemBytes, err := marshalEd25519PEM(priv)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return gossh.ParsePrivateKey(pemBytes)
}

func loadOrGenerateRSA(path string, bits int) (gossh.Signer, error) {
	if data, err := os.ReadFile(path); err == nil {
		return gossh.ParsePrivateKey(data)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return gossh.ParsePrivateKey(pemBytes)
}

// marshalEd25519PEM writes an ed25519 private key as an OpenSSH-compatible
// PRIVATE KEY (PKCS#8) PEM. golang.org/x/crypto/ssh accepts PKCS#8 ed25519.
func marshalEd25519PEM(priv ed25519.PrivateKey) ([]byte, error) {
	if l := len(priv); l != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("ssh: bad ed25519 key length %d", l)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// unused helpers retained for completeness in case we add ECDSA in v1.x.
var _ = elliptic.P256
var _ = (*ecdsa.PrivateKey)(nil)
var errUnsupported = errors.New("ssh: unsupported key type")
