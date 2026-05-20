// Package envadapter implements the `env` adapter (Section 16) — a
// materialized secret type.
//
// Unlike the proxied adapters, an env secret has no network listener. It
// stores a flat key/value bag and, on materialization, delivers those values
// to a child process as environment variables and/or rendered files. The real
// secret reaches the client process: this is the weaker of Cloak's two secret
// tiers (Section 16.1).
package envadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/store"
)

// envNameRe matches a valid POSIX environment variable name.
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Adapter is the env adapter.
type Adapter struct{}

// New returns an Adapter ready to register.
func New() *Adapter { return &Adapter{} }

func (a *Adapter) Type() store.SecretType { return store.TypeEnv }

// Kind reports that env is a materialized adapter.
func (a *Adapter) Kind() adapters.AdapterKind { return adapters.KindMaterialized }

// FileSpec describes one file to render. The template is structure (non-secret)
// and lives in config_json; only its rendered output is secret.
type FileSpec struct {
	Basename string `json:"basename"`
	PathEnv  string `json:"path_env"`
	Template string `json:"template"`
}

// Config is the non-secret portion stored in config_json.
type Config struct {
	// InjectEnv controls whether the values map is injected as environment
	// variables. A nil pointer means unset and defaults to true.
	InjectEnv *bool      `json:"inject_env"`
	Files     []FileSpec `json:"files"`
}

func (c Config) injectEnv() bool { return c.InjectEnv == nil || *c.InjectEnv }

// Payload is the secret portion stored encrypted in secret_blob.
type Payload struct {
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

func decodePayloadMap(m map[string]any) (Payload, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return Payload{}, err
	}
	return parsePayload(data)
}

func parsePayload(sb []byte) (Payload, error) {
	var p Payload
	if len(sb) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(sb, &p); err != nil {
		return p, fmt.Errorf("env: payload: %w", err)
	}
	return p, nil
}

// ValidateConfig implements adapters.Adapter (Section 16.2.5).
func (a *Adapter) ValidateConfig(config map[string]any, secret map[string]any) error {
	cfg, err := decodeConfig(config)
	if err != nil {
		return fmt.Errorf("env: config: %w", err)
	}
	payload, err := decodePayloadMap(secret)
	if err != nil {
		return fmt.Errorf("env: %w", err)
	}
	if len(payload.Values) == 0 {
		return errors.New("env: at least one key/value pair is required")
	}
	for k := range payload.Values {
		if !envNameRe.MatchString(k) {
			return fmt.Errorf("env: %q is not a valid environment variable name", k)
		}
	}
	if !cfg.injectEnv() && len(cfg.Files) == 0 {
		return errors.New("env: secret delivers nothing — enable inject_env or define a file")
	}
	for i, f := range cfg.Files {
		if f.Basename == "" || filepath.Base(f.Basename) != f.Basename ||
			f.Basename == "." || f.Basename == ".." || strings.ContainsAny(f.Basename, `/\`) {
			return fmt.Errorf("env: files[%d].basename %q must be a single path element", i, f.Basename)
		}
		if !envNameRe.MatchString(f.PathEnv) {
			return fmt.Errorf("env: files[%d].path_env %q is not a valid environment variable name", i, f.PathEnv)
		}
		if _, err := renderTemplate(f.Template, payload.Values); err != nil {
			return fmt.Errorf("env: files[%d].template: %w", i, err)
		}
	}
	return nil
}

// Materialize implements adapters.MaterializingAdapter. It decrypts the secret
// into the values to inject and the files to render; the endpoint manager
// owns writing those files to disk.
func (a *Adapter) Materialize(dec adapters.DecryptedSecret) (adapters.Materialization, error) {
	cfg, err := decodeConfig(dec.Config)
	if err != nil {
		return adapters.Materialization{}, fmt.Errorf("env: config: %w", err)
	}
	payload, err := parsePayload(dec.Payload.Bytes())
	if err != nil {
		return adapters.Materialization{}, err
	}

	var mat adapters.Materialization
	if cfg.injectEnv() {
		mat.Env = make(map[string]string, len(payload.Values))
		for k, v := range payload.Values {
			mat.Env[k] = v
		}
	}
	for _, f := range cfg.Files {
		body, err := renderTemplate(f.Template, payload.Values)
		if err != nil {
			return adapters.Materialization{}, fmt.Errorf("env: render %q: %w", f.Basename, err)
		}
		mat.Files = append(mat.Files, adapters.RenderedFile{
			Basename: f.Basename,
			PathEnv:  f.PathEnv,
			Content:  secrets.NewFromString(body),
		})
	}
	return mat, nil
}

func renderTemplate(tpl string, values map[string]string) (string, error) {
	t, err := template.New("env").Option("missingkey=error").Parse(tpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := t.Execute(&sb, values); err != nil {
		return "", err
	}
	return sb.String(), nil
}
