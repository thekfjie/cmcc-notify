package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	gotifyplugin "github.com/gotify/plugin-api"
	"github.com/thekfjie/cmcc-notify/cmcc"
)

func TestValidateConfig(t *testing.T) {
	cfg := defaultConfig()
	cfg.Enabled = true
	cfg.ClientToken = "Cclient-token"
	cfg.Accounts = []AccountConfig{{Name: "primary", Note: "主通道", APIKey: "ak_test", Enabled: true}}
	if err := validateConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestDisplayUsesGotifyNativeMarkdownAndRedactsSecrets(t *testing.T) {
	plugin := &GotifyPlugin{
		config: &Config{
			Enabled: true,
			Accounts: []AccountConfig{{
				Name:    "primary",
				Note:    "主通道",
				APIKey:  "ak_example-secret-value",
				Enabled: true,
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
		"主通道",
		"1, 2",
		"POST /plugin/1/custom/token/send",
	} {
		if !strings.Contains(display, expected) {
			t.Fatalf("display does not contain %q:\n%s", expected, display)
		}
	}
	for _, secret := range []string{"ak_example-secret-value"} {
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
		"cmcc::media": map[string]interface{}{"type": "IMAGE", "url": "https://example.invalid/a.jpg"},
	})
	if !ok || media.URL == "" || media.Type != "IMAGE" {
		t.Fatalf("media=%#v ok=%t", media, ok)
	}
}

func TestMediaForwardSendsCompanionTextBeforeMedia(t *testing.T) {
	frames := make(chan []map[string]any, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var auth map[string]any
		if conn.ReadJSON(&auth) != nil || conn.WriteJSON(map[string]string{"type": "auth_ok"}) != nil {
			return
		}
		pair := make([]map[string]any, 0, 2)
		for i := 0; i < 2; i++ {
			var frame map[string]any
			if conn.ReadJSON(&frame) != nil {
				return
			}
			pair = append(pair, frame)
		}
		frames <- pair
	}))
	defer gateway.Close()

	client, err := cmcc.NewClient("ak_test", cmcc.Config{
		ServerURL:      "ws" + strings.TrimPrefix(gateway.URL, "http"),
		HeartbeatEvery: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	plugin := &GotifyPlugin{clients: map[string]*accountClient{"primary": {client: client}}}
	cfg := &Config{Accounts: []AccountConfig{{Name: "primary", APIKey: "ak_test", Enabled: true}}}
	message := gotifyMessage{Message: "deployment report", Extras: map[string]interface{}{
		"cmcc::media": map[string]interface{}{
			"type": "IMAGE", "url": "https://cdn.example.invalid/report.jpg", "caption": "release complete",
		},
	}}
	if err := plugin.forwardToAccount(context.Background(), message, "primary", cfg); err != nil {
		t.Fatal(err)
	}

	select {
	case pair := <-frames:
		if pair[0]["content"] != "release complete" || pair[0]["mediaType"] != nil {
			t.Fatalf("companion text frame = %#v", pair[0])
		}
		if pair[1]["mediaType"] != "IMAGE" || pair[1]["mediaUrl"] != "https://cdn.example.invalid/report.jpg" {
			t.Fatalf("media frame = %#v", pair[1])
		}
		if _, exists := pair[1]["content"]; exists {
			t.Fatalf("media frame must omit companion text: %#v", pair[1])
		}
	case <-time.After(time.Second):
		t.Fatal("missing companion text or media frame")
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
