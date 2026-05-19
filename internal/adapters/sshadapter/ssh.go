// Package sshadapter implements the SSH adapter (Section 3.4.4).
//
// Cloak runs an SSH server on the listening port using x/crypto/ssh. On each
// accepted client connection (password-authenticated against the ephemeral
// local password), it opens an upstream SSH connection using the stored
// credentials and proxies channels and requests transparently.
package sshadapter

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	gossh "golang.org/x/crypto/ssh"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/store"
)

// Adapter is the SSH adapter.
type Adapter struct {
	hostKeyDir string
	once       sync.Once
	signers    []gossh.Signer
	signersErr error
}

// New returns an SSH adapter that loads host keys from hostKeyDir.
func New(hostKeyDir string) *Adapter { return &Adapter{hostKeyDir: hostKeyDir} }

// Type returns store.TypeSSH.
func (a *Adapter) Type() store.SecretType { return store.TypeSSH }

// Config is the non-secret portion.
type Config struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	User               string `json:"user"`
	HostKeyFingerprint string `json:"host_key_fingerprint"`
	JumpHostSecretID   string `json:"jump_host_secret_id,omitempty"`
}

// Payload is the secret portion.
type Payload struct {
	AuthMethod    string `json:"auth_method"`
	PrivateKeyPEM string `json:"private_key_pem,omitempty"`
	Passphrase    string `json:"passphrase,omitempty"`
	Password      string `json:"password,omitempty"`
}

func decodeConfig(m map[string]any) (Config, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if c.Port == 0 {
		c.Port = 22
	}
	return c, nil
}

// ValidateConfig implements adapters.Adapter.
func (a *Adapter) ValidateConfig(config map[string]any, secret map[string]any) error {
	c, err := decodeConfig(config)
	if err != nil {
		return fmt.Errorf("ssh: config: %w", err)
	}
	if c.Host == "" || c.User == "" {
		return errors.New("ssh: host and user are required")
	}
	method, _ := secret["auth_method"].(string)
	switch method {
	case "password":
		if pw, _ := secret["password"].(string); pw == "" {
			return errors.New("ssh: password is required for auth_method=password")
		}
	case "private_key":
		if k, _ := secret["private_key_pem"].(string); k == "" {
			return errors.New("ssh: private_key_pem is required for auth_method=private_key")
		}
	default:
		return errors.New(`ssh: auth_method must be "password" or "private_key"`)
	}
	return nil
}

func (a *Adapter) hostSigners() ([]gossh.Signer, error) {
	a.once.Do(func() {
		a.signers, a.signersErr = LoadOrGenerateHostKeys(a.hostKeyDir)
	})
	return a.signers, a.signersErr
}

// ServeConnection serves one accepted TCP connection from a client.
func (a *Adapter) ServeConnection(ctx context.Context, client net.Conn, dec adapters.DecryptedSecret, localCreds adapters.LocalCredentials) error {
	cfg, err := decodeConfig(dec.Config)
	if err != nil {
		return err
	}
	var payload Payload
	if err := json.Unmarshal(dec.Payload.Bytes(), &payload); err != nil {
		return fmt.Errorf("ssh: payload: %w", err)
	}
	signers, err := a.hostSigners()
	if err != nil {
		return err
	}

	// Server-side SSH config — password-only auth against the local creds.
	serverCfg := &gossh.ServerConfig{
		PasswordCallback: func(conn gossh.ConnMetadata, pw []byte) (*gossh.Permissions, error) {
			if localCreds.Password == nil || localCreds.Password.Len() == 0 {
				return nil, errors.New("ssh: local auth not configured")
			}
			if subtle.ConstantTimeCompare(pw, localCreds.Password.Bytes()) != 1 {
				return nil, fmt.Errorf("ssh: %w", adapters.ErrLocalAuth)
			}
			return nil, nil
		},
	}
	for _, s := range signers {
		serverCfg.AddHostKey(s)
	}

	srvConn, chans, reqs, err := gossh.NewServerConn(client, serverCfg)
	if err != nil {
		return fmt.Errorf("ssh: handshake: %w", err)
	}
	defer srvConn.Close()

	// Dial upstream using stored credentials.
	upstream, err := dialUpstream(ctx, cfg, payload)
	if err != nil {
		return fmt.Errorf("ssh: upstream dial: %w", err)
	}
	defer upstream.Close()

	// Plumb global requests both directions.
	go forwardGlobalRequests(reqs, upstream)

	// Per-channel proxying.
	var wg sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case newCh, ok := <-chans:
			if !ok {
				wg.Wait()
				return nil
			}
			wg.Add(1)
			go func(nc gossh.NewChannel) {
				defer wg.Done()
				proxyChannel(ctx, nc, upstream)
			}(newCh)
		}
	}
}

// ConnectionString returns an informational ssh:// URL.
func (a *Adapter) ConnectionString(localAddr string, _ adapters.DecryptedSecret, creds adapters.LocalCredentials) string {
	host, port := splitHostPort(localAddr)
	return fmt.Sprintf("ssh://%s@%s:%s", creds.Username, host, port)
}

// EnvVars returns <NAME>_SSH_* values for `cloak exec`.
func (a *Adapter) EnvVars(localAddr string, _ adapters.DecryptedSecret, creds adapters.LocalCredentials, prefix string) map[string]string {
	host, port := splitHostPort(localAddr)
	out := map[string]string{
		prefix + "_SSH_HOST": host,
		prefix + "_SSH_PORT": port,
		prefix + "_SSH_USER": creds.Username,
	}
	if creds.Password != nil {
		out[prefix+"_SSH_PASSWORD"] = creds.Password.String()
	}
	return out
}

// --- upstream + proxying ---

func dialUpstream(ctx context.Context, cfg Config, payload Payload) (*gossh.Client, error) {
	auth, err := buildAuth(payload)
	if err != nil {
		return nil, err
	}
	clientCfg := &gossh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: makeHostKeyCallback(cfg.HostKeyFingerprint),
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	var d net.Dialer
	rawConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	sshConn, chans, reqs, err := gossh.NewClientConn(rawConn, addr, clientCfg)
	if err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	return gossh.NewClient(sshConn, chans, reqs), nil
}

func buildAuth(p Payload) ([]gossh.AuthMethod, error) {
	switch p.AuthMethod {
	case "password":
		return []gossh.AuthMethod{gossh.Password(p.Password)}, nil
	case "private_key":
		var (
			signer gossh.Signer
			err    error
		)
		if p.Passphrase != "" {
			signer, err = gossh.ParsePrivateKeyWithPassphrase([]byte(p.PrivateKeyPEM), []byte(p.Passphrase))
		} else {
			signer, err = gossh.ParsePrivateKey([]byte(p.PrivateKeyPEM))
		}
		if err != nil {
			return nil, fmt.Errorf("ssh: parse private key: %w", err)
		}
		return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("ssh: unsupported auth_method %q", p.AuthMethod)
	}
}

// makeHostKeyCallback returns a HostKeyCallback that validates the upstream
// host key's SHA256 fingerprint against expected. An empty expected fingerprint
// rejects every key (TOFU is not a v1 feature).
//
// Accepted forms for `expected`:
//   - "SHA256:<base64>" — standard ssh-keygen -lf output (canonical form)
//   - "sha256:<base64>" — same, lowercase prefix
//   - "sha256:<hex>"    — legacy hex form
func makeHostKeyCallback(expected string) gossh.HostKeyCallback {
	expected = strings.TrimSpace(expected)
	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		if expected == "" {
			return errors.New("ssh: host_key_fingerprint is required (pin the upstream key)")
		}
		actual := gossh.FingerprintSHA256(key) // "SHA256:<base64>"
		// Canonicalise both sides to "SHA256:<base64>" for comparison.
		canon := canonicalFingerprint(expected)
		if subtle.ConstantTimeCompare([]byte(actual), []byte(canon)) == 1 {
			return nil
		}
		// Hex form fallback.
		if strings.HasPrefix(strings.ToLower(expected), "sha256:") {
			rawHex := strings.TrimPrefix(strings.ToLower(expected), "sha256:")
			if _, err := hex.DecodeString(rawHex); err == nil && strings.EqualFold(actual, "SHA256:"+rawHex) {
				return nil
			}
		}
		return fmt.Errorf("ssh: host key mismatch: got %s, expected %s", actual, expected)
	}
}

// canonicalFingerprint normalises lowercase "sha256:" prefix to uppercase.
// Leaves anything else untouched (so a mis-formatted fingerprint still fails
// in the constant-time compare instead of silently matching).
func canonicalFingerprint(s string) string {
	if strings.HasPrefix(s, "sha256:") {
		return "SHA256:" + s[len("sha256:"):]
	}
	return s
}

func forwardGlobalRequests(in <-chan *gossh.Request, upstream *gossh.Client) {
	for req := range in {
		// `tcpip-forward` and `cancel-tcpip-forward` are reverse forwarding —
		// rejected per the spec.
		if req.Type == "tcpip-forward" || req.Type == "cancel-tcpip-forward" {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			continue
		}
		ok, payload, err := upstream.SendRequest(req.Type, req.WantReply, req.Payload)
		if err != nil {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			continue
		}
		if req.WantReply {
			_ = req.Reply(ok, payload)
		}
	}
}

// proxyChannel handles a single channel-open request from the client by
// opening the corresponding channel upstream and bidirectionally proxying
// data and channel-scoped requests.
func proxyChannel(ctx context.Context, newCh gossh.NewChannel, upstream *gossh.Client) {
	switch newCh.ChannelType() {
	case "session", "direct-tcpip":
		// supported
	case "x11", "auth-agent@openssh.com":
		_ = newCh.Reject(gossh.Prohibited, "channel type rejected by cloak")
		return
	default:
		_ = newCh.Reject(gossh.UnknownChannelType, "channel type not supported")
		return
	}

	upCh, upReqs, err := upstream.OpenChannel(newCh.ChannelType(), newCh.ExtraData())
	if err != nil {
		var oce *gossh.OpenChannelError
		if errors.As(err, &oce) {
			_ = newCh.Reject(oce.Reason, oce.Message)
		} else {
			_ = newCh.Reject(gossh.ConnectionFailed, err.Error())
		}
		return
	}
	clientCh, clientReqs, err := newCh.Accept()
	if err != nil {
		_ = upCh.Close()
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upCh, clientCh)
		_ = upCh.CloseWrite()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientCh, upCh)
		_ = clientCh.CloseWrite()
	}()

	// channel-scoped request forwarders, both directions
	go forwardChannelRequests(clientReqs, upCh, "client→upstream")
	go forwardChannelRequests(upReqs, clientCh, "upstream→client")

	wg.Wait()
	_ = clientCh.Close()
	_ = upCh.Close()
}

func forwardChannelRequests(in <-chan *gossh.Request, dst gossh.Channel, _ string) {
	for req := range in {
		ok, err := dst.SendRequest(req.Type, req.WantReply, req.Payload)
		if err != nil {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			continue
		}
		if req.WantReply {
			_ = req.Reply(ok, nil)
		}
	}
}

func splitHostPort(addr string) (string, string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, ""
	}
	return host, port
}
