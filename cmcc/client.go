package cmcc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	DefaultServerURL       = "wss://5gvas01.cmicmaap.com/gtw-ai/openclaw/ws/msg"
	DefaultUploadURL       = "https://5gvas01.cmicmaap.com/gtw-ai/openclaw/api"
	DefaultProtocolVersion = "2.0"
	DefaultMaxUploadBytes  = 200 << 20
)

type Config struct {
	ServerURL        string
	UploadURL        string
	Version          string
	HTTPClient       *http.Client
	Dialer           *websocket.Dialer
	AuthTimeout      time.Duration
	WriteTimeout     time.Duration
	HeartbeatEvery   time.Duration
	HeartbeatTimeout time.Duration
	MaxUploadBytes   int64
}

func (c Config) withDefaults() Config {
	if c.ServerURL == "" {
		c.ServerURL = DefaultServerURL
	}
	if c.UploadURL == "" {
		c.UploadURL = DefaultUploadURL
	}
	if c.Version == "" {
		c.Version = DefaultProtocolVersion
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if c.Dialer == nil {
		c.Dialer = &websocket.Dialer{
			TLSClientConfig:  &tls.Config{MinVersion: tls.VersionTLS12},
			HandshakeTimeout: 15 * time.Second,
		}
	}
	if c.AuthTimeout <= 0 {
		c.AuthTimeout = 10 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 10 * time.Second
	}
	if c.HeartbeatEvery <= 0 {
		c.HeartbeatEvery = 15 * time.Second
	}
	if c.HeartbeatTimeout <= 0 {
		c.HeartbeatTimeout = 10 * time.Second
	}
	if c.MaxUploadBytes <= 0 {
		c.MaxUploadBytes = DefaultMaxUploadBytes
	}
	return c
}

// Client is a concurrency-safe CMCC gateway connection. It never logs the
// API key and never treats a socket write as a delivery receipt.
type Client struct {
	apiKey string
	cfg    Config

	connectMu sync.Mutex
	writeMu   sync.Mutex
	mu        sync.RWMutex
	conn      *websocket.Conn
	connected bool
	lastPong  time.Time

	events chan Event
	closed chan struct{}
}

func NewClient(apiKey string, cfg Config) (*Client, error) {
	apiKey = NormalizeAPIKey(apiKey)
	if !ValidAPIKey(apiKey) {
		return nil, ErrInvalidAPIKey
	}
	cfg = cfg.withDefaults()
	return &Client{
		apiKey: apiKey,
		cfg:    cfg,
		events: make(chan Event, 32),
		closed: make(chan struct{}),
	}, nil
}

func (c *Client) Events() <-chan Event { return c.events }

func (c *Client) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Connect performs the gateway handshake. It is safe to call repeatedly.
func (c *Client) Connect(ctx context.Context) error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	if c.Connected() {
		return nil
	}

	header := http.Header{}
	header.Set("X-API-Key", c.apiKey)
	conn, _, err := c.cfg.Dialer.DialContext(ctx, c.cfg.ServerURL, header)
	if err != nil {
		return err
	}

	if err := conn.SetReadDeadline(time.Now().Add(c.cfg.AuthTimeout)); err != nil {
		_ = conn.Close()
		return err
	}
	auth := map[string]any{"type": "auth", "apiKey": c.apiKey, "version": c.cfg.Version}
	if err := conn.WriteJSON(auth); err != nil {
		_ = conn.Close()
		return err
	}
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			_ = conn.Close()
			return err
		}
		var frame struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(payload, &frame); err != nil {
			continue
		}
		switch frame.Type {
		case "auth_ok":
			goto authenticated
		case "auth_failed":
			_ = conn.Close()
			if frame.Message == "" {
				return ErrAuthFailed
			}
			return fmt.Errorf("%w: %s", ErrAuthFailed, frame.Message)
		}
	}

authenticated:
	_ = conn.SetReadDeadline(time.Time{})
	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.lastPong = time.Now()
	c.mu.Unlock()
	c.emit(Event{Kind: EventConnected})
	go c.readLoop(conn)
	go c.heartbeatLoop(conn)
	return nil
}

func (c *Client) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.connected = false
	c.mu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

// ReconnectLoop maintains one connection until ctx is cancelled. The first
// connection is attempted immediately; subsequent failures use capped
// exponential backoff with jitter.
func (c *Client) ReconnectLoop(ctx context.Context) {
	for attempt := 1; ; attempt++ {
		if err := c.Connect(ctx); err == nil {
			attempt = 0
		} else {
			if ctx.Err() != nil {
				return
			}
			delay := backoff(attempt)
			c.emit(Event{Kind: EventReconnecting, Err: err, Attempt: attempt, NextRetryIn: delay})
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !c.Connected() {
			continue
		}
		// Do not consume c.events here: callers may be listening for inbound
		// messages. Polling keeps the event channel available to consumers.
		ticker := time.NewTicker(500 * time.Millisecond)
		for c.Connected() {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
			}
		}
		ticker.Stop()
		attempt = 0
	}
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 3 * time.Second
	for i := 1; i < attempt && d < 60*time.Second; i++ {
		d *= 2
	}
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

func (c *Client) SendText(ctx context.Context, to, content string) (SendResult, error) {
	to = strings.TrimSpace(to)
	messageID := newMessageID()
	frame := map[string]any{"type": "send", "apiKey": c.apiKey, "content": content, "messageId": messageID}
	if to != "" {
		frame["to"] = to
	}
	if err := c.writeJSON(ctx, frame); err != nil {
		return SendResult{}, err
	}
	return SendResult{MessageID: messageID, Accepted: true}, nil
}

func (c *Client) SendMedia(ctx context.Context, message MediaMessage) (SendResult, error) {
	message.To = strings.TrimSpace(message.To)
	if message.MediaType == "" || message.MediaURL == "" {
		return SendResult{}, errors.New("cmcc: mediaType and mediaUrl are required")
	}
	if message.MessageID == "" {
		message.MessageID = newMessageID()
	}
	if message.Timestamp == 0 {
		message.Timestamp = time.Now().UnixMilli()
	}
	frame := map[string]any{
		"type": "send", "apiKey": c.apiKey,
		"mediaType": message.MediaType,
		"mediaUrl":  message.MediaURL, "messageId": message.MessageID,
		"timestamp": message.Timestamp,
	}
	if message.To != "" {
		frame["to"] = message.To
	}
	if message.Content != "" {
		frame["content"] = message.Content
	}
	if message.ThumbnailURL != "" {
		frame["thumbnailUrl"] = message.ThumbnailURL
	}
	if message.MediaFileName != "" {
		frame["mediaFileName"] = message.MediaFileName
	}
	if message.MediaSize > 0 {
		frame["mediaSize"] = message.MediaSize
	}
	if message.MediaMIMEType != "" {
		frame["mediaMimeType"] = message.MediaMIMEType
	}
	if err := c.writeJSON(ctx, frame); err != nil {
		return SendResult{}, err
	}
	return SendResult{MessageID: message.MessageID, Accepted: true}, nil
}

func (c *Client) writeJSON(ctx context.Context, frame any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil || !c.Connected() {
		return ErrNotConnected
	}
	deadline := time.Now().Add(c.cfg.WriteTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return conn.WriteJSON(frame)
}

func (c *Client) readLoop(conn *websocket.Conn) {
	defer c.clearConnection(conn)
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			c.emit(Event{Kind: EventError, Err: err})
			return
		}
		var frame struct {
			Type          string    `json:"type"`
			ID            string    `json:"id"`
			MessageID     string    `json:"messageId"`
			From          string    `json:"from"`
			Phone         string    `json:"phone"`
			Content       string    `json:"content"`
			Timestamp     int64     `json:"timestamp"`
			MediaType     MediaType `json:"mediaType"`
			MediaURL      string    `json:"mediaUrl"`
			MediaFileName string    `json:"mediaFileName"`
			ThumbnailURL  string    `json:"thumbnailUrl"`
			MediaSize     int64     `json:"mediaSize"`
			MediaMIMEType string    `json:"mediaMimeType"`
			Message       string    `json:"message"`
		}
		if err := json.Unmarshal(payload, &frame); err != nil {
			c.emit(Event{Kind: EventError, Err: err})
			continue
		}
		if frame.Type == "pong" {
			c.mu.Lock()
			c.lastPong = time.Now()
			c.mu.Unlock()
			continue
		}
		if frame.Type == "error" {
			c.emit(Event{Kind: EventError, Err: errors.New(frame.Message)})
			continue
		}
		if frame.Type != "message" && frame.Type != "text_message" && frame.Type != "media_message" {
			continue
		}
		stamp := time.UnixMilli(frame.Timestamp)
		if frame.Timestamp == 0 {
			stamp = time.Now()
		}
		from := frame.From
		if from == "" {
			from = frame.Phone
		}
		msg := &IncomingMessage{
			ID: frame.MessageID, From: from, Content: frame.Content, Timestamp: stamp,
			MediaType: frame.MediaType, MediaURL: frame.MediaURL, MediaFileName: frame.MediaFileName,
			ThumbnailURL: frame.ThumbnailURL, MediaSize: frame.MediaSize, MediaMIMEType: frame.MediaMIMEType,
		}
		if msg.ID == "" {
			msg.ID = frame.ID
		}
		c.emit(Event{Kind: EventMessage, Message: msg})
	}
}

func (c *Client) heartbeatLoop(conn *websocket.Conn) {
	ticker := time.NewTicker(c.cfg.HeartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.mu.RLock()
			lastPong := c.lastPong
			active := c.conn == conn && c.connected
			c.mu.RUnlock()
			if !active {
				return
			}
			if time.Since(lastPong) > c.cfg.HeartbeatEvery+c.cfg.HeartbeatTimeout {
				_ = conn.Close()
				return
			}
			_ = c.writeJSON(context.Background(), map[string]string{"type": "ping"})
		case <-c.closed:
			return
		}
	}
}

func (c *Client) clearConnection(conn *websocket.Conn) {
	c.mu.Lock()
	if c.conn != conn {
		c.mu.Unlock()
		return
	}
	c.conn = nil
	c.connected = false
	c.mu.Unlock()
	_ = conn.Close()
	c.emit(Event{Kind: EventDisconnected})
}

func (c *Client) emit(event Event) {
	select {
	case c.events <- event:
	default:
		// State events must not block the reader goroutine.
	}
}

func newMessageID() string { return fmt.Sprintf("msg_%d", time.Now().UnixNano()) }

func (c *Client) Upload(ctx context.Context, fileName, localPath string, mediaType MediaType) (string, int64, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if info.Size() > c.cfg.MaxUploadBytes {
		return "", info.Size(), fmt.Errorf("cmcc: file size %d exceeds limit %d", info.Size(), c.cfg.MaxUploadBytes)
	}
	endpoint, err := uploadEndpoint(c.cfg.UploadURL)
	if err != nil {
		return "", info.Size(), err
	}
	pr, pw := io.Pipe()
	writerErr := make(chan error, 1)
	boundary := "cmcc-notify-boundary"
	go func() {
		defer pw.Close()
		write := func(s string) error { _, e := io.WriteString(pw, s); return e }
		if err := write("--" + boundary + "\r\n" +
			"Content-Disposition: form-data; name=\"file\"; filename=\"" + sanitizeFileName(fileName) + "\"\r\n" +
			"Content-Type: application/octet-stream\r\n\r\n"); err != nil {
			writerErr <- err
			return
		}
		if _, err := io.Copy(pw, file); err != nil {
			writerErr <- err
			return
		}
		if err := write("\r\n--" + boundary + "\r\n" +
			"Content-Disposition: form-data; name=\"apiKey\"\r\n\r\n" + c.apiKey + "\r\n" +
			"--" + boundary + "--\r\n"); err != nil {
			writerErr <- err
			return
		}
		writerErr <- nil
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		return "", info.Size(), err
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		_ = pr.CloseWithError(err)
		return "", info.Size(), err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return "", info.Size(), readErr
	}
	if err := <-writerErr; err != nil {
		return "", info.Size(), err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", info.Size(), &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", info.Size(), fmt.Errorf("cmcc: invalid upload response: %w", err)
	}
	if result.Code != 10200 || result.Data == "" {
		if result.Message == "" {
			result.Message = fmt.Sprintf("gateway code %d", result.Code)
		}
		return "", info.Size(), errors.New("cmcc: upload failed: " + result.Message)
	}
	_ = mediaType // media type is inferred by the gateway from the uploaded file.
	return result.Data, info.Size(), nil
}

func uploadEndpoint(base string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("cmcc: invalid upload URL")
	}
	u.Path = path.Join(u.Path, "upload")
	return u.String(), nil
}

func sanitizeFileName(name string) string {
	name = path.Base(name)
	name = strings.ReplaceAll(name, "\"", "_")
	if name == "." || name == "" {
		return "upload.bin"
	}
	return name
}
