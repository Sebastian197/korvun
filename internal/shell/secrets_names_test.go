// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// B10 RED — name discovery + the folder opener (spec
// 2026-08-29-b10-secrets-panel.md FR-B10-1/2). Names only, ever: the
// binding's return type cannot carry a value, and discovery must work with
// the controller PATH-LESS — the star case is a boot that failed on a
// broken config, which is exactly when the panel is needed.

// b10Config references all four secret-name kinds, one duplicated, one
// model without a key — and is VALID-SHAPED but deliberately not required
// to pass config.Validate (parse-only discovery).
const b10Config = `{
  "channels": [
    {"type": "telegram", "mode": "polling", "token_env": "KORVUN_TG_TOKEN"},
    {"type": "webhook", "token_env": "HOOK_IN_TOKEN",
     "webhook": {"outbound_url": "http://127.0.0.1:9/x", "outbound_token_env": "HOOK_OUT_TOKEN"}}
  ],
  "brains": [
    {"name": "a", "sensitivity": "public", "policy": {"kind": "priority"}, "dispatch": "fanout",
     "models": [
       {"provider": "groq", "model_id": "m1", "locality": "cloud", "api_key_env": "GROQ_API_KEY"},
       {"provider": "ollama", "model_id": "m2", "locality": "local"},
       {"provider": "openai-compatible", "model_id": "m3", "locality": "cloud",
        "base_url": "https://x/v1", "api_key_env": "GROQ_API_KEY"}
     ]}
  ],
  "routes": [{"channel": "telegram", "brain": "a"}],
  "admin": {"token_env": "KORVUN_ADMIN_TOKEN"}
}`

func writeB10Config(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestListSecretNames_discoversAllKindsDeduplicated(t *testing.T) {
	path := writeB10Config(t, b10Config)
	// The star shape: the controller holds NO path (broken-boot state);
	// discovery rides the fallback seam.
	d := testDesktop(testController(), withConfigPathFallback(func() (string, error) {
		return path, nil
	}))

	names, err := d.ListSecretNames()
	if err != nil {
		t.Fatalf("ListSecretNames: %v", err)
	}
	want := []string{"KORVUN_TG_TOKEN", "HOOK_IN_TOKEN", "HOOK_OUT_TOKEN", "GROQ_API_KEY", "KORVUN_ADMIN_TOKEN"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want appearance-ordered deduplicated %v", names, want)
	}
}

func TestListSecretNames_parseOnly_invalidButParseableStillYieldsNames(t *testing.T) {
	// No brains at all — config.Validate would reject this file; the panel
	// still needs the channel's name row.
	path := writeB10Config(t, `{"channels":[{"type":"telegram","mode":"polling","token_env":"KORVUN_TG_TOKEN"}]}`)
	d := testDesktop(testController(), withConfigPathFallback(func() (string, error) {
		return path, nil
	}))
	names, err := d.ListSecretNames()
	if err != nil {
		t.Fatalf("ListSecretNames: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"KORVUN_TG_TOKEN"}) {
		t.Fatalf("names = %v, want the channel token", names)
	}
}

func TestListSecretNames_unreadableConfigErrors(t *testing.T) {
	path := writeB10Config(t, `{not json`)
	d := testDesktop(testController(), withConfigPathFallback(func() (string, error) {
		return path, nil
	}))
	if _, err := d.ListSecretNames(); err == nil {
		t.Fatalf("ListSecretNames on an unparseable config = nil error, want the sealed-notice error")
	}
}

func TestOpenConfigFolder_opensTheConfigDirectory(t *testing.T) {
	path := writeB10Config(t, b10Config)
	opened := ""
	d := testDesktop(testController(),
		withConfigPathFallback(func() (string, error) { return path, nil }),
		withFolderOpener(func(dir string) error {
			opened = dir
			return nil
		}))
	if err := d.OpenConfigFolder(); err != nil {
		t.Fatalf("OpenConfigFolder: %v", err)
	}
	if opened != filepath.Dir(path) {
		t.Fatalf("opened %q, want the config dir %q", opened, filepath.Dir(path))
	}
}
