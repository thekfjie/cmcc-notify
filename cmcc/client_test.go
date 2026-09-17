package cmcc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClientConnectAndSendText(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		var auth map[string]any
		if err := conn.ReadJSON(&auth); err != nil {
			t.Errorf("read auth: %v", err)
			return
		}
		if auth["type"] != "auth" || auth["apiKey"] != "ak_test" || auth["version"] != "2.0" {
			t.Errorf("unexpected auth: %#v", auth)
			return
		}
		if err := conn.WriteJSON(map[string]string{"type": "auth_ok"}); err != nil {
			return
		}
		for {
			var frame map[string]any
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			switch frame["type"] {
			case "send":
				if frame["to"] != "13800138000" || frame["content"] != "hello" {
					t.Errorf("unexpected send frame: %#v", frame)
				}
			case "ping":
				_ = conn.WriteJSON(map[string]string{"type": "pong"})
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, err := NewClient("ak_test", Config{ServerURL: wsURL, HeartbeatEvery: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	result, err := client.SendText(context.Background(), "13800138000", "hello")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !result.Accepted || result.Acknowledged || result.MessageID == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClientSendTextOmitsEmptyTarget(t *testing.T) {
	frames := make(chan map[string]any, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})
		var frame map[string]any
		if conn.ReadJSON(&frame) == nil {
			frames <- frame
		}
	}))
	defer srv.Close()

	client, err := NewClient("ak_test", Config{ServerURL: "ws" + strings.TrimPrefix(srv.URL, "http"), HeartbeatEvery: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendText(context.Background(), "", "self notification"); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-frames:
		if _, exists := frame["to"]; exists || frame["content"] != "self notification" {
			t.Fatalf("frame = %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("missing text frame")
	}
}

func TestClientNormalizesIncomingMedia(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})
		_ = conn.WriteJSON(map[string]any{
			"type": "media_message", "messageId": "m1", "from": "13800138000",
			"content": "photo", "mediaType": "IMAGE", "mediaUrl": "https://example.invalid/a.jpg",
			"timestamp": time.Now().UnixMilli(),
		})
		select {}
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, err := NewClient("app_test", Config{ServerURL: wsURL, HeartbeatEvery: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-client.Events():
		if event.Kind != EventConnected {
			t.Fatalf("first event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing connected event")
	}
	select {
	case event := <-client.Events():
		if event.Kind != EventMessage || event.Message == nil || event.Message.MediaType != MediaImage || event.Message.ID != "m1" {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing message event")
	}
}

func TestValidAPIKey(t *testing.T) {
	for _, tc := range []struct {
		key string
		ok  bool
	}{
		{"ak_x", true}, {"app_x", true}, {"ak_", false}, {"token", false}, {"", false},
	} {
		if got := ValidAPIKey(tc.key); got != tc.ok {
			t.Errorf("ValidAPIKey(%q) = %v, want %v", tc.key, got, tc.ok)
		}
	}
}

func TestMediaMessageJSON(t *testing.T) {
	b, err := json.Marshal(MediaMessage{To: "13800138000", MediaType: MediaImage, Content: "caption", MediaURL: "https://example.invalid/a.jpg"})
	if err != nil || !strings.Contains(string(b), `"mediaType":"IMAGE"`) || !strings.Contains(string(b), `"to":"13800138000"`) {
		t.Fatalf("marshal = %s, err=%v", b, err)
	}
}

func TestClientSendMediaIncludesRecipient(t *testing.T) {
	frames := make(chan map[string]any, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})
		var frame map[string]any
		if conn.ReadJSON(&frame) == nil {
			frames <- frame
		}
	}))
	defer srv.Close()

	client, err := NewClient("ak_test", Config{ServerURL: "ws" + strings.TrimPrefix(srv.URL, "http"), HeartbeatEvery: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendMedia(context.Background(), MediaMessage{
		To: "13800138000", MediaType: MediaImage, MediaURL: "https://example.invalid/a.jpg",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-frames:
		if frame["to"] != "13800138000" || frame["mediaType"] != "IMAGE" {
			t.Fatalf("frame = %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("missing media frame")
	}
}

func TestClientSendMediaOmitsEmptyTarget(t *testing.T) {
	frames := make(chan map[string]any, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})
		var frame map[string]any
		if conn.ReadJSON(&frame) == nil {
			frames <- frame
		}
	}))
	defer srv.Close()

	client, err := NewClient("ak_test", Config{ServerURL: "ws" + strings.TrimPrefix(srv.URL, "http"), HeartbeatEvery: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendMedia(context.Background(), MediaMessage{MediaType: MediaImage, MediaURL: "https://example.invalid/a.jpg"}); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-frames:
		if _, exists := frame["to"]; exists || frame["mediaType"] != "IMAGE" {
			t.Fatalf("frame = %#v", frame)
		}
		if _, exists := frame["content"]; exists {
			t.Fatalf("empty media content must be omitted: %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("missing media frame")
	}
}
