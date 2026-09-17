package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thekfjie/cmcc-notify/cmcc"
	"gopkg.in/yaml.v3"
)

type Account struct {
	Name       string `yaml:"name"`
	Note       string `yaml:"note,omitempty"`
	APIKey     string `yaml:"api_key"`
	APIKeyEnv  string `yaml:"api_key_env"`
	APIKeyFile string `yaml:"api_key_file"`
	Enabled    bool   `yaml:"enabled"`
	ServerURL  string `yaml:"server_url"`
	UploadURL  string `yaml:"upload_url"`
}

type Group struct {
	Name     string   `yaml:"name" json:"name"`
	Channels []string `yaml:"channels" json:"channels"`
}

type Application struct {
	Name               string `yaml:"name"`
	Enabled            bool   `yaml:"enabled"`
	Account            string `yaml:"account,omitempty"`
	Group              string `yaml:"group,omitempty"`
	RateLimitPerMinute int    `yaml:"rate_limit_per_minute,omitempty"`
	TokenHash          string `yaml:"token_hash,omitempty"`
	TokenHint          string `yaml:"token_hint,omitempty"`
	TokenEnv           string `yaml:"token_env,omitempty"`
	TokenFile          string `yaml:"token_file,omitempty"`
}

type Config struct {
	Listen        string        `yaml:"listen"`
	AuthToken     string        `yaml:"auth_token"`
	AuthTokenEnv  string        `yaml:"auth_token_env"`
	AuthTokenFile string        `yaml:"auth_token_file"`
	StateFile     string        `yaml:"state_file"`
	Accounts      []Account     `yaml:"accounts"`
	Groups        []Group       `yaml:"groups"`
	Applications  []Application `yaml:"applications"`
}

type stateDocument struct {
	Accounts     []Account     `yaml:"accounts"`
	Groups       []Group       `yaml:"groups"`
	Applications []Application `yaml:"applications"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.AuthToken == "" && cfg.AuthTokenEnv != "" {
		cfg.AuthToken = os.Getenv(cfg.AuthTokenEnv)
	}
	if cfg.AuthToken == "" && cfg.AuthTokenFile != "" {
		cfg.AuthToken, err = readSecret(cfg.AuthTokenFile)
		if err != nil {
			return Config{}, fmt.Errorf("read auth token: %w", err)
		}
	}
	if cfg.AuthToken == "" {
		return Config{}, fmt.Errorf("auth_token, auth_token_env or auth_token_file is required")
	}
	if cfg.StateFile != "" {
		state, stateErr := loadState(cfg.StateFile)
		if stateErr != nil && !os.IsNotExist(stateErr) {
			return Config{}, fmt.Errorf("load state: %w", stateErr)
		}
		if stateErr == nil {
			cfg.Accounts = state.Accounts
			cfg.Groups = state.Groups
			cfg.Applications = state.Applications
		}
	}
	if err := cfg.ResolveAndValidate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) ResolveAndValidate() error {
	var err error
	seen := make(map[string]struct{}, len(c.Accounts))
	for i := range c.Accounts {
		a := &c.Accounts[i]
		a.Name = strings.TrimSpace(a.Name)
		a.Note = strings.TrimSpace(a.Note)
		if len(a.Note) > 500 {
			return fmt.Errorf("account %q note is too long", a.Name)
		}
		if a.Name == "" {
			a.Name = fmt.Sprintf("account-%d", i+1)
		}
		if _, ok := seen[a.Name]; ok {
			return fmt.Errorf("duplicate account %q", a.Name)
		}
		seen[a.Name] = struct{}{}
		if a.APIKey == "" && a.APIKeyEnv != "" {
			a.APIKey = os.Getenv(a.APIKeyEnv)
		}
		if a.APIKey == "" && a.APIKeyFile != "" {
			a.APIKey, err = readSecret(a.APIKeyFile)
			if err != nil {
				return fmt.Errorf("read account %q API key: %w", a.Name, err)
			}
		}
		if a.Enabled && !cmcc.ValidAPIKey(a.APIKey) {
			return fmt.Errorf("account %q has invalid or missing API key", a.Name)
		}
		if a.APIKey != "" && !cmcc.ValidAPIKey(a.APIKey) {
			return fmt.Errorf("account %q has an invalid API key", a.Name)
		}
	}
	groupNames := make(map[string]struct{}, len(c.Groups))
	for i := range c.Groups {
		group := &c.Groups[i]
		group.Name = strings.TrimSpace(group.Name)
		if group.Name == "" {
			return fmt.Errorf("group name is required")
		}
		if _, ok := groupNames[group.Name]; ok {
			return fmt.Errorf("duplicate group %q", group.Name)
		}
		groupNames[group.Name] = struct{}{}
		group.Channels = normalizeValues(group.Channels)
		if len(group.Channels) == 0 {
			return fmt.Errorf("group %q has no channels", group.Name)
		}
		for _, channel := range group.Channels {
			if _, ok := seen[channel]; !ok {
				return fmt.Errorf("group %q references unknown account %q", group.Name, channel)
			}
		}
	}
	applicationNames := make(map[string]struct{}, len(c.Applications))
	applicationTokenHashes := make(map[string]string, len(c.Applications))
	for i := range c.Applications {
		application := &c.Applications[i]
		application.Name = strings.TrimSpace(application.Name)
		application.Account = strings.TrimSpace(application.Account)
		application.Group = strings.TrimSpace(application.Group)
		application.TokenHash = strings.ToLower(strings.TrimSpace(application.TokenHash))
		application.TokenEnv = strings.TrimSpace(application.TokenEnv)
		application.TokenFile = strings.TrimSpace(application.TokenFile)
		if application.Name == "" {
			return fmt.Errorf("application name is required")
		}
		if _, ok := applicationNames[application.Name]; ok {
			return fmt.Errorf("duplicate application %q", application.Name)
		}
		applicationNames[application.Name] = struct{}{}
		if (application.Account == "") == (application.Group == "") {
			return fmt.Errorf("application %q must configure exactly one of account or group", application.Name)
		}
		if application.Account != "" {
			if _, ok := seen[application.Account]; !ok {
				return fmt.Errorf("application %q references unknown account %q", application.Name, application.Account)
			}
		}
		if application.Group != "" {
			if _, ok := groupNames[application.Group]; !ok {
				return fmt.Errorf("application %q references unknown group %q", application.Name, application.Group)
			}
		}
		var token string
		if application.TokenEnv != "" {
			token = os.Getenv(application.TokenEnv)
		}
		if token == "" && application.TokenFile != "" {
			token, err = readSecret(application.TokenFile)
			if err != nil {
				return fmt.Errorf("read application %q token: %w", application.Name, err)
			}
		}
		if token != "" {
			if !ValidApplicationToken(token) {
				return fmt.Errorf("application %q has an invalid token", application.Name)
			}
			application.TokenHash = HashApplicationToken(token)
			application.TokenHint = ApplicationTokenHint(token)
		}
		if !validTokenHash(application.TokenHash) {
			return fmt.Errorf("application %q has an invalid or missing token hash", application.Name)
		}
		if existing, ok := applicationTokenHashes[application.TokenHash]; ok {
			return fmt.Errorf("applications %q and %q use the same token", existing, application.Name)
		}
		applicationTokenHashes[application.TokenHash] = application.Name
		if application.TokenHint == "" {
			application.TokenHint = "cn_app_…"
		}
		if application.RateLimitPerMinute == 0 {
			application.RateLimitPerMinute = 60
		}
		if application.RateLimitPerMinute < 1 || application.RateLimitPerMinute > 1000 {
			return fmt.Errorf("application %q rate_limit_per_minute must be between 1 and 1000", application.Name)
		}
	}
	return nil
}

func loadState(path string) (stateDocument, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return stateDocument{}, err
	}
	var state stateDocument
	if err := yaml.Unmarshal(b, &state); err != nil {
		return stateDocument{}, fmt.Errorf("parse state: %w", err)
	}
	return state, nil
}

func (c Config) SaveState() error {
	if strings.TrimSpace(c.StateFile) == "" {
		return fmt.Errorf("state_file is required for WebUI changes")
	}
	state := stateDocument{
		Accounts: make([]Account, len(c.Accounts)), Groups: c.Groups,
		Applications: make([]Application, len(c.Applications)),
	}
	copy(state.Accounts, c.Accounts)
	copy(state.Applications, c.Applications)
	for i := range state.Accounts {
		if state.Accounts[i].APIKeyFile != "" || state.Accounts[i].APIKeyEnv != "" {
			state.Accounts[i].APIKey = ""
		}
	}
	for i := range state.Applications {
		if state.Applications[i].TokenFile != "" || state.Applications[i].TokenEnv != "" {
			state.Applications[i].TokenHash = ""
		}
	}
	b, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	dir := filepath.Dir(c.StateFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".cmcc-notify-state-*")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(tmpPath, c.StateFile); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

func normalizeValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func readSecret(path string) (string, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func (c Config) Account(name string) (Account, bool) {
	for _, account := range c.Accounts {
		if account.Name == name {
			return account, true
		}
	}
	return Account{}, false
}

func (c Config) Group(name string) (Group, bool) {
	for _, group := range c.Groups {
		if group.Name == name {
			return group, true
		}
	}
	return Group{}, false
}

func (c Config) Application(name string) (Application, bool) {
	for _, application := range c.Applications {
		if application.Name == name {
			return application, true
		}
	}
	return Application{}, false
}

func HashApplicationToken(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func ValidApplicationToken(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "cn_app_") && len(value) >= 39
}

func ApplicationTokenHint(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 12 {
		return "cn_app_…"
	}
	return value[:10] + "…" + value[len(value)-4:]
}

func validTokenHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func (c Config) Redacted() string {
	values := make([]string, 0, len(c.Accounts))
	for _, account := range c.Accounts {
		key := account.APIKey
		if len(key) >= 8 {
			key = key[:3] + "***" + key[len(key)-3:]
		} else if key != "" {
			key = "***"
		}
		values = append(values, account.Name+"="+key)
	}
	return strings.Join(values, ", ")
}
