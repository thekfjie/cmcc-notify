package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/thekfjie/cmcc-notify/cmcc"
)

const multipartOverheadBytes = 2 << 20

type mediaUploadResponse struct {
	cmcc.SendResult
	MediaType string `json:"media_type"`
	FileName  string `json:"file_name"`
	MIMEType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	MediaURL  string `json:"media_url"`
}

type mediaUploadBatchResponse struct {
	batchSendResponse
	MediaType string `json:"media_type"`
	FileName  string `json:"file_name"`
	MIMEType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	MediaURL  string `json:"media_url"`
}

func (s *Server) sendMediaUpload(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	if !authorized(r, cfg.AuthToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, cmcc.DefaultMaxUploadBytes+multipartOverheadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart upload or file too large")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	if header.Size > cmcc.DefaultMaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "file exceeds 200 MiB limit")
		return
	}
	accountName := strings.TrimSpace(r.FormValue("account"))
	if accountName == "" && len(cfg.Accounts) == 1 {
		accountName = cfg.Accounts[0].Name
	}
	account, ok := cfg.Account(accountName)
	if !ok || !account.Enabled {
		writeError(w, http.StatusBadRequest, "unknown or disabled account")
		return
	}
	recipients, groupName, err := resolveRecipients(cfg, account, r.FormValue("to"), r.FormValue("group"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		prefix := make([]byte, 512)
		read, readErr := file.Read(prefix)
		if readErr != nil && readErr != io.EOF {
			writeError(w, http.StatusBadRequest, "cannot inspect uploaded file")
			return
		}
		mimeType = http.DetectContentType(prefix[:read])
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			writeError(w, http.StatusBadRequest, "cannot rewind uploaded file")
			return
		}
	}
	mediaType, err := requestedMediaType(r.FormValue("type"), mimeType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tmp, err := os.CreateTemp("", "cmcc-notify-upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot stage uploaded file")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	written, copyErr := io.Copy(tmp, io.LimitReader(file, cmcc.DefaultMaxUploadBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		writeError(w, http.StatusInternalServerError, "cannot stage uploaded file")
		return
	}
	if written > cmcc.DefaultMaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "file exceeds 200 MiB limit")
		return
	}
	client, err := s.client(account)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	if !client.Connected() {
		if err := client.Connect(ctx); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	mediaURL, size, err := client.Upload(ctx, header.Filename, tmpPath, mediaType)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	result, batch, err := sendMediaToRecipients(ctx, client, recipients, groupName, cmcc.MediaMessage{
		MediaType: mediaType, Content: strings.TrimSpace(r.FormValue("caption")), MediaURL: mediaURL,
		MediaFileName: header.Filename, MediaSize: size, MediaMIMEType: mimeType,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if batch != nil {
		response := mediaUploadBatchResponse{
			batchSendResponse: *batch, MediaType: string(mediaType), FileName: header.Filename,
			MIMEType: mimeType, Size: size, MediaURL: mediaURL,
		}
		writeJSON(w, batchStatus(*batch), response)
		return
	}
	writeJSON(w, http.StatusAccepted, mediaUploadResponse{
		SendResult: result, MediaType: string(mediaType), FileName: header.Filename,
		MIMEType: mimeType, Size: size, MediaURL: mediaURL,
	})
}

func requestedMediaType(value, mimeType string) (cmcc.MediaType, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" || value == "AUTO" {
		return parseMediaType("", mimeType), nil
	}
	switch value {
	case string(cmcc.MediaImage):
		return cmcc.MediaImage, nil
	case string(cmcc.MediaAudio):
		return cmcc.MediaAudio, nil
	case string(cmcc.MediaVideo):
		return cmcc.MediaVideo, nil
	case string(cmcc.MediaFile):
		return cmcc.MediaFile, nil
	default:
		return "", fmt.Errorf("unsupported media type")
	}
}
