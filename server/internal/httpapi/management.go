package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/thekfjie/cmcc-notify/cmcc"
	"github.com/thekfjie/cmcc-notify/server/internal/config"
)

type accountSetting struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	APIKey    string `json:"api_key"`
	HasAPIKey bool   `json:"has_api_key"`
	DefaultTo string `json:"default_to,omitempty"`
	ServerURL string `json:"server_url,omitempty"`
	UploadURL string `json:"upload_url,omitempty"`
}

type settingsResponse struct {
	Writable         bool                 `json:"writable"`
	Accounts         []accountSetting     `json:"accounts"`
	Groups           []config.Group       `json:"groups"`
	Applications     []applicationSetting `json:"applications"`
	ApplicationToken string               `json:"application_token,omitempty"`
}

type accountMutation struct {
	Name      string `json:"name"`
	APIKey    string `json:"api_key"`
	Enabled   bool   `json:"enabled"`
	DefaultTo string `json:"default_to"`
	ServerURL string `json:"server_url"`
	UploadURL string `json:"upload_url"`
}

type groupMutation struct {
	Name       string   `json:"name"`
	Recipients []string `json:"recipients"`
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.writeSettings(w, http.StatusOK, cfg)
}

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request accountMutation
	if err := decodeManagementJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if err := validateAccountMutation(request, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.updateConfig(true, func(next *config.Config) error {
		if _, exists := next.Account(request.Name); exists {
			return fmt.Errorf("account already exists")
		}
		next.Accounts = append(next.Accounts, config.Account{
			Name: request.Name, APIKey: strings.TrimSpace(request.APIKey), Enabled: request.Enabled,
			DefaultTo: strings.TrimSpace(request.DefaultTo), ServerURL: strings.TrimSpace(request.ServerURL),
			UploadURL: strings.TrimSpace(request.UploadURL),
		})
		return nil
	}); err != nil {
		writeMutationError(w, err)
		return
	}
	s.writeSettings(w, http.StatusCreated, s.configSnapshot())
}

func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	name, err := resourceName(r, "/v1/accounts/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch r.Method {
	case http.MethodPut:
		var request accountMutation
		if err := decodeManagementJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		request.Name = strings.TrimSpace(request.Name)
		if request.Name == "" {
			request.Name = name
		}
		if err := validateAccountMutation(request, false); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.updateConfig(true, func(next *config.Config) error {
			if request.Name != name {
				if _, exists := next.Account(request.Name); exists {
					return fmt.Errorf("account already exists")
				}
			}
			for i := range next.Accounts {
				if next.Accounts[i].Name != name {
					continue
				}
				account := &next.Accounts[i]
				account.Name = request.Name
				if key := strings.TrimSpace(request.APIKey); key != "" {
					account.APIKey = key
					account.APIKeyEnv = ""
					account.APIKeyFile = ""
				}
				account.Enabled = request.Enabled
				account.DefaultTo = strings.TrimSpace(request.DefaultTo)
				account.ServerURL = strings.TrimSpace(request.ServerURL)
				account.UploadURL = strings.TrimSpace(request.UploadURL)
				if request.Name != name {
					for j := range next.Applications {
						if next.Applications[j].Account == name {
							next.Applications[j].Account = request.Name
						}
					}
				}
				return nil
			}
			return fmt.Errorf("account not found")
		}); err != nil {
			writeMutationError(w, err)
			return
		}
		s.writeSettings(w, http.StatusOK, s.configSnapshot())
	case http.MethodDelete:
		if err := s.updateConfig(true, func(next *config.Config) error {
			for i := range next.Accounts {
				if next.Accounts[i].Name == name {
					next.Accounts = append(next.Accounts[:i], next.Accounts[i+1:]...)
					return nil
				}
			}
			return fmt.Errorf("account not found")
		}); err != nil {
			writeMutationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) groups(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request groupMutation
	if err := decodeManagementJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if err := validateResourceName(request.Name, "group"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.updateConfig(false, func(next *config.Config) error {
		if _, exists := next.Group(request.Name); exists {
			return fmt.Errorf("group already exists")
		}
		next.Groups = append(next.Groups, config.Group{Name: request.Name, Recipients: request.Recipients})
		return nil
	}); err != nil {
		writeMutationError(w, err)
		return
	}
	s.writeSettings(w, http.StatusCreated, s.configSnapshot())
}

func (s *Server) group(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	name, err := resourceName(r, "/v1/groups/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch r.Method {
	case http.MethodPut:
		var request groupMutation
		if err := decodeManagementJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if request.Name != "" && strings.TrimSpace(request.Name) != name {
			writeError(w, http.StatusBadRequest, "group name cannot be changed")
			return
		}
		if err := s.updateConfig(false, func(next *config.Config) error {
			for i := range next.Groups {
				if next.Groups[i].Name == name {
					next.Groups[i].Recipients = request.Recipients
					return nil
				}
			}
			return fmt.Errorf("group not found")
		}); err != nil {
			writeMutationError(w, err)
			return
		}
		s.writeSettings(w, http.StatusOK, s.configSnapshot())
	case http.MethodDelete:
		if err := s.updateConfig(false, func(next *config.Config) error {
			for i := range next.Groups {
				if next.Groups[i].Name == name {
					next.Groups = append(next.Groups[:i], next.Groups[i+1:]...)
					return nil
				}
			}
			return fmt.Errorf("group not found")
		}); err != nil {
			writeMutationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) updateConfig(rebuild bool, mutate func(*config.Config) error) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	next := s.configSnapshot()
	if err := mutate(&next); err != nil {
		return err
	}
	if err := next.ResolveAndValidate(); err != nil {
		return err
	}
	if err := next.SaveState(); err != nil {
		return err
	}
	s.cfgMu.Lock()
	s.cfg = next
	s.cfgMu.Unlock()
	if rebuild {
		return s.rebuildClients()
	}
	return nil
}

func (s *Server) writeSettings(w http.ResponseWriter, status int, cfg config.Config) {
	writeJSON(w, status, s.settingsResponse(cfg))
}

func (s *Server) writeSettingsWithApplicationToken(w http.ResponseWriter, status int, cfg config.Config, token string) {
	response := s.settingsResponse(cfg)
	response.ApplicationToken = token
	writeJSON(w, status, response)
}

func (s *Server) settingsResponse(cfg config.Config) settingsResponse {
	s.mu.Lock()
	clients := make(map[string]*cmcc.Client, len(s.clients))
	for name, client := range s.clients {
		clients[name] = client
	}
	s.mu.Unlock()
	response := settingsResponse{Writable: strings.TrimSpace(cfg.StateFile) != "", Groups: cfg.Groups}
	response.Accounts = make([]accountSetting, 0, len(cfg.Accounts))
	for _, account := range cfg.Accounts {
		client := clients[account.Name]
		response.Accounts = append(response.Accounts, accountSetting{
			Name: account.Name, Enabled: account.Enabled,
			Connected: account.Enabled && client != nil && client.Connected(),
			APIKey:    maskAPIKey(account.APIKey), HasAPIKey: account.APIKey != "",
			DefaultTo: account.DefaultTo, ServerURL: account.ServerURL, UploadURL: account.UploadURL,
		})
	}
	response.Applications = make([]applicationSetting, 0, len(cfg.Applications))
	for _, application := range cfg.Applications {
		response.Applications = append(response.Applications, applicationSetting{
			Name: application.Name, Enabled: application.Enabled, Account: application.Account,
			To: application.To, Group: application.Group,
			AllowRecipientOverride: application.AllowRecipientOverride,
			RateLimitPerMinute:     application.RateLimitPerMinute, TokenHint: application.TokenHint,
		})
	}
	return response
}

func decodeManagementJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON")
	}
	return nil
}

func validateAccountMutation(request accountMutation, creating bool) error {
	if err := validateResourceName(request.Name, "account"); err != nil {
		return err
	}
	key := strings.TrimSpace(request.APIKey)
	if creating && request.Enabled && key == "" {
		return fmt.Errorf("api_key is required for an enabled account")
	}
	if key != "" && !cmcc.ValidAPIKey(key) {
		return fmt.Errorf("invalid api_key")
	}
	if err := validateOptionalURL(request.ServerURL, "server_url", "ws", "wss"); err != nil {
		return err
	}
	return validateOptionalURL(request.UploadURL, "upload_url", "http", "https")
}

func validateOptionalURL(value, field string, schemes ...string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("invalid %s", field)
	}
	for _, scheme := range schemes {
		if parsed.Scheme == scheme {
			return nil
		}
	}
	return fmt.Errorf("invalid %s scheme", field)
}

func resourceName(r *http.Request, prefix string) (string, error) {
	raw := strings.TrimPrefix(r.URL.EscapedPath(), prefix)
	name, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("invalid resource name")
	}
	name = strings.TrimSpace(name)
	if err := validateResourceName(name, "resource"); err != nil {
		return "", err
	}
	return name, nil
}

func validateResourceName(name, kind string) error {
	if name == "" {
		return fmt.Errorf("%s name is required", kind)
	}
	if len(name) > 64 || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid %s name", kind)
	}
	return nil
}

func writeMutationError(w http.ResponseWriter, err error) {
	message := err.Error()
	status := http.StatusBadRequest
	if strings.Contains(message, "state_file") || strings.Contains(message, "state directory") || strings.Contains(message, "write state") || strings.Contains(message, "replace state") {
		status = http.StatusConflict
	}
	writeError(w, status, message)
}
