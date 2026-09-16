package cmcc

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUpload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/upload" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			return
		}
		if r.FormValue("apiKey") != "ak_test" {
			t.Errorf("apiKey = %q", r.FormValue("apiKey"))
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("file: %v", err)
			return
		}
		defer file.Close()
		if header.Filename != "hello.txt" {
			t.Errorf("filename = %q", header.Filename)
		}
		body, _ := io.ReadAll(file)
		if string(body) != "hello" {
			t.Errorf("body = %q", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 10200, "message": "成功", "data": "https://cdn.example.invalid/hello.txt"})
	}))
	defer srv.Close()

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("ak_test", Config{UploadURL: srv.URL + "/api", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	remote, size, err := client.Upload(context.Background(), "hello.txt", filePath, MediaText)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if remote == "" || size != 5 {
		t.Fatalf("remote=%q size=%d", remote, size)
	}
}

func TestUploadRejectsOversize(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "big.bin")
	if err := os.WriteFile(filePath, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("ak_test", Config{MaxUploadBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Upload(context.Background(), "big.bin", filePath, MediaFile); err == nil {
		t.Fatal("expected oversize error")
	}
}

var _ *multipart.FileHeader
