package httpapi

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/thekfjie/cmcc-notify/cmcc"
	"github.com/thekfjie/cmcc-notify/server/internal/config"
)

//go:embed web
var embeddedWeb embed.FS

type Server struct {
	cfg               config.Config
	clients           map[string]*cmcc.Client
	version           string
	startedAt         time.Time
	cfgMu             sync.RWMutex
	mu                sync.Mutex
	mutationMu        sync.Mutex
	applicationRateMu sync.Mutex
	applicationRates  map[string]applicationRateWindow
	baseCtx           context.Context
	clientsCancel     context.CancelFunc
}

func New(cfg config.Config, version string) (*Server, error) {
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	return &Server{
		cfg:              cfg,
		clients:          make(map[string]*cmcc.Client),
		applicationRates: make(map[string]applicationRateWindow),
		version:          version,
		startedAt:        time.Now(),
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	s.baseCtx = ctx
	s.mu.Unlock()
	return s.rebuildClients()
}

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clientsCancel != nil {
		s.clientsCancel()
		s.clientsCancel = nil
	}
	for name, client := range s.clients {
		_ = client.Close()
		delete(s.clients, name)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/v1/status", s.status)
	mux.HandleFunc("/v1/send", s.send)
	mux.HandleFunc("/v1/send/media", s.sendMediaUpload)
	mux.HandleFunc("/v1/notify", s.notify)
	mux.HandleFunc("/v1/settings", s.settings)
	mux.HandleFunc("/v1/accounts", s.accounts)
	mux.HandleFunc("/v1/accounts/", s.account)
	mux.HandleFunc("/v1/groups", s.groups)
	mux.HandleFunc("/v1/groups/", s.group)
	mux.HandleFunc("/v1/applications", s.applications)
	mux.HandleFunc("/v1/applications/", s.application)
	web, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		panic(err)
	}
	mux.Handle("/assets/", http.FileServer(http.FS(web)))
	mux.Handle("/cmcc-new-message-activation.svg", http.FileServer(http.FS(web)))
	mux.HandleFunc("/", s.index(web))
	return securityHeaders(mux)
}

type sendRequest struct {
	Account string `json:"account"`
	To      string `json:"to"`
	Group   string `json:"group"`
	Text    string `json:"text"`
	Media   *struct {
		Type         string `json:"type"`
		URL          string `json:"url"`
		Caption      string `json:"caption"`
		FileName     string `json:"file_name"`
		MIMEType     string `json:"mime_type"`
		ThumbnailURL string `json:"thumbnail_url"`
	} `json:"media,omitempty"`
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type statusResponse struct {
	Status        string          `json:"status"`
	Version       string          `json:"version"`
	UptimeSeconds int64           `json:"uptime_seconds"`
	Accounts      []accountStatus `json:"accounts"`
}

type accountStatus struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	APIKey    string `json:"api_key"`
	DefaultTo string `json:"default_to,omitempty"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.mu.Lock()
	clients := make(map[string]*cmcc.Client, len(s.clients))
	for name, client := range s.clients {
		clients[name] = client
	}
	s.mu.Unlock()

	accounts := make([]accountStatus, 0, len(cfg.Accounts))
	for _, account := range cfg.Accounts {
		client := clients[account.Name]
		accounts = append(accounts, accountStatus{
			Name:      account.Name,
			Enabled:   account.Enabled,
			Connected: account.Enabled && client != nil && client.Connected(),
			APIKey:    maskAPIKey(account.APIKey),
			DefaultTo: account.DefaultTo,
		})
	}
	writeJSON(w, http.StatusOK, statusResponse{
		Status:        "ok",
		Version:       s.version,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
		Accounts:      accounts,
	})
}

func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request sendRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	accountName := request.Account
	if accountName == "" && len(cfg.Accounts) == 1 {
		accountName = cfg.Accounts[0].Name
	}
	account, ok := cfg.Account(accountName)
	if !ok || !account.Enabled {
		writeError(w, http.StatusBadRequest, "unknown or disabled account")
		return
	}
	if request.Media != nil {
		if strings.TrimSpace(request.Media.URL) == "" {
			writeError(w, http.StatusBadRequest, "media.url is required")
			return
		}
		if !validRemoteMediaURL(request.Media.URL) {
			writeError(w, http.StatusBadRequest, "media.url must use http or https")
			return
		}
	} else {
		if strings.TrimSpace(request.Text) == "" {
			writeError(w, http.StatusBadRequest, "text is required")
			return
		}
	}
	recipients, groupName, err := resolveRecipients(cfg, account, request.To, request.Group)
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
	if request.Media != nil {
		result, batch, sendErr := sendMediaToRecipients(ctx, client, recipients, groupName, cmcc.MediaMessage{
			MediaType: parseMediaType(request.Media.Type, request.Media.MIMEType),
			Content:   request.Media.Caption, MediaURL: request.Media.URL,
			MediaFileName: request.Media.FileName, MediaMIMEType: request.Media.MIMEType,
			ThumbnailURL: request.Media.ThumbnailURL,
		})
		if sendErr != nil {
			writeError(w, http.StatusBadGateway, sendErr.Error())
			return
		}
		if batch != nil {
			writeJSON(w, batchStatus(*batch), batch)
			return
		}
		writeJSON(w, http.StatusAccepted, result)
		return
	}
	result, batch, sendErr := sendTextToRecipients(ctx, client, recipients, groupName, request.Text)
	if sendErr != nil {
		writeError(w, http.StatusBadGateway, sendErr.Error())
		return
	}
	if batch != nil {
		writeJSON(w, batchStatus(*batch), batch)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

type recipientSendResult struct {
	To string `json:"to"`
	cmcc.SendResult
	Error string `json:"error,omitempty"`
}

type batchSendResponse struct {
	Group         string                `json:"group"`
	Total         int                   `json:"total"`
	AcceptedCount int                   `json:"accepted_count"`
	FailedCount   int                   `json:"failed_count"`
	Results       []recipientSendResult `json:"results"`
}

func resolveRecipients(cfg config.Config, account config.Account, to, groupName string) ([]string, string, error) {
	to = strings.TrimSpace(to)
	groupName = strings.TrimSpace(groupName)
	if to != "" && groupName != "" {
		return nil, "", errors.New("to and group cannot be used together")
	}
	if groupName != "" {
		group, ok := cfg.Group(groupName)
		if !ok {
			return nil, "", errors.New("unknown group")
		}
		return append([]string(nil), group.Recipients...), group.Name, nil
	}
	if to == "" {
		to = strings.TrimSpace(account.DefaultTo)
	}
	if to == "" {
		return nil, "", errors.New("recipient is required")
	}
	return []string{to}, "", nil
}

func sendMediaToRecipients(ctx context.Context, client *cmcc.Client, recipients []string, groupName string, message cmcc.MediaMessage) (cmcc.SendResult, *batchSendResponse, error) {
	if groupName == "" {
		message.To = recipients[0]
		result, err := client.SendMedia(ctx, message)
		return result, nil, err
	}
	response := &batchSendResponse{Group: groupName, Total: len(recipients)}
	for _, recipient := range recipients {
		message.To = recipient
		message.MessageID = ""
		message.Timestamp = 0
		result, sendErr := client.SendMedia(ctx, message)
		item := recipientSendResult{To: recipient, SendResult: result}
		if sendErr != nil {
			item.Error = sendErr.Error()
			response.FailedCount++
		} else {
			response.AcceptedCount++
		}
		response.Results = append(response.Results, item)
	}
	return cmcc.SendResult{}, response, nil
}

func sendTextToRecipients(ctx context.Context, client *cmcc.Client, recipients []string, groupName, content string) (cmcc.SendResult, *batchSendResponse, error) {
	if groupName == "" {
		result, err := client.SendText(ctx, recipients[0], content)
		return result, nil, err
	}
	response := &batchSendResponse{Group: groupName, Total: len(recipients)}
	for _, recipient := range recipients {
		result, sendErr := client.SendText(ctx, recipient, content)
		item := recipientSendResult{To: recipient, SendResult: result}
		if sendErr != nil {
			item.Error = sendErr.Error()
			response.FailedCount++
		} else {
			response.AcceptedCount++
		}
		response.Results = append(response.Results, item)
	}
	return cmcc.SendResult{}, response, nil
}

func batchStatus(response batchSendResponse) int {
	if response.AcceptedCount == 0 {
		return http.StatusBadGateway
	}
	return http.StatusAccepted
}

func (s *Server) index(web fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		content, err := fs.ReadFile(web, "index.html")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "web interface unavailable")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet {
			_, _ = w.Write(content)
		}
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; img-src 'self' data:; object-src 'none'; script-src 'self'; style-src 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		next.ServeHTTP(w, r)
	})
}

func maskAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 7 {
		return "***"
	}
	prefix := value[:3]
	return prefix + "***" + value[len(value)-4:]
}

func (s *Server) client(account config.Account) (*cmcc.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client := s.clients[account.Name]; client != nil {
		return client, nil
	}
	client, err := cmcc.NewClient(account.APIKey, cmcc.Config{ServerURL: account.ServerURL, UploadURL: account.UploadURL})
	if err != nil {
		return nil, err
	}
	s.clients[account.Name] = client
	return client, nil
}

func (s *Server) configSnapshot() config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return cloneConfig(s.cfg)
}

func cloneConfig(cfg config.Config) config.Config {
	copyConfig := cfg
	copyConfig.Accounts = append([]config.Account(nil), cfg.Accounts...)
	copyConfig.Groups = make([]config.Group, len(cfg.Groups))
	for i, group := range cfg.Groups {
		copyConfig.Groups[i] = group
		copyConfig.Groups[i].Recipients = append([]string(nil), group.Recipients...)
	}
	copyConfig.Applications = append([]config.Application(nil), cfg.Applications...)
	return copyConfig
}

func (s *Server) rebuildClients() error {
	cfg := s.configSnapshot()
	s.mu.Lock()
	if s.clientsCancel != nil {
		s.clientsCancel()
	}
	for _, client := range s.clients {
		_ = client.Close()
	}
	s.clients = make(map[string]*cmcc.Client)
	parent := s.baseCtx
	if parent == nil {
		parent = context.Background()
	}
	clientsCtx, cancel := context.WithCancel(parent)
	s.clientsCancel = cancel
	s.mu.Unlock()
	for _, account := range cfg.Accounts {
		if !account.Enabled {
			continue
		}
		client, err := s.client(account)
		if err != nil {
			cancel()
			return err
		}
		go client.ReconnectLoop(clientsCtx)
	}
	return nil
}

func validRemoteMediaURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func authorized(r *http.Request, expected string) bool {
	value := r.Header.Get("Authorization")
	actual := strings.TrimSpace(value)
	want := "Bearer " + expected
	return len(actual) == len(want) && subtle.ConstantTimeCompare([]byte(actual), []byte(want)) == 1
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func parseMediaType(kind, mimeType string) cmcc.MediaType {
	switch strings.ToUpper(kind) {
	case "IMAGE":
		return cmcc.MediaImage
	case "AUDIO":
		return cmcc.MediaAudio
	case "VIDEO":
		return cmcc.MediaVideo
	case "FILE":
		return cmcc.MediaFile
	}
	switch {
	case strings.HasPrefix(strings.ToLower(mimeType), "image/"):
		return cmcc.MediaImage
	case strings.HasPrefix(strings.ToLower(mimeType), "audio/"):
		return cmcc.MediaAudio
	case strings.HasPrefix(strings.ToLower(mimeType), "video/"):
		return cmcc.MediaVideo
	default:
		return cmcc.MediaFile
	}
}
