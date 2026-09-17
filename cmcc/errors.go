package cmcc

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidAPIKey = errors.New("cmcc: API key must start with ak_ or app_")
	ErrNotConnected  = errors.New("cmcc: websocket is not connected")
	ErrAuthFailed    = errors.New("cmcc: authentication failed")
	// ErrNoRecipient is retained for source compatibility. Self notifications
	// no longer require an explicit recipient because the API key owns a
	// default binding.
	ErrNoRecipient = errors.New("cmcc: recipient is required")
)

// HTTPError preserves the status and response body for diagnostics without
// including the Authorization/API key headers.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("cmcc: http status %d", e.StatusCode)
	}
	return fmt.Sprintf("cmcc: http status %d: %s", e.StatusCode, e.Body)
}

// Temporary reports whether a failure is safe to retry before a send has
// been accepted by the remote gateway.
func Temporary(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 408 || httpErr.StatusCode == 425 ||
			httpErr.StatusCode == 429 || httpErr.StatusCode >= 500
	}
	return errors.Is(err, ErrNotConnected)
}
