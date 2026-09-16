package cmcc

import "time"

// MediaType is the media discriminator understood by the CMCC gateway.
type MediaType string

const (
	MediaImage MediaType = "IMAGE"
	MediaText  MediaType = "TEXT"
	MediaAudio MediaType = "AUDIO"
	MediaVideo MediaType = "VIDEO"
	MediaFile  MediaType = "FILE"
)

// MediaMessage is the outbound rich-media envelope. The gateway accepts a
// URL; callers should upload local files first with Client.Upload.
type MediaMessage struct {
	To            string    `json:"to"`
	MediaType     MediaType `json:"mediaType"`
	Content       string    `json:"content,omitempty"`
	MediaURL      string    `json:"mediaUrl"`
	ThumbnailURL  string    `json:"thumbnailUrl,omitempty"`
	MediaFileName string    `json:"mediaFileName,omitempty"`
	MediaSize     int64     `json:"mediaSize,omitempty"`
	MediaMIMEType string    `json:"mediaMimeType,omitempty"`
	MessageID     string    `json:"messageId,omitempty"`
	Timestamp     int64     `json:"timestamp,omitempty"`
}

// IncomingMessage is normalized from the gateway's message and
// media_message frames.
type IncomingMessage struct {
	ID            string    `json:"id"`
	From          string    `json:"from,omitempty"`
	Content       string    `json:"content,omitempty"`
	Timestamp     time.Time `json:"timestamp,omitempty"`
	MediaType     MediaType `json:"mediaType,omitempty"`
	MediaURL      string    `json:"mediaUrl,omitempty"`
	MediaFileName string    `json:"mediaFileName,omitempty"`
	ThumbnailURL  string    `json:"thumbnailUrl,omitempty"`
	MediaSize     int64     `json:"mediaSize,omitempty"`
	MediaMIMEType string    `json:"mediaMimeType,omitempty"`
}

// SendResult intentionally distinguishes socket acceptance from a remote
// delivery acknowledgement. The audited gateway protocol does not expose a
// reliable text delivery acknowledgement.
type SendResult struct {
	MessageID    string `json:"message_id"`
	Accepted     bool   `json:"accepted"`
	Acknowledged bool   `json:"acknowledged"`
}

// Event is emitted by Client.Events for connection and inbound message state.
type Event struct {
	Kind        string
	Message     *IncomingMessage
	Err         error
	Attempt     int
	NextRetryIn time.Duration
}

const (
	EventConnected    = "connected"
	EventDisconnected = "disconnected"
	EventReconnecting = "reconnecting"
	EventMessage      = "message"
	EventError        = "error"
)
