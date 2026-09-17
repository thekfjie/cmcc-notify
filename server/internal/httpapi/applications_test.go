package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/thekfjie/cmcc-notify/server/internal/config"
)

func TestApplicationNotifyUsesScopedChannelAndRateLimit(t *testing.T) {
	const token = "cn_app_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	frames := make(chan map[string]any, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var auth map[string]any
		if conn.ReadJSON(&auth) != nil {
			return
		}
		if conn.WriteJSON(map[string]string{"type": "auth_ok"}) != nil {
			return
		}
		var frame map[string]any
		if conn.ReadJSON(&frame) == nil {
			frames <- frame
		}
	}))
	defer gateway.Close()

	api, err := New(config.Config{
		AuthToken: "admin-secret",
		Accounts: []config.Account{{
			Name: "primary", APIKey: "ak_test", Enabled: true,
			ServerURL: "ws" + strings.TrimPrefix(gateway.URL, "http"),
		}},
		Applications: []config.Application{{
			Name: "monitoring", Enabled: true, Account: "primary",
			RateLimitPerMinute: 1, TokenHash: config.HashApplicationToken(token), TokenHint: config.ApplicationTokenHint(token),
		}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	api.now = func() time.Time {
		return time.Date(2026, time.September, 17, 18, 30, 5, 0, time.FixedZone("CST", 8*60*60))
	}
	defer api.Close()

	request := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(`{"title":"Server Alert","message":"Disk usage is high"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result notifyResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Application != "monitoring" || !result.Accepted {
		t.Fatalf("result = %#v", result)
	}
	select {
	case frame := <-frames:
		if _, exists := frame["to"]; exists || frame["content"] != "Server Alert\n\nDisk usage is high\n\n发送时间：18:30:05" {
			t.Fatalf("frame = %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("missing application notification frame")
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(`{"message":"second"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit status = %d, headers = %v, body = %s", response.Code, response.Header(), response.Body.String())
	}
}

func TestApplicationSendTimeCanBeDisabled(t *testing.T) {
	now := time.Date(2026, time.September, 17, 18, 30, 5, 0, time.FixedZone("CST", 8*60*60))
	if got := withApplicationSendTime("hello", config.Application{}, now); got != "hello\n\n发送时间：18:30:05" {
		t.Fatalf("default send time = %q", got)
	}
	disabled := false
	if got := withApplicationSendTime("hello", config.Application{IncludeSendTime: &disabled}, now); got != "hello" {
		t.Fatalf("disabled send time = %q", got)
	}
}

func TestApplicationNotifyRejectsUnknownTargetFieldBeforeConnecting(t *testing.T) {
	const token = "cn_app_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	api, err := New(config.Config{
		AuthToken: "admin-secret",
		Accounts:  []config.Account{{Name: "primary", APIKey: "ak_test", Enabled: true}},
		Applications: []config.Application{{
			Name: "monitoring", Enabled: true, Account: "primary",
			RateLimitPerMinute: 60, TokenHash: config.HashApplicationToken(token), TokenHint: config.ApplicationTokenHint(token),
		}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(`{"message":"hello","to":"13900139000"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid JSON") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(api.clients) != 0 {
		t.Fatal("rejected application request created a CMCC client")
	}
}

func TestApplicationManagementStoresOnlyTokenHashAndRotates(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yaml")
	api, err := New(config.Config{
		AuthToken: "admin-secret", StateFile: statePath,
		Accounts: []config.Account{{Name: "primary", Enabled: false}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	handler := api.Handler()

	request := httptest.NewRequest(http.MethodPost, "/v1/applications", strings.NewReader(`{
  "name":"monitoring","enabled":false,"account":"primary","rate_limit_per_minute":30
}`))
	request.Header.Set("Authorization", "Bearer admin-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var created settingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	oldToken := created.ApplicationToken
	if !config.ValidApplicationToken(oldToken) || len(created.Applications) != 1 {
		t.Fatalf("created = %#v", created)
	}
	if !created.Applications[0].IncludeSendTime {
		t.Fatal("new application did not enable send time by default")
	}
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), oldToken) || !strings.Contains(string(state), "token_hash:") {
		t.Fatalf("state did not safely hash application token:\n%s", state)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/settings", nil)
	request.Header.Set("Authorization", "Bearer admin-secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), oldToken) || strings.Contains(response.Body.String(), "application_token") {
		t.Fatalf("settings leaked token: %s", response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/v1/applications/monitoring", strings.NewReader(`{
  "name":"monitoring","enabled":false,"account":"primary","include_send_time":false,"rate_limit_per_minute":30
}`))
	request.Header.Set("Authorization", "Bearer admin-secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", response.Code, response.Body.String())
	}
	var updated settingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Applications) != 1 || updated.Applications[0].IncludeSendTime {
		t.Fatalf("updated application = %#v", updated.Applications)
	}
	state, err = os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), "include_send_time: false") {
		t.Fatalf("state did not persist disabled send time:\n%s", state)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/applications/monitoring/rotate-token", nil)
	request.Header.Set("Authorization", "Bearer admin-secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("rotate status = %d, body = %s", response.Code, response.Body.String())
	}
	var rotated settingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	if !config.ValidApplicationToken(rotated.ApplicationToken) || rotated.ApplicationToken == oldToken {
		t.Fatalf("rotated token = %q", rotated.ApplicationToken)
	}

	for _, test := range []struct {
		token string
		want  int
	}{{oldToken, http.StatusUnauthorized}, {rotated.ApplicationToken, http.StatusForbidden}} {
		request = httptest.NewRequest(http.MethodPost, "/v1/notify", strings.NewReader(`{"message":"hello"}`))
		request.Header.Set("Authorization", "Bearer "+test.token)
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("token status = %d, want %d, body = %s", response.Code, test.want, response.Body.String())
		}
	}

	loaded, err := config.Load(writeBaseConfig(t, statePath))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Applications) != 1 || loaded.Applications[0].TokenHash != config.HashApplicationToken(rotated.ApplicationToken) || loaded.Applications[0].SendTimeEnabled() {
		t.Fatalf("loaded applications = %#v", loaded.Applications)
	}
}
