package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thekfjie/cmcc-notify/server/internal/config"
)

func TestManageAccountsAndGroupsPersistsState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yaml")
	api, err := New(config.Config{AuthToken: "secret", StateFile: statePath}, "test")
	if err != nil {
		t.Fatal(err)
	}
	handler := api.Handler()

	request := httptest.NewRequest(http.MethodPost, "/v1/accounts", strings.NewReader(`{
  "name":"backup","note":"备用通知通道",
  "api_key":"【新消息Claw】请根据 https://example.invalid/channel-guide.md 安装插件。我的新消息Channel API Key为ak_backup",
  "enabled":false
}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create account status = %d, body = %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/accounts", strings.NewReader(`{
  "name":"secondary","note":"第二位接收者","api_key":"ak_secondary","enabled":false
}`))
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create second account status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/groups", strings.NewReader(`{
  "name":"family","channels":["backup","secondary","backup"]
}`))
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create group status = %d, body = %s", response.Code, response.Body.String())
	}

	state, err := config.Load(writeBaseConfig(t, statePath))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Accounts) != 2 || state.Accounts[0].Name != "backup" || state.Accounts[0].Note != "备用通知通道" || state.Accounts[0].APIKey != "ak_backup" {
		t.Fatalf("persisted accounts = %#v", state.Accounts)
	}
	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stateBytes), "新消息Claw") || strings.Contains(string(stateBytes), "channel-guide.md") {
		t.Fatal("authorization message text was persisted instead of the normalized API key")
	}
	if len(state.Groups) != 1 || len(state.Groups[0].Channels) != 2 {
		t.Fatalf("persisted groups = %#v", state.Groups)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/settings", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("settings status = %d", response.Code)
	}
	var settings settingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if !settings.Writable || settings.Accounts[0].APIKey != "ak_***ckup" || settings.Accounts[0].Note != "备用通知通道" {
		t.Fatalf("settings = %#v", settings)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/applications", strings.NewReader(`{
  "name":"monitoring","enabled":false,"group":"family","rate_limit_per_minute":30
}`))
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create application status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/v1/accounts/backup", strings.NewReader(`{
  "name":"primary","note":"主通知通道","api_key":"","enabled":false
}`))
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("rename account status = %d, body = %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Accounts[0].Name != "primary" || settings.Accounts[0].Note != "主通知通道" || settings.Groups[0].Channels[0] != "primary" || settings.Applications[0].Group != "family" {
		t.Fatalf("renamed settings = %#v", settings)
	}

	state, err = config.Load(writeBaseConfig(t, statePath))
	if err != nil {
		t.Fatal(err)
	}
	if state.Accounts[0].Name != "primary" || state.Accounts[0].APIKey != "ak_backup" || state.Groups[0].Channels[0] != "primary" || state.Applications[0].Group != "family" {
		t.Fatalf("persisted rename = %#v", state)
	}
}

func TestAccountMutationRejectsInvalidAuthorizationTextWithoutEchoingIt(t *testing.T) {
	api, err := New(config.Config{AuthToken: "secret", StateFile: filepath.Join(t.TempDir(), "state.yaml")}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	const invalid = "【新消息Claw】授权完成，但消息中没有可用密钥"
	request := httptest.NewRequest(http.MethodPost, "/v1/accounts", strings.NewReader(`{
  "name":"invalid","api_key":"`+invalid+`","enabled":true
}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid api_key") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), invalid) {
		t.Fatal("invalid authorization text was echoed in the response")
	}
}

func writeBaseConfig(t *testing.T, statePath string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "auth_token: secret\nstate_file: " + statePath + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
