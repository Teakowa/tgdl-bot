package util

import (
	"net/url"
	"regexp"
	"strings"
)

var telegramURLPattern = regexp.MustCompile(`https?://(?:t\.me|telegram\.me)/(?:c/\d+/\d+|[A-Za-z0-9_]+/\d+)`)

// ExtractFirstTelegramURL finds the first supported Telegram message URL in text.
func ExtractFirstTelegramURL(text string) (string, bool) {
	match := telegramURLPattern.FindString(text)
	if match == "" {
		return "", false
	}

	parsed, err := url.Parse(match)
	if err != nil {
		return "", false
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimSpace(parsed.String()), true
}
