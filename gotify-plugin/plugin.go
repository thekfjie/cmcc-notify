package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	gotifyplugin "github.com/gotify/plugin-api"
	"github.com/thekfjie/cmcc-notify/cmcc"
)

type accountClient struct {
	client *cmcc.Client
}

type pluginStats struct {
	sent      atomic.Uint64
	failed    atomic.Uint64
	lastError atomic.Value
	lastOK    atomic.Value
}

var version = "dev"

type GotifyPlugin struct {
	mu     sync.RWMutex
	config *Config
	ctx    context.Context
	cancel context.CancelFunc

	clientsMu sync.Mutex
	clients   map[string]*accountClient
	stream    *websocket.Conn
	state     stateStore
	stats     pluginStats
	basePath  string
}

func GetGotifyPluginInfo() gotifyplugin.Info {
	return gotifyplugin.Info{
		ModulePath:  "github.com/thekfjie/cmcc-notify/gotify-plugin",
		Name:        "CMCC New Message",
		Version:     version,
		Author:      "cmcc-notify contributors",
		Website:     "https://github.com/thekfjie/cmcc-notify/tree/main/gotify-plugin",
		Description: "Forward Gotify messages to China Mobile New Message",
		License:     "MIT",
	}
}

func NewGotifyPluginInstance(ctx gotifyplugin.UserContext) gotifyplugin.Plugin {
	return &GotifyPlugin{config: defaultConfig(), clients: make(map[string]*accountClient)}
}

func (p *GotifyPlugin) DefaultConfig() interface{} { return defaultConfig() }

func (p *GotifyPlugin) ValidateAndSetConfig(value interface{}) error {
	c, ok := value.(*Config)
	if !ok {
		return errors.New("invalid CMCC plugin config type")
	}
	if err := validateConfig(c); err != nil {
		return err
	}
	p.mu.Lock()
	p.config = cloneConfig(c)
	running := p.cancel != nil
	p.mu.Unlock()
	if running {
		// Gotify calls this method for live config updates. Reconnect using the
		// new snapshot without exposing a data race to the stream goroutine.
		go p.restart()
	}
	return nil
}

func (p *GotifyPlugin) SetStorageHandler(handler gotifyplugin.StorageHandler) {
	p.state.setHandler(handler)
}

func (p *GotifyPlugin) Enable() error {
	cfg := p.configSnapshot()
	if !cfg.Enabled {
		return nil
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}
	p.mu.Lock()
	if p.cancel != nil {
		p.mu.Unlock()
		return nil
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	ctx := p.ctx
	p.mu.Unlock()
	for _, account := range cfg.Accounts {
		if !account.Enabled {
			continue
		}
		clientCfg := cmcc.Config{UploadURL: account.UploadURL}
		client, err := cmcc.NewClient(account.APIKey, clientCfg)
		if err != nil {
			return err
		}
		p.clientsMu.Lock()
		p.clients[account.Name] = &accountClient{client: client}
		p.clientsMu.Unlock()
		go client.ReconnectLoop(ctx)
	}
	go p.streamLoop(ctx)
	return nil
}

func (p *GotifyPlugin) Disable() error {
	p.mu.Lock()
	cancel := p.cancel
	p.cancel = nil
	p.ctx = nil
	stream := p.stream
	p.stream = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if stream != nil {
		_ = stream.Close()
	}
	p.clientsMu.Lock()
	for name, account := range p.clients {
		_ = account.client.Close()
		delete(p.clients, name)
	}
	p.clientsMu.Unlock()
	return nil
}

func (p *GotifyPlugin) restart() {
	_ = p.Disable()
	_ = p.Enable()
}

func (p *GotifyPlugin) streamLoop(ctx context.Context) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		cfg := p.configSnapshot()
		conn, err := p.connectStream(ctx, cfg)
		if err != nil {
			attempt++
			p.recordFailure(err)
			timer := time.NewTimer(backoff(attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		attempt = 0
		if ctx.Err() != nil {
			_ = conn.Close()
			return
		}
		p.mu.Lock()
		p.stream = conn
		p.mu.Unlock()
		for {
			_, payload, readErr := conn.ReadMessage()
			if readErr != nil {
				_ = conn.Close()
				break
			}
			var message gotifyMessage
			if json.Unmarshal(payload, &message) == nil {
				p.routeMessage(ctx, message)
			}
		}
		p.mu.Lock()
		if p.stream == conn {
			p.stream = nil
		}
		p.mu.Unlock()
	}
}

func (p *GotifyPlugin) connectStream(ctx context.Context, cfg *Config) (*websocket.Conn, error) {
	base := strings.TrimRight(cfg.GotifyURL, "/")
	streamURL := base + "/stream"
	if strings.HasPrefix(streamURL, "http://") {
		streamURL = "ws://" + strings.TrimPrefix(streamURL, "http://")
	} else if strings.HasPrefix(streamURL, "https://") {
		streamURL = "wss://" + strings.TrimPrefix(streamURL, "https://")
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+cfg.ClientToken)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, streamURL, header)
	return conn, err
}

func (p *GotifyPlugin) RegisterWebhook(basePath string, mux *gin.RouterGroup) {
	p.mu.Lock()
	p.basePath = basePath
	p.mu.Unlock()
	mux.POST("/send", p.handleSend)
	mux.GET("/status", p.handleStatus)
}

type sendRequest struct {
	Account string `json:"account"`
	To      string `json:"to"`
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

func (p *GotifyPlugin) handleSend(c *gin.Context) {
	var request sendRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	cfg := p.configSnapshot()
	if request.Account == "" && len(cfg.Accounts) == 1 {
		request.Account = cfg.Accounts[0].Name
	}
	accountCfg, ok := findAccount(cfg, request.Account)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown account"})
		return
	}
	if strings.TrimSpace(request.To) == "" {
		request.To = accountCfg.DefaultTo
	}
	if strings.TrimSpace(request.To) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recipient is required"})
		return
	}
	account := p.clientFor(accountCfg)
	if account == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account is unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if !account.Connected() {
		if err := account.Connect(ctx); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	}
	if request.Media != nil {
		result, err := account.SendMedia(ctx, cmcc.MediaMessage{
			To:        request.To,
			MediaType: parseMediaType(request.Media.Type, request.Media.MIMEType), Content: request.Media.Caption,
			MediaURL: request.Media.URL, MediaFileName: request.Media.FileName, MediaMIMEType: request.Media.MIMEType,
			ThumbnailURL: request.Media.ThumbnailURL,
		})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, result)
		return
	}
	result, err := account.SendText(ctx, request.To, request.Text)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (p *GotifyPlugin) handleStatus(c *gin.Context) {
	cfg := p.configSnapshot()
	type accountStatus struct {
		Name      string `json:"name"`
		Enabled   bool   `json:"enabled"`
		Connected bool   `json:"connected"`
		APIKey    string `json:"api_key"`
	}
	result := make([]accountStatus, 0, len(cfg.Accounts))
	for _, account := range cfg.Accounts {
		client := p.clientFor(account)
		result = append(result, accountStatus{Name: account.Name, Enabled: account.Enabled, Connected: client != nil && client.Connected(), APIKey: maskKey(account.APIKey)})
	}
	c.JSON(http.StatusOK, gin.H{"accounts": result, "sent": p.stats.sent.Load(), "failed": p.stats.failed.Load()})
}

func (p *GotifyPlugin) GetDisplay(_ *url.URL) string {
	cfg := p.configSnapshot()
	p.mu.RLock()
	streamConnected := p.stream != nil
	basePath := strings.TrimRight(p.basePath, "/")
	p.mu.RUnlock()

	enabledAccounts := 0
	connectedAccounts := 0
	for _, account := range cfg.Accounts {
		client := p.clientFor(account)
		if account.Enabled {
			enabledAccounts++
			if client != nil && client.Connected() {
				connectedAccounts++
			}
		}
	}

	lines := []string{
		"## CMCC Notify",
		"",
		"| 项目 | 状态 |",
		"| --- | --- |",
		fmt.Sprintf("| 插件 | %s |", enabledLabel(cfg.Enabled)),
		fmt.Sprintf("| Gotify 消息流 | %s |", connectedLabel(streamConnected)),
		fmt.Sprintf("| CMCC 通道 | %d / %d 已连接 |", connectedAccounts, enabledAccounts),
		fmt.Sprintf("| 已成功转发 | %d |", p.stats.sent.Load()),
		fmt.Sprintf("| 转发失败 | %d |", p.stats.failed.Load()),
	}
	if lastOK := atomicString(&p.stats.lastOK); lastOK != "" {
		lines = append(lines, fmt.Sprintf("| 最近成功 | `%s` |", markdownCell(lastOK)))
	}
	if lastError := atomicString(&p.stats.lastError); lastError != "" {
		lines = append(lines, fmt.Sprintf("| 最近错误 | `%s` |", markdownCell(lastError)))
	}

	lines = append(lines, "", "### CMCC 通道状态", "")
	if len(cfg.Accounts) == 0 {
		lines = append(lines, "尚未配置 CMCC 通道。")
	} else {
		lines = append(lines, "| 通道 | 配置 | 连接 | Channel API Key | 默认 to 目标 |", "| --- | --- | --- | --- | --- |")
		for _, account := range cfg.Accounts {
			client := p.clientFor(account)
			lines = append(lines, fmt.Sprintf(
				"| %s | %s | %s | `%s` | `%s` |",
				markdownCell(account.Name),
				enabledLabel(account.Enabled),
				connectedLabel(account.Enabled && client != nil && client.Connected()),
				markdownCell(maskKey(account.APIKey)),
				markdownCell(maskRecipient(account.DefaultTo)),
			))
		}
	}

	lines = append(lines, "", "### 路由", "")
	if len(cfg.Routes) == 0 {
		lines = append(lines, "未配置路由时，消息会逐个转发到所有已启用通道；这不是 CMCC 原生群发。")
	} else {
		lines = append(lines, "| Gotify Applications | CMCC 通道 | 最低优先级 |", "| --- | --- | --- |")
		for _, route := range cfg.Routes {
			applications := uintList(route.Applications)
			accounts := strings.Join(route.Accounts, ", ")
			if accounts == "" {
				accounts = "—"
			}
			lines = append(lines, fmt.Sprintf("| %s | %s | %d |", markdownCell(applications), markdownCell(accounts), route.MinimumPriority))
		}
	}

	lines = append(lines,
		"",
		"### 使用说明",
		"",
		"- 配置内容由 Gotify 的统一 **Configurer** 面板保存。",
		"- YAML/API 为兼容性仍使用 `accounts` 和 `account` 字段；界面中的“通道”就是一份 Channel API Key 配置。",
		"- 文字转发需要通道设置 `default_to`；直接 API 请求也可以提供 `to`。`to` 是实际路由目标，不是备注。",
		"- 一条 Gotify 消息命中多个通道时，插件会对各通道分别发送；这属于插件侧扇出，不是 CMCC 原生广播或群聊。",
		"- HTTP `202 Accepted` 仅表示消息已写入 CMCC 网关，不表示送达或已读。",
	)
	if basePath != "" {
		lines = append(lines,
			"- 插件直连接口：",
			"",
			"```text",
			"POST "+basePath+"/send",
			"GET  "+basePath+"/status",
			"```",
		)
	}
	return strings.Join(lines, "\n")
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "已启用"
	}
	return "已停用"
}

func connectedLabel(connected bool) string {
	if connected {
		return "已连接"
	}
	return "未连接"
}

func atomicString(value *atomic.Value) string {
	loaded := value.Load()
	text, _ := loaded.(string)
	return text
}

func markdownCell(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func uintList(values []uint) string {
	if len(values) == 0 {
		return "全部"
	}
	items := make([]string, len(values))
	for i, value := range values {
		items[i] = fmt.Sprint(value)
	}
	return strings.Join(items, ", ")
}

func (p *GotifyPlugin) configSnapshot() *Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return cloneConfig(p.config)
}

func cloneConfig(c *Config) *Config {
	if c == nil {
		return defaultConfig()
	}
	out := *c
	out.Accounts = append([]AccountConfig(nil), c.Accounts...)
	out.Routes = make([]RouteConfig, len(c.Routes))
	for i, route := range c.Routes {
		out.Routes[i] = route
		out.Routes[i].Applications = append([]uint(nil), route.Applications...)
		out.Routes[i].Accounts = append([]string(nil), route.Accounts...)
	}
	return &out
}

func (p *GotifyPlugin) clientFor(account AccountConfig) *cmcc.Client {
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()
	if value := p.clients[account.Name]; value != nil {
		return value.client
	}
	return nil
}

func (p *GotifyPlugin) recordSuccess() {
	p.stats.sent.Add(1)
	p.stats.lastOK.Store(time.Now().UTC().Format(time.RFC3339))
}

func (p *GotifyPlugin) recordFailure(err error) {
	p.stats.failed.Add(1)
	if err != nil {
		p.stats.lastError.Store(err.Error())
	}
}

func maskKey(key string) string {
	if len(key) < 8 {
		return "***"
	}
	return key[:3] + "***" + key[len(key)-3:]
}

func maskRecipient(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 7 {
		return "***"
	}
	return value[:3] + "****" + value[len(value)-4:]
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 3 * time.Second
	for i := 1; i < attempt && d < time.Minute; i++ {
		d *= 2
	}
	if d > time.Minute {
		d = time.Minute
	}
	return d
}

func main() { panic("cmcc-notify Gotify plugin must be built with -buildmode=plugin") }

var (
	_ gotifyplugin.Plugin     = (*GotifyPlugin)(nil)
	_ gotifyplugin.Configurer = (*GotifyPlugin)(nil)
	_ gotifyplugin.Storager   = (*GotifyPlugin)(nil)
	_ gotifyplugin.Displayer  = (*GotifyPlugin)(nil)
	_ gotifyplugin.Webhooker  = (*GotifyPlugin)(nil)
)
