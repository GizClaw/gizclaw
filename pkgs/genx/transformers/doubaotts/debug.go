package doubaotts

import (
	"os"
	"unicode/utf8"
)

func ttsDebugEnabled() bool {
	return os.Getenv("GIZCLAW_TTS_DEBUG") != ""
}

func ttsDebugPreview(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for index := range text {
		if count == limit {
			return text[:index] + "..."
		}
		count++
	}
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return text
}
