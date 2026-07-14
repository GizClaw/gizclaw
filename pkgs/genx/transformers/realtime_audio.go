package transformers

import (
	"encoding/binary"
	"strings"
)

func realtimeBaseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func realtimeAudioFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "pcm"
	}
	return format
}

func realtimeAudioSampleRate(sampleRate int) int {
	if sampleRate <= 0 {
		return 16000
	}
	return sampleRate
}

func realtimeAudioChannels(channels int) int {
	if channels <= 0 {
		return 1
	}
	return channels
}

func realtimeStreamKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return "default"
	}
	return streamID
}

func isRealtimeOpusMIME(mimeType string) bool {
	mimeType = realtimeBaseMIME(mimeType)
	return mimeType == "audio/opus" || strings.HasPrefix(mimeType, "audio/ogg")
}

func isRealtimePCMInputMIME(mimeType string) bool {
	mimeType = realtimeBaseMIME(mimeType)
	return strings.HasPrefix(mimeType, "audio/l16") || mimeType == "audio/pcm" || mimeType == "audio/x-pcm"
}

func isRealtimeMP3InputMIME(mimeType string) bool {
	mimeType = realtimeBaseMIME(mimeType)
	return mimeType == "audio/mpeg" || mimeType == "audio/mp3" || mimeType == "audio/x-mpeg" || mimeType == "audio/x-mp3"
}

func realtimePCM16LE(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}
