package bot

import (
	"net/url"
	"regexp"
	"strings"

	"tgdl-bot/internal/util"
)

func ExtractTaskURL(text string) (string, bool) {
	return util.ExtractFirstTelegramURL(text)
}

func IsAllowedUser(allowed []int64, userID int64) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, id := range allowed {
		if id == userID {
			return true
		}
	}
	return false
}

var numericTargetPattern = regexp.MustCompile(`^-?\d+$`)
var publicTargetPattern = regexp.MustCompile(`^[A-Za-z0-9_]{5,}$`)

func NormalizeTargetPeer(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if numericTargetPattern.MatchString(raw) {
		return raw, true
	}
	if strings.HasPrefix(raw, "@") {
		raw = strings.TrimPrefix(raw, "@")
	}
	if publicTargetPattern.MatchString(raw) {
		return raw, true
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", false
	}
	host := strings.ToLower(parsed.Host)
	if host != "t.me" && host != "telegram.me" {
		return "", false
	}

	path := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	if path == "" || strings.HasPrefix(path, "+") {
		return "", false
	}
	if strings.Contains(path, "/") {
		return "", false
	}
	if !publicTargetPattern.MatchString(path) {
		return "", false
	}
	return path, true
}
