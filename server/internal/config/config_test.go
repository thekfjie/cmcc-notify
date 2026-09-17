package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUsesEnvironmentSecrets(t *testing.T) {
	t.Setenv("TEST_CMCC_KEY", "ak_test")
	t.Setenv("TEST_AUTH", "local-token")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`listen: ":8080"
auth_token_env: TEST_AUTH
accounts:
  - name: primary
    enabled: true
    api_key_env: TEST_CMCC_KEY
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthToken != "local-token" || cfg.Accounts[0].APIKey != "ak_test" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.Redacted() != "primary=***" {
		t.Fatalf("redaction = %q", cfg.Redacted())
	}
}

func TestLoadRequiresAuthToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("accounts: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected auth token error")
	}
}

func TestLoadUsesSecretFiles(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth-token")
	keyPath := filepath.Join(dir, "cmcc-key")
	if err := os.WriteFile(authPath, []byte("file-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("ak_file-test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	content := "auth_token_file: " + authPath + "\naccounts:\n  - name: primary\n    enabled: true\n    api_key_file: " + keyPath + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthToken != "file-token" || cfg.Accounts[0].APIKey != "ak_file-test" {
		t.Fatalf("unexpected file secrets: %#v", cfg)
	}
}

func TestLoadHashesApplicationTokenFromEnvironment(t *testing.T) {
	const token = "cn_app_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	t.Setenv("TEST_APPLICATION_TOKEN", token)
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `auth_token: admin-secret
accounts:
  - name: primary
    enabled: false
applications:
  - name: monitoring
    enabled: true
    account: primary
    token_env: TEST_APPLICATION_TOKEN
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Applications) != 1 || cfg.Applications[0].TokenHash != HashApplicationToken(token) {
		t.Fatalf("applications = %#v", cfg.Applications)
	}
	if cfg.Applications[0].TokenHint != ApplicationTokenHint(token) || cfg.Applications[0].RateLimitPerMinute != 60 {
		t.Fatalf("application defaults = %#v", cfg.Applications[0])
	}
}

func TestResolveRejectsDuplicateApplicationTokens(t *testing.T) {
	const token = "cn_app_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	cfg := Config{
		Accounts: []Account{{Name: "primary", Enabled: false}},
		Applications: []Application{
			{Name: "one", Account: "primary", TokenHash: HashApplicationToken(token)},
			{Name: "two", Account: "primary", TokenHash: HashApplicationToken(token)},
		},
	}
	if err := cfg.ResolveAndValidate(); err == nil || !strings.Contains(err.Error(), "use the same token") {
		t.Fatalf("error = %v", err)
	}
}
