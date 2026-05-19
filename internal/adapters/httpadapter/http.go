// Package httpadapter implements the HTTP adapter (Section 3.4.1).
//
// The listener accepts plain HTTP requests, validates the local-auth bearer
// token (when required), applies header/query injection templates against the
// stored secret values, and reverse-proxies the request to the upstream URL.
package httpadapter

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"text/template"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/store"
)

// Adapter is the HTTP adapter.
type Adapter struct{}

// New returns an Adapter ready to register.
func New() *Adapter { return &Adapter{} }

func (a *Adapter) Type() store.SecretType { return store.TypeHTTP }

// Config is the non-secret portion stored in config_json.
type Config struct {
	Upstream            string   `json:"upstream"`
	FollowRedirects     bool     `json:"follow_redirects"`
	StripRequestHeaders []string `json:"strip_request_headers"`
}

// Payload is the secret portion stored encrypted in secret_blob.
type Payload struct {
	Inject struct {
		Headers map[string]string `json:"headers"`
		Query   map[string]string `json:"query"`
	} `json:"inject"`
	Values map[string]string `json:"values"`
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
	return c, nil
}

// ValidateConfig implements adapters.Adapter.
func (a *Adapter) ValidateConfig(config map[string]any, secret map[string]any) error {
	c, err := decodeConfig(config)
	if err != nil {
		return fmt.Errorf("http: config: %w", err)
	}
	if c.Upstream == "" {
		return errors.New("http: upstream is required")
	}
	u, err := url.Parse(c.Upstream)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("http: upstream must be a valid http:// or https:// URL")
	}
	// secret is allowed to be empty.
	return nil
}

func parsePayload(sb []byte) (Payload, error) {
	var p Payload
	if len(sb) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(sb, &p); err != nil {
		return p, fmt.Errorf("http: payload: %w", err)
	}
	return p, nil
}

// ServeConnection serves a single TCP connection by running an http.Server
// scoped to that connection. Each adapter call thus owns one keep-alive
// session worth of HTTP traffic.
func (a *Adapter) ServeConnection(ctx context.Context, client net.Conn, dec adapters.DecryptedSecret, localCreds adapters.LocalCredentials) error {
	cfg, err := decodeConfig(dec.Config)
	if err != nil {
		return err
	}
	payload, err := parsePayload(dec.Payload.Bytes())
	if err != nil {
		return err
	}
	upstreamURL, err := url.Parse(cfg.Upstream)
	if err != nil {
		return err
	}

	handler := buildHandler(cfg, payload, upstreamURL, localCreds)

	// Use a one-shot listener that yields just this conn.
	ln := &oneShotListener{conn: client, done: make(chan struct{})}
	srv := &http.Server{Handler: handler}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, errOneShotDone) {
		return nil
	}
	return err
}

// ConnectionString returns the bare http://addr URL.
func (a *Adapter) ConnectionString(localAddr string, _ adapters.DecryptedSecret, _ adapters.LocalCredentials) string {
	return "http://" + localAddr
}

// EnvVars returns <NAME>_URL and (if local auth) <NAME>_TOKEN.
func (a *Adapter) EnvVars(localAddr string, _ adapters.DecryptedSecret, creds adapters.LocalCredentials, prefix string) map[string]string {
	out := map[string]string{
		prefix + "_URL": "http://" + localAddr,
	}
	if creds.Password != nil && creds.Password.Len() > 0 {
		out[prefix+"_TOKEN"] = creds.Password.String()
	}
	return out
}

// buildHandler constructs the per-secret reverse-proxy handler.
func buildHandler(cfg Config, payload Payload, upstreamURL *url.URL, localCreds adapters.LocalCredentials) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(upstreamURL)
	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		// Strip local auth header so it never reaches upstream.
		req.Header.Del("Authorization")
		req.Header.Del("X-Cloak-Token")
		for _, h := range cfg.StripRequestHeaders {
			req.Header.Del(h)
		}
		// Apply header injection.
		for k, tpl := range payload.Inject.Headers {
			v, err := renderTemplate(tpl, payload.Values)
			if err == nil {
				req.Header.Set(k, v)
			}
		}
		// Apply query injection.
		if len(payload.Inject.Query) > 0 {
			q := req.URL.Query()
			for k, tpl := range payload.Inject.Query {
				v, err := renderTemplate(tpl, payload.Values)
				if err == nil {
					q.Set(k, v)
				}
			}
			req.URL.RawQuery = q.Encode()
		}
		req.Host = upstreamURL.Host
	}
	if !cfg.FollowRedirects {
		rp.ModifyResponse = func(*http.Response) error { return nil }
	}
	// Default ErrorHandler logs to the standard logger; that pollutes the
	// daemon's stderr with noisy "context canceled" messages on connection
	// teardown. Use a quieter one that surfaces only an HTTP 502 to the client.
	rp.ErrorLog = log.New(io.Discard, "", 0)
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "upstream error", http.StatusBadGateway)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if localCreds.Password != nil && localCreds.Password.Len() > 0 {
			if !authorize(r, localCreds.Password.Bytes()) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="cloak"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		rp.ServeHTTP(w, r)
	})
}

func authorize(r *http.Request, password []byte) bool {
	if got := r.Header.Get("X-Cloak-Token"); got != "" {
		return subtle.ConstantTimeCompare([]byte(got), password) == 1
	}
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), password) == 1
}

func renderTemplate(tpl string, values map[string]string) (string, error) {
	t, err := template.New("inject").Option("missingkey=error").Parse(tpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := t.Execute(&sb, values); err != nil {
		return "", err
	}
	return sb.String(), nil
}
