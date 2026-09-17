package httpapi

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thekfjie/cmcc-notify/server/internal/config"
)

func TestHealthAndAuth(t *testing.T) {
	api, err := New(config.Config{AuthToken: "secret"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	handler := api.Handler()

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}

	unauthorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/send", strings.NewReader(`{"text":"hello"}`))
	handler.ServeHTTP(unauthorized, request)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
}

func TestStatusIsAuthenticatedAndRedacted(t *testing.T) {
	api, err := New(config.Config{
		AuthToken: "secret",
		Accounts: []config.Account{{
			Name:    "primary",
			Note:    "主用通知通道",
			APIKey:  "ak_example-secret-value",
			Enabled: true,
		}},
	}, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	handler := api.Handler()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status cache control = %q", response.Header().Get("Cache-Control"))
	}
	var body statusResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Version != "1.2.3" || len(body.Accounts) != 1 {
		t.Fatalf("unexpected response: %+v", body)
	}
	account := body.Accounts[0]
	if account.Connected {
		t.Fatal("unstarted account reported connected")
	}
	if strings.Contains(response.Body.String(), "ak_example-secret-value") || account.APIKey != "ak_***alue" {
		t.Fatalf("API key was not safely redacted: %q", account.APIKey)
	}
	if account.Note != "主用通知通道" {
		t.Fatalf("note = %q", account.Note)
	}
}

func TestWebInterfaceAndSecurityHeaders(t *testing.T) {
	api, err := New(config.Config{AuthToken: "secret"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	handler := api.Handler()

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("index status = %d, body = %q", index.Code, index.Body.String())
	}
	if index.Header().Get("Content-Security-Policy") == "" || index.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("security headers missing: %v", index.Header())
	}

	assets, err := fs.Glob(embeddedWeb, "web/assets/*.js")
	if err != nil || len(assets) == 0 {
		t.Fatalf("find embedded JavaScript: %v, %v", assets, err)
	}
	assetPath := strings.TrimPrefix(assets[0], "web")
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, assetPath, nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status = %d", asset.Code)
	}
	if !strings.Contains(asset.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset cache control = %q", asset.Header().Get("Cache-Control"))
	}

	qr := httptest.NewRecorder()
	handler.ServeHTTP(qr, httptest.NewRequest(http.MethodGet, "/cmcc-new-message-activation.svg", nil))
	if qr.Code != http.StatusOK || !strings.Contains(qr.Body.String(), "<svg") {
		t.Fatalf("QR asset status = %d, body = %q", qr.Code, qr.Body.String())
	}
	if qr.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("QR content type = %q", qr.Header().Get("Content-Type"))
	}
}

func TestSendValidatesBeforeConnecting(t *testing.T) {
	api, err := New(config.Config{
		AuthToken: "secret",
		Accounts:  []config.Account{{Name: "primary", APIKey: "ak_test", Enabled: true}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	handler := api.Handler()

	tests := []struct {
		body string
		want string
	}{
		{body: `{"account":"primary","text":" "}`, want: "text is required"},
		{body: `{"account":"primary","media":{"type":"IMAGE","url":" "}}`, want: "media.url is required"},
		{body: `{"account":"primary","media":{"type":"IMAGE","url":"file:///tmp/a.jpg"}}`, want: "media.url must use http or https"},
		{body: `{"account":"primary","to":"13800138000","text":"hello"}`, want: "invalid JSON"},
		{body: `{"group":"missing","text":"hello"}`, want: "unknown group"},
		{body: `{"account":"primary","group":"missing","text":"hello"}`, want: "account and group cannot be used together"},
		{body: `{"account":"missing","text":"hello"}`, want: "unknown or disabled channel"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodPost, "/v1/send", strings.NewReader(test.body))
		request.Header.Set("Authorization", "Bearer secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.want) {
			t.Fatalf("body %s: status = %d, response = %s", test.body, response.Code, response.Body.String())
		}
	}
	if len(api.clients) != 0 {
		t.Fatal("validation failure created a CMCC client")
	}
}
