package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/thekfjie/cmcc-notify/cmcc"
)

type AccountConfig struct {
	Name      string `yaml:"name"`
	Note      string `yaml:"note,omitempty"`
	APIKey    string `yaml:"api_key"`
	Enabled   bool   `yaml:"enabled"`
	UploadURL string `yaml:"upload_url"`
}

type RouteConfig struct {
	Applications    []uint   `yaml:"applications"`
	Accounts        []string `yaml:"accounts"`
	MinimumPriority int      `yaml:"minimum_priority"`
}

type Config struct {
	Enabled        bool            `yaml:"enabled"`
	GotifyURL      string          `yaml:"gotify_url"`
	ClientToken    string          `yaml:"client_token"`
	Accounts       []AccountConfig `yaml:"accounts"`
	Routes         []RouteConfig   `yaml:"routes"`
	IncludeTitle   bool            `yaml:"include_title"`
	TitleSeparator string          `yaml:"title_separator"`
}

func defaultConfig() *Config {
	return &Config{
		Enabled:        false,
		GotifyURL:      "http://127.0.0.1:80",
		TitleSeparator: "\n\n",
	}
}

func validateConfig(c *Config) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if c.GotifyURL == "" {
		c.GotifyURL = "http://127.0.0.1:80"
	}
	u, err := url.Parse(strings.TrimRight(c.GotifyURL, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("gotify_url must be an http(s) URL")
	}
	if c.TitleSeparator == "" {
		c.TitleSeparator = "\n\n"
	}
	names := make(map[string]struct{}, len(c.Accounts))
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
		if _, exists := names[a.Name]; exists {
			return fmt.Errorf("duplicate account name %q", a.Name)
		}
		names[a.Name] = struct{}{}
		if !cmcc.ValidAPIKey(a.APIKey) {
			return fmt.Errorf("account %q has an invalid api_key", a.Name)
		}
	}
	for _, route := range c.Routes {
		for _, account := range route.Accounts {
			if _, exists := names[account]; !exists {
				return fmt.Errorf("route references unknown account %q", account)
			}
		}
	}
	if c.Enabled && strings.TrimSpace(c.ClientToken) == "" {
		return fmt.Errorf("client_token is required when the forwarder is enabled")
	}
	return nil
}
