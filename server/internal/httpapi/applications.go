package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thekfjie/cmcc-notify/cmcc"
	"github.com/thekfjie/cmcc-notify/server/internal/config"
)

var (
	errApplicationAlreadyExists    = errors.New("application already exists")
	errApplicationNotFound         = errors.New("application not found")
	errMessageAndTextConflict      = errors.New("message and text cannot be used together")
	errNotificationContentRequired = errors.New("title, message or text is required")
	errInvalidApplicationName      = errors.New("invalid application name")
)

type applicationSetting struct {
	Name                   string `json:"name"`
	Enabled                bool   `json:"enabled"`
	Account                string `json:"account"`
	To                     string `json:"to,omitempty"`
	Group                  string `json:"group,omitempty"`
	AllowRecipientOverride bool   `json:"allow_recipient_override"`
	RateLimitPerMinute     int    `json:"rate_limit_per_minute"`
	TokenHint              string `json:"token_hint"`
}

type applicationMutation struct {
	Name                   string `json:"name"`
	Enabled                bool   `json:"enabled"`
	Account                string `json:"account"`
	To                     string `json:"to"`
	Group                  string `json:"group"`
	AllowRecipientOverride bool   `json:"allow_recipient_override"`
	RateLimitPerMinute     int    `json:"rate_limit_per_minute"`
}

type notifyRequest struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Text    string `json:"text"`
	To      string `json:"to"`
	Group   string `json:"group"`
}

type notifyResponse struct {
	Application string `json:"application"`
	cmcc.SendResult
}

type notifyBatchResponse struct {
	Application string `json:"application"`
	batchSendResponse
}

type applicationRateWindow struct {
	StartedAt time.Time
	Count     int
}

func (s *Server) applications(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request applicationMutation
	if err := decodeManagementJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if err := validateResourceName(request.Name, "application"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	token, err := newApplicationToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot generate application token")
		return
	}
	if err := s.updateConfig(false, func(next *config.Config) error {
		if _, exists := next.Application(request.Name); exists {
			return errApplicationAlreadyExists
		}
		next.Applications = append(next.Applications, applicationFromMutation(request, token))
		return nil
	}); err != nil {
		writeMutationError(w, err)
		return
	}
	s.writeSettingsWithApplicationToken(w, http.StatusCreated, s.configSnapshot(), token)
}

func (s *Server) application(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	name, rotate, err := applicationResource(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rotate {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		token, tokenErr := newApplicationToken()
		if tokenErr != nil {
			writeError(w, http.StatusInternalServerError, "cannot generate application token")
			return
		}
		if err := s.updateConfig(false, func(next *config.Config) error {
			for i := range next.Applications {
				if next.Applications[i].Name != name {
					continue
				}
				next.Applications[i].TokenHash = config.HashApplicationToken(token)
				next.Applications[i].TokenHint = config.ApplicationTokenHint(token)
				next.Applications[i].TokenEnv = ""
				next.Applications[i].TokenFile = ""
				return nil
			}
			return errApplicationNotFound
		}); err != nil {
			writeMutationError(w, err)
			return
		}
		if previous, ok := cfg.Application(name); ok {
			s.forgetApplicationRate(previous.TokenHash)
		}
		s.writeSettingsWithApplicationToken(w, http.StatusOK, s.configSnapshot(), token)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var request applicationMutation
		if err := decodeManagementJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if request.Name != "" && strings.TrimSpace(request.Name) != name {
			writeError(w, http.StatusBadRequest, "application name cannot be changed")
			return
		}
		if err := s.updateConfig(false, func(next *config.Config) error {
			for i := range next.Applications {
				if next.Applications[i].Name != name {
					continue
				}
				application := &next.Applications[i]
				application.Enabled = request.Enabled
				application.Account = strings.TrimSpace(request.Account)
				application.To = strings.TrimSpace(request.To)
				application.Group = strings.TrimSpace(request.Group)
				application.AllowRecipientOverride = request.AllowRecipientOverride
				application.RateLimitPerMinute = request.RateLimitPerMinute
				return nil
			}
			return errApplicationNotFound
		}); err != nil {
			writeMutationError(w, err)
			return
		}
		s.writeSettings(w, http.StatusOK, s.configSnapshot())
	case http.MethodDelete:
		if err := s.updateConfig(false, func(next *config.Config) error {
			for i := range next.Applications {
				if next.Applications[i].Name == name {
					next.Applications = append(next.Applications[:i], next.Applications[i+1:]...)
					return nil
				}
			}
			return errApplicationNotFound
		}); err != nil {
			writeMutationError(w, err)
			return
		}
		if previous, ok := cfg.Application(name); ok {
			s.forgetApplicationRate(previous.TokenHash)
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) notify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := s.configSnapshot()
	application, ok := authenticateApplication(r, cfg)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !application.Enabled {
		writeError(w, http.StatusForbidden, "application is disabled")
		return
	}
	if allowed, retryAfter := s.allowApplicationRequest(application); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeError(w, http.StatusTooManyRequests, "application rate limit exceeded")
		return
	}
	var request notifyRequest
	if err := decodeManagementJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	text, err := notificationText(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	to := strings.TrimSpace(request.To)
	groupName := strings.TrimSpace(request.Group)
	if (to != "" || groupName != "") && !application.AllowRecipientOverride {
		writeError(w, http.StatusForbidden, "recipient override is not allowed")
		return
	}
	if to == "" && groupName == "" {
		to = application.To
		groupName = application.Group
	}
	account, ok := cfg.Account(application.Account)
	if !ok || !account.Enabled {
		writeError(w, http.StatusServiceUnavailable, "application account is unavailable")
		return
	}
	recipients, resolvedGroup, err := resolveRecipients(cfg, account, to, groupName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	client, err := s.client(account)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if !client.Connected() {
		if err := client.Connect(ctx); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	result, batch, err := sendTextToRecipients(ctx, client, recipients, resolvedGroup, text)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if batch != nil {
		writeJSON(w, batchStatus(*batch), notifyBatchResponse{Application: application.Name, batchSendResponse: *batch})
		return
	}
	if !result.Accepted {
		writeError(w, http.StatusBadGateway, "message was not accepted")
		return
	}
	writeJSON(w, http.StatusAccepted, notifyResponse{Application: application.Name, SendResult: result})
}

func applicationFromMutation(request applicationMutation, token string) config.Application {
	return config.Application{
		Name: strings.TrimSpace(request.Name), Enabled: request.Enabled,
		Account: strings.TrimSpace(request.Account), To: strings.TrimSpace(request.To), Group: strings.TrimSpace(request.Group),
		AllowRecipientOverride: request.AllowRecipientOverride, RateLimitPerMinute: request.RateLimitPerMinute,
		TokenHash: config.HashApplicationToken(token), TokenHint: config.ApplicationTokenHint(token),
	}
}

func newApplicationToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "cn_app_" + base64.RawURLEncoding.EncodeToString(random), nil
}

func authenticateApplication(r *http.Request, cfg config.Config) (config.Application, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !config.ValidApplicationToken(parts[1]) {
		return config.Application{}, false
	}
	hash := config.HashApplicationToken(parts[1])
	for _, application := range cfg.Applications {
		if len(hash) == len(application.TokenHash) && subtle.ConstantTimeCompare([]byte(hash), []byte(application.TokenHash)) == 1 {
			return application, true
		}
	}
	return config.Application{}, false
}

func (s *Server) allowApplicationRequest(application config.Application) (bool, int) {
	now := time.Now()
	s.applicationRateMu.Lock()
	defer s.applicationRateMu.Unlock()
	window := s.applicationRates[application.TokenHash]
	if window.StartedAt.IsZero() || now.Sub(window.StartedAt) >= time.Minute {
		window = applicationRateWindow{StartedAt: now}
	}
	if window.Count >= application.RateLimitPerMinute {
		retryAfter := int(time.Until(window.StartedAt.Add(time.Minute)).Seconds()) + 1
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}
	window.Count++
	s.applicationRates[application.TokenHash] = window
	return true, 0
}

func (s *Server) forgetApplicationRate(tokenHash string) {
	s.applicationRateMu.Lock()
	delete(s.applicationRates, tokenHash)
	s.applicationRateMu.Unlock()
}

func notificationText(request notifyRequest) (string, error) {
	title := strings.TrimSpace(request.Title)
	message := strings.TrimSpace(request.Message)
	text := strings.TrimSpace(request.Text)
	if message != "" && text != "" {
		return "", errMessageAndTextConflict
	}
	if message == "" {
		message = text
	}
	if title == "" && message == "" {
		return "", errNotificationContentRequired
	}
	if title == "" {
		return message, nil
	}
	if message == "" {
		return title, nil
	}
	return title + "\n\n" + message, nil
}

func applicationResource(r *http.Request) (string, bool, error) {
	raw := strings.TrimPrefix(r.URL.EscapedPath(), "/v1/applications/")
	rotate := strings.HasSuffix(raw, "/rotate-token")
	if rotate {
		raw = strings.TrimSuffix(raw, "/rotate-token")
	}
	name, err := url.PathUnescape(raw)
	if err != nil {
		return "", false, errInvalidApplicationName
	}
	name = strings.TrimSpace(name)
	if err := validateResourceName(name, "application"); err != nil {
		return "", false, err
	}
	return name, rotate, nil
}
