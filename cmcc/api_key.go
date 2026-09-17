package cmcc

import (
	"regexp"
	"strings"
)

var apiKeyPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])((?:ak|app)_[A-Za-z0-9][A-Za-z0-9_-]*)`)

// ExtractAPIKey returns the first CMCC Channel API Key found in value. It
// accepts either a bare key or the complete authorization message delivered by
// New Message ClawBot. The returned value never contains surrounding message
// text.
func ExtractAPIKey(value string) (string, bool) {
	match := apiKeyPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

// NormalizeAPIKey extracts a key when possible and otherwise returns the
// trimmed input. Keeping invalid input intact lets existing validation paths
// return their normal generic error without logging or echoing the secret.
func NormalizeAPIKey(value string) string {
	if key, ok := ExtractAPIKey(value); ok {
		return key
	}
	return strings.TrimSpace(value)
}

func ValidAPIKey(key string) bool {
	key = strings.TrimSpace(key)
	extracted, ok := ExtractAPIKey(key)
	return ok && extracted == key
}
