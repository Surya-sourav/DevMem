package state

import (
	"os"
	"testing"
)

func TestAPIKeysPerProvider(t *testing.T) {
	dir := t.TempDir()
	if err := SaveAPIKey(dir, "anthropic", "sk-ant"); err != nil {
		t.Fatal(err)
	}
	if err := SaveAPIKey(dir, "openai", "sk-oai"); err != nil {
		t.Fatal(err)
	}
	for provider, want := range map[string]string{"anthropic": "sk-ant", "openai": "sk-oai"} {
		got, err := LoadAPIKey(dir, provider)
		if err != nil || got != want {
			t.Errorf("LoadAPIKey(%s) = %q, %v; want %q", provider, got, err, want)
		}
	}
	info, err := os.Stat(CredentialsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("credentials mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestLoadAPIKeyLegacyFile(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDirs(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CredentialsPath(dir), []byte(`{"anthropic_api_key":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadAPIKey(dir, "anthropic"); got != "old" {
		t.Errorf("got %q", got)
	}
	if got, _ := LoadAPIKey(dir, "openai"); got != "" {
		t.Errorf("openai key should be empty, got %q", got)
	}
}
