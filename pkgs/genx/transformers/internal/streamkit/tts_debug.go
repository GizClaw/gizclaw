package streamkit

import (
	"log/slog"
	"os"
	"unicode/utf8"
)

func debugTTSSegment(meta TTSMeta, segment string, flushAll bool) {
	if !ttsDebugEnabled() {
		return
	}
	slog.Info(
		"tts: segment",
		"stream_id", meta.StreamID,
		"name", meta.Name,
		"runes", utf8.RuneCountInString(segment),
		"text", ttsDebugPreview(segment, 120),
		"flush_all", flushAll,
	)
}

func ttsDebugEnabled() bool {
	return os.Getenv("GIZCLAW_TTS_DEBUG") != ""
}

func ttsDebugPreview(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for idx := range text {
		if count == limit {
			return text[:idx] + "..."
		}
		count++
	}
	return text
}
