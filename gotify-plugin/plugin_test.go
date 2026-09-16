package main

import (
	"strings"
	"testing"

	gotifyplugin "github.com/gotify/plugin-api"
)

func TestValidateConfig(t *testing.T) {
	cfg := defaultConfig()
	cfg.Enabled = true
	cfg.ClientToken = "Cclient-token"
	cfg.Accounts = []AccountConfig{{Name: "primary", APIKey: "ak_test", Enabled: true, DefaultTo: "13800138000"}}
	if err := validateConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestDisplayUsesGotifyNativeMarkdownAndRedactsSecrets(t *testing.T) {
	plugin := &GotifyPlugin{
		config: &Config{
			Enabled: true,
			Accounts: []AccountConfig{{
				Name:      "primary",
				APIKey:    "ak_example-secret-value",
				Enabled:   true,
				DefaultTo: "13800138000",
			}},
			Routes: []RouteConfig{{Applications: []uint{1, 2}, Accounts: []string{"primary"}, MinimumPriority: 5}},
		},
		clients:  make(map[string]*accountClient),
		basePath: "/plugin/1/custom/token/",
	}
	plugin.stats.sent.Store(3)
	plugin.stats.failed.Store(1)
	plugin.stats.lastError.Store("temporary | gateway error")

	display := plugin.GetDisplay(nil)
	for _, expected := range []string{
		"## CMCC Notify",
		"| 已成功转发 | 3 |",
		"ak_***lue",
		"138****8000",
		"1, 2",
		"POST /plugin/1/custom/token/send",
	} {
		if !strings.Contains(display, expected) {
			t.Fatalf("display does not contain %q:\n%s", expected, display)
		}
	}
	for _, secret := range []string{"ak_example-secret-value", "13800138000"} {
		if strings.Contains(display, secret) {
			t.Fatalf("display leaked %q", secret)
		}
	}
}

func TestValidateConfigRejectsUnknownRouteAccount(t *testing.T) {
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "primary", APIKey: "ak_test", Enabled: true}}
	cfg.Routes = []RouteConfig{{Accounts: []string{"missing"}}}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected unknown account error")
	}
}

func TestExtractMedia(t *testing.T) {
	media, ok := extractMedia(map[string]interface{}{
		"cmcc::media": map[string]interface{}{"to": "13800138000", "type": "IMAGE", "url": "https://example.invalid/a.jpg"},
	})
	if !ok || media.URL == "" || media.Type != "IMAGE" || media.To != "13800138000" {
		t.Fatalf("media=%#v ok=%t", media, ok)
	}
}

func TestApplicationMatches(t *testing.T) {
	if !applicationMatches(nil, 42) || !applicationMatches([]uint{42}, 42) || applicationMatches([]uint{1}, 42) {
		t.Fatal("unexpected application matching")
	}
}

func TestPluginCapabilitiesAndDefaults(t *testing.T) {
	instance := NewGotifyPluginInstance(gotifyplugin.UserContext{ID: 1, Name: "alice"})
	if _, ok := instance.(gotifyplugin.Configurer); !ok {
		t.Fatal("plugin must implement Configurer")
	}
	if _, ok := instance.(gotifyplugin.Webhooker); !ok {
		t.Fatal("plugin must implement Webhooker")
	}
	if _, ok := instance.(gotifyplugin.Displayer); !ok {
		t.Fatal("plugin must implement Displayer")
	}
	if _, ok := instance.(gotifyplugin.Storager); !ok {
		t.Fatal("plugin must implement Storager")
	}
}

func TestCloneConfigDeepCopiesRoutes(t *testing.T) {
	original := &Config{
		Accounts: []AccountConfig{{Name: "primary", APIKey: "ak_test"}},
		Routes:   []RouteConfig{{Applications: []uint{1}, Accounts: []string{"primary"}}},
	}
	cloned := cloneConfig(original)
	original.Accounts[0].Name = "changed"
	original.Routes[0].Applications[0] = 2
	original.Routes[0].Accounts[0] = "changed"
	if cloned.Accounts[0].Name != "primary" || cloned.Routes[0].Applications[0] != 1 || cloned.Routes[0].Accounts[0] != "primary" {
		t.Fatalf("clone shares mutable config state: %#v", cloned)
	}
}
