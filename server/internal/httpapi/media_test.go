package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/thekfjie/cmcc-notify/server/internal/config"
)

func TestMediaUploadUsesCMCCUploadBeforeSending(t *testing.T) {
	frames := make(chan map[string]any, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws":
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
			if auth["apiKey"] != "ak_test" {
				t.Errorf("auth = %#v", auth)
			}
			if err := conn.WriteJSON(map[string]string{"type": "auth_ok"}); err != nil {
				return
			}
			var frame map[string]any
			if err := conn.ReadJSON(&frame); err == nil {
				frames <- frame
			}
		case "/api/upload":
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Errorf("parse gateway upload: %v", err)
				return
			}
			if r.FormValue("apiKey") != "ak_test" {
				t.Errorf("upload apiKey = %q", r.FormValue("apiKey"))
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Errorf("gateway file: %v", err)
				return
			}
			defer file.Close()
			body, _ := io.ReadAll(file)
			if len(body) == 0 {
				t.Error("gateway received empty file")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 10200, "message": "成功", "data": "https://cdn.example.invalid/photo.jpg",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	api, err := New(config.Config{
		AuthToken: "secret",
		Accounts: []config.Account{{
			Name: "primary", APIKey: "ak_test", Enabled: true,
			ServerURL: "ws" + strings.TrimPrefix(gateway.URL, "http") + "/ws",
			UploadURL: gateway.URL + "/api",
		}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("account", "primary")
	_ = writer.WriteField("to", "13800138000")
	_ = writer.WriteField("type", "AUTO")
	_ = writer.WriteField("caption", "photo")
	part, err := writer.CreateFormFile("file", "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/send/media", &body)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result mediaUploadResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.MediaType != "IMAGE" || result.MediaURL == "" {
		t.Fatalf("result = %#v", result)
	}
	select {
	case frame := <-frames:
		if frame["to"] != "13800138000" || frame["mediaType"] != "IMAGE" || frame["mediaUrl"] != "https://cdn.example.invalid/photo.jpg" || frame["mediaFileName"] != "photo.jpg" {
			t.Fatalf("send frame = %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("missing CMCC media send frame")
	}
}

func TestMediaUploadGroupUploadsOnceAndFansOut(t *testing.T) {
	frames := make(chan map[string]any, 2)
	uploads := make(chan struct{}, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws":
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
			for i := 0; i < 2; i++ {
				var frame map[string]any
				if conn.ReadJSON(&frame) != nil {
					return
				}
				frames <- frame
			}
		case "/api/upload":
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Errorf("parse gateway upload: %v", err)
				return
			}
			if r.FormValue("apiKey") != "ak_test" {
				t.Errorf("upload apiKey = %q", r.FormValue("apiKey"))
			}
			uploads <- struct{}{}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 10200, "message": "成功", "data": "https://cdn.example.invalid/archive.zip",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	api, err := New(config.Config{
		AuthToken: "secret",
		Accounts: []config.Account{{
			Name: "primary", APIKey: "ak_test", Enabled: true,
			ServerURL: "ws" + strings.TrimPrefix(gateway.URL, "http") + "/ws",
			UploadURL: gateway.URL + "/api",
		}},
		Groups: []config.Group{{Name: "family", Recipients: []string{"13800138000", "13900139000"}}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("account", "primary")
	_ = writer.WriteField("group", "family")
	_ = writer.WriteField("type", "FILE")
	part, err := writer.CreateFormFile("file", "archive.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte{'P', 'K', 0x03, 0x04})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/send/media", &body)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result mediaUploadBatchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Mode != "local_fanout" || result.NativeBroadcast || result.Group != "family" || result.Total != 2 || result.AcceptedCount != 2 || result.FailedCount != 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.MediaType != "FILE" || result.FileName != "archive.zip" || result.MediaURL != "https://cdn.example.invalid/archive.zip" {
		t.Fatalf("media result = %#v", result)
	}
	select {
	case <-uploads:
	default:
		t.Fatal("CMCC upload was not called")
	}
	recipients := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case frame := <-frames:
			recipients[frame["to"].(string)] = true
			if frame["mediaType"] != "FILE" || frame["mediaUrl"] != "https://cdn.example.invalid/archive.zip" || frame["mediaFileName"] != "archive.zip" {
				t.Fatalf("frame = %#v", frame)
			}
		case <-time.After(time.Second):
			t.Fatal("missing uploaded group media frame")
		}
	}
	if !recipients["13800138000"] || !recipients["13900139000"] {
		t.Fatalf("recipients = %#v", recipients)
	}
}

func TestRemoteMediaGroupFansOutToEveryRecipient(t *testing.T) {
	frames := make(chan map[string]any, 2)
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
		for i := 0; i < 2; i++ {
			var frame map[string]any
			if conn.ReadJSON(&frame) != nil {
				return
			}
			frames <- frame
		}
	}))
	defer gateway.Close()

	api, err := New(config.Config{
		AuthToken: "secret",
		Accounts: []config.Account{{
			Name: "primary", APIKey: "ak_test", Enabled: true,
			ServerURL: "ws" + strings.TrimPrefix(gateway.URL, "http"),
		}},
		Groups: []config.Group{{Name: "family", Recipients: []string{"13800138000", "13900139000"}}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	request := httptest.NewRequest(http.MethodPost, "/v1/send", strings.NewReader(`{
		"account":"primary","group":"family",
		"media":{"type":"IMAGE","url":"https://cdn.example.invalid/photo.jpg","caption":"photo"}
	}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result batchSendResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Mode != "local_fanout" || result.NativeBroadcast || result.Total != 2 || result.AcceptedCount != 2 {
		t.Fatalf("result = %#v", result)
	}
	recipients := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case frame := <-frames:
			recipients[frame["to"].(string)] = true
			if frame["mediaType"] != "IMAGE" || frame["mediaUrl"] != "https://cdn.example.invalid/photo.jpg" {
				t.Fatalf("frame = %#v", frame)
			}
		case <-time.After(time.Second):
			t.Fatal("missing group media frame")
		}
	}
	if !recipients["13800138000"] || !recipients["13900139000"] {
		t.Fatalf("recipients = %#v", recipients)
	}
}

func TestGroupSendFansOutToEveryRecipient(t *testing.T) {
	frames := make(chan map[string]any, 2)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		var auth map[string]any
		if err := conn.ReadJSON(&auth); err != nil {
			return
		}
		if err := conn.WriteJSON(map[string]string{"type": "auth_ok"}); err != nil {
			return
		}
		for i := 0; i < 2; i++ {
			var frame map[string]any
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			frames <- frame
		}
	}))
	defer gateway.Close()

	api, err := New(config.Config{
		AuthToken: "secret",
		Accounts: []config.Account{{
			Name: "primary", APIKey: "ak_test", Enabled: true,
			ServerURL: "ws" + strings.TrimPrefix(gateway.URL, "http"),
		}},
		Groups: []config.Group{{Name: "family", Recipients: []string{"13800138000", "13900139000"}}},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	request := httptest.NewRequest(http.MethodPost, "/v1/send", strings.NewReader(`{"account":"primary","group":"family","text":"hello"}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result batchSendResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Mode != "local_fanout" || result.NativeBroadcast || result.Total != 2 || result.AcceptedCount != 2 || result.FailedCount != 0 {
		t.Fatalf("result = %#v", result)
	}
	recipients := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case frame := <-frames:
			recipients[frame["to"].(string)] = true
		case <-time.After(time.Second):
			t.Fatal("missing group send frame")
		}
	}
	if !recipients["13800138000"] || !recipients["13900139000"] {
		t.Fatalf("recipients = %#v", recipients)
	}
}

func TestBatchStatusDistinguishesPartialFailure(t *testing.T) {
	tests := []struct {
		response batchSendResponse
		want     int
	}{
		{response: batchSendResponse{AcceptedCount: 2}, want: http.StatusAccepted},
		{response: batchSendResponse{AcceptedCount: 1, FailedCount: 1}, want: http.StatusMultiStatus},
		{response: batchSendResponse{FailedCount: 2}, want: http.StatusBadGateway},
	}
	for _, test := range tests {
		if got := batchStatus(test.response); got != test.want {
			t.Fatalf("batchStatus(%+v) = %d, want %d", test.response, got, test.want)
		}
	}
}
