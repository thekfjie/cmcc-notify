package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/thekfjie/cmcc-notify/cmcc"
)

type gotifyMessage struct {
	ID       uint                   `json:"id"`
	AppID    uint                   `json:"appid"`
	Title    string                 `json:"title"`
	Message  string                 `json:"message"`
	Priority int                    `json:"priority"`
	Extras   map[string]interface{} `json:"extras"`
	Date     time.Time              `json:"date"`
}

type mediaExtra struct {
	To           string `json:"to"`
	Phone        string `json:"phone"`
	Type         string `json:"type"`
	URL          string `json:"url"`
	MediaURL     string `json:"media_url"`
	Caption      string `json:"caption"`
	FileName     string `json:"file_name"`
	MIMEType     string `json:"mime_type"`
	ThumbnailURL string `json:"thumbnail_url"`
}

func (p *GotifyPlugin) routeMessage(ctx context.Context, msg gotifyMessage) {
	if msg.ID == 0 {
		return
	}
	cfg := p.configSnapshot()
	accounts := p.accountsFor(msg, cfg)
	for _, accountName := range accounts {
		seenKey := fmt.Sprintf("%d/%s", msg.ID, accountName)
		if p.state.seenRecently(seenKey) {
			continue
		}
		if err := p.forwardToAccount(ctx, msg, accountName, cfg); err != nil {
			p.recordFailure(err)
			continue
		}
		p.state.markSeen(seenKey)
	}
}

func (p *GotifyPlugin) accountsFor(msg gotifyMessage, cfg *Config) []string {
	if len(cfg.Routes) == 0 {
		result := make([]string, 0, len(cfg.Accounts))
		for _, account := range cfg.Accounts {
			if account.Enabled {
				result = append(result, account.Name)
			}
		}
		return result
	}
	selected := make(map[string]struct{})
	for _, route := range cfg.Routes {
		if msg.Priority < route.MinimumPriority || !applicationMatches(route.Applications, msg.AppID) {
			continue
		}
		for _, account := range route.Accounts {
			selected[account] = struct{}{}
		}
	}
	result := make([]string, 0, len(selected))
	for _, account := range cfg.Accounts {
		if _, ok := selected[account.Name]; ok && account.Enabled {
			result = append(result, account.Name)
		}
	}
	return result
}

func applicationMatches(ids []uint, appID uint) bool {
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == appID {
			return true
		}
	}
	return false
}

func (p *GotifyPlugin) forwardToAccount(ctx context.Context, msg gotifyMessage, accountName string, cfg *Config) error {
	accountCfg, ok := findAccount(cfg, accountName)
	if !ok {
		return fmt.Errorf("account %q not found", accountName)
	}
	client := p.clientFor(accountCfg)
	if client == nil {
		return fmt.Errorf("account %q is unavailable", accountName)
	}
	if !client.Connected() {
		connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := client.Connect(connectCtx)
		cancel()
		if err != nil {
			return err
		}
	}
	if extra, ok := extractMedia(msg.Extras); ok {
		mediaURL := extra.URL
		if mediaURL == "" {
			mediaURL = extra.MediaURL
		}
		if mediaURL == "" {
			return fmt.Errorf("account %q: media extra has no URL", accountName)
		}
		to := firstNonEmpty(extra.To, extra.Phone, accountCfg.DefaultTo, cfgDefaultTo(cfg))
		if strings.TrimSpace(to) == "" {
			return fmt.Errorf("account %q has no default_to; media forwarding requires a recipient", accountName)
		}
		_, err := client.SendMedia(ctx, cmcc.MediaMessage{
			To:            to,
			MediaType:     parseMediaType(extra.Type, extra.MIMEType),
			Content:       firstNonEmpty(extra.Caption, msg.Message),
			MediaURL:      mediaURL,
			ThumbnailURL:  extra.ThumbnailURL,
			MediaFileName: extra.FileName,
			MediaMIMEType: extra.MIMEType,
		})
		if err != nil {
			return err
		}
		p.recordSuccess()
		return nil
	}
	to := accountCfg.DefaultTo
	if to == "" {
		to = cfgDefaultTo(cfg)
	}
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("account %q has no default_to; text forwarding requires a recipient", accountName)
	}
	text := msg.Message
	if cfg.IncludeTitle && msg.Title != "" {
		text = msg.Title + cfg.TitleSeparator + text
	}
	_, err := client.SendText(ctx, to, text)
	if err != nil {
		return err
	}
	p.recordSuccess()
	return nil
}

func findAccount(cfg *Config, name string) (AccountConfig, bool) {
	for _, account := range cfg.Accounts {
		if account.Name == name {
			return account, true
		}
	}
	return AccountConfig{}, false
}

func cfgDefaultTo(cfg *Config) string {
	for _, account := range cfg.Accounts {
		if account.DefaultTo != "" {
			return account.DefaultTo
		}
	}
	return ""
}

func extractMedia(extras map[string]interface{}) (mediaExtra, bool) {
	if len(extras) == 0 {
		return mediaExtra{}, false
	}
	for _, key := range []string{"cmcc::media", "cmcc.media", "cmcc"} {
		value, ok := extras[key]
		if !ok {
			continue
		}
		b, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var media mediaExtra
		if json.Unmarshal(b, &media) == nil && (media.URL != "" || media.MediaURL != "") {
			return media, true
		}
	}
	return mediaExtra{}, false
}

func parseMediaType(kind, mimeType string) cmcc.MediaType {
	switch strings.ToUpper(kind) {
	case "IMAGE":
		return cmcc.MediaImage
	case "AUDIO":
		return cmcc.MediaAudio
	case "VIDEO":
		return cmcc.MediaVideo
	case "FILE":
		return cmcc.MediaFile
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return cmcc.MediaImage
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "audio/") {
		return cmcc.MediaAudio
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "video/") {
		return cmcc.MediaVideo
	}
	return cmcc.MediaFile
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
