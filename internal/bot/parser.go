package bot

import "tgdl-bot/internal/util"

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
