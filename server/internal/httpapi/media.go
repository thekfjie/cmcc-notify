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
	if strings.TrimSpace(r.FormValue("to")) != "" {
		writeError(w, http.StatusBadRequest, "to is not supported; use account or group")
		return
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
	targets, groupName, err := resolveTargets(cfg, r.FormValue("account"), r.FormValue("group"))
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
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	result, batch, mediaURL, size, err := s.uploadAndSendMediaToTargets(
		ctx, targets, groupName, header.Filename, tmpPath, mediaType, mimeType, strings.TrimSpace(r.FormValue("caption")),
	)
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

func (s *Server) uploadAndSendMediaToTargets(ctx context.Context, targets []sendTarget, groupName, fileName, filePath string, mediaType cmcc.MediaType, mimeType, caption string) (cmcc.SendResult, *batchSendResponse, string, int64, error) {
	if groupName == "" {
		client, err := s.connectedClient(ctx, targets[0].Account)
		if err != nil {
			return cmcc.SendResult{}, nil, "", 0, err
		}
		mediaURL, size, err := client.Upload(ctx, fileName, filePath, mediaType)
		if err != nil {
			return cmcc.SendResult{}, nil, "", 0, err
		}
		result, err := sendMediaWithCompanionText(ctx, client, caption, cmcc.MediaMessage{
			MediaType: mediaType, MediaURL: mediaURL,
			MediaFileName: fileName, MediaSize: size, MediaMIMEType: mimeType,
		})
		return result, nil, mediaURL, size, err
	}

	response := &batchSendResponse{Mode: "channel_fanout", Group: groupName, Total: len(targets)}
	var firstMediaURL string
	var uploadedSize int64
	for _, target := range targets {
		item := channelSendResult{Channel: target.Account.Name}
		client, err := s.connectedClient(ctx, target.Account)
		if err == nil {
			var mediaURL string
			var size int64
			mediaURL, size, err = client.Upload(ctx, fileName, filePath, mediaType)
			if firstMediaURL == "" && mediaURL != "" {
				firstMediaURL, uploadedSize = mediaURL, size
			}
			if err == nil {
				item.SendResult, err = sendMediaWithCompanionText(ctx, client, caption, cmcc.MediaMessage{
					MediaType: mediaType, MediaURL: mediaURL,
					MediaFileName: fileName, MediaSize: size, MediaMIMEType: mimeType,
				})
			}
		}
		if err != nil {
			item.Error = err.Error()
			response.FailedCount++
		} else {
			response.AcceptedCount++
		}
		response.Results = append(response.Results, item)
	}
	return cmcc.SendResult{}, response, firstMediaURL, uploadedSize, nil
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
