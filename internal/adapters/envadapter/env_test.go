package envadapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/store"
)

func TestValidateConfig(t *testing.T) {
	a := New()
	values := map[string]any{"values": map[string]any{
		"AWS_ACCESS_KEY_ID":     "AKIA",
		"AWS_SECRET_ACCESS_KEY": "sk",
	}}

	t.Run("ok env only", func(t *testing.T) {
		if err := a.ValidateConfig(map[string]any{}, values); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty values rejected", func(t *testing.T) {
		if err := a.ValidateConfig(map[string]any{}, map[string]any{"values": map[string]any{}}); err == nil {
			t.Fatal("expected error for empty values")
		}
	})

	t.Run("bad env name rejected", func(t *testing.T) {
		bad := map[string]any{"values": map[string]any{"1BAD": "x"}}
		if err := a.ValidateConfig(map[string]any{}, bad); err == nil {
			t.Fatal("expected error for invalid env var name")
		}
	})

	t.Run("delivers nothing rejected", func(t *testing.T) {
		cfg := map[string]any{"inject_env": false}
		if err := a.ValidateConfig(cfg, values); err == nil {
			t.Fatal("expected error when inject_env=false and no files")
		}
	})

	t.Run("ok with file", func(t *testing.T) {
		cfg := map[string]any{
			"inject_env": false,
			"files": []any{map[string]any{
				"basename": "credentials",
				"path_env": "AWS_SHARED_CREDENTIALS_FILE",
				"template": "[default]\nkey={{ .AWS_ACCESS_KEY_ID }}\n",
			}},
		}
		if err := a.ValidateConfig(cfg, values); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("template referencing missing key rejected", func(t *testing.T) {
		cfg := map[string]any{
			"files": []any{map[string]any{
				"basename": "f",
				"path_env": "F",
				"template": "{{ .NOT_THERE }}",
			}},
		}
		if err := a.ValidateConfig(cfg, values); err == nil {
			t.Fatal("expected error for missing template key")
		}
	})

	t.Run("bad basename rejected", func(t *testing.T) {
		cfg := map[string]any{
			"files": []any{map[string]any{
				"basename": "../escape",
				"path_env": "F",
				"template": "x",
			}},
		}
		if err := a.ValidateConfig(cfg, values); err == nil {
			t.Fatal("expected error for path-traversing basename")
		}
	})
}

func TestMaterialize(t *testing.T) {
	a := New()
	payload, _ := json.Marshal(Payload{Values: map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIA",
		"AWS_SECRET_ACCESS_KEY": "sk",
	}})
	sb := secrets.NewFromString(string(payload))
	defer sb.Zero()

	dec := adapters.DecryptedSecret{
		ID:   "01",
		Name: "aws",
		Type: store.TypeEnv,
		Config: map[string]any{
			"files": []any{map[string]any{
				"basename": "credentials",
				"path_env": "AWS_SHARED_CREDENTIALS_FILE",
				"template": "[default]\naws_access_key_id={{ .AWS_ACCESS_KEY_ID }}\n",
			}},
		},
		Payload: sb,
	}

	mat, err := a.Materialize(dec)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if mat.Env["AWS_ACCESS_KEY_ID"] != "AKIA" {
		t.Fatalf("env not injected: %v", mat.Env)
	}
	if len(mat.Files) != 1 {
		t.Fatalf("want 1 file, got %d", len(mat.Files))
	}
	f := mat.Files[0]
	if f.Basename != "credentials" || f.PathEnv != "AWS_SHARED_CREDENTIALS_FILE" {
		t.Fatalf("file spec wrong: %+v", f)
	}
	if got := string(f.Content.Bytes()); !strings.Contains(got, "aws_access_key_id=AKIA") {
		t.Fatalf("rendered file wrong: %q", got)
	}
	f.Content.Zero()
}
