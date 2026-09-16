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
  "name":"backup","api_key":"ak_backup","enabled":false,"default_to":"13800138000"
}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create account status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/groups", strings.NewReader(`{
  "name":"family","recipients":["13800138000","13900139000","13800138000"]
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
	if len(state.Accounts) != 1 || state.Accounts[0].Name != "backup" || state.Accounts[0].APIKey != "ak_backup" {
		t.Fatalf("persisted accounts = %#v", state.Accounts)
	}
	if len(state.Groups) != 1 || len(state.Groups[0].Recipients) != 2 {
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
	if !settings.Writable || settings.Accounts[0].APIKey != "ak_***ckup" || settings.Accounts[0].DefaultTo != "13800138000" {
		t.Fatalf("settings = %#v", settings)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/applications", strings.NewReader(`{
  "name":"monitoring","enabled":false,"account":"backup","group":"family","rate_limit_per_minute":30
}`))
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create application status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/v1/accounts/backup", strings.NewReader(`{
  "name":"primary","api_key":"","enabled":false,"default_to":"13800138000"
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
	if settings.Accounts[0].Name != "primary" || settings.Applications[0].Account != "primary" {
		t.Fatalf("renamed settings = %#v", settings)
	}

	state, err = config.Load(writeBaseConfig(t, statePath))
	if err != nil {
		t.Fatal(err)
	}
	if state.Accounts[0].Name != "primary" || state.Accounts[0].APIKey != "ak_backup" || state.Applications[0].Account != "primary" {
		t.Fatalf("persisted rename = %#v", state)
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
