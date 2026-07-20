package codecconv

import (
	"bytes"
	"strings"
)

func normalizeTTSAudio(mimeType string, data []byte) []byte {
	switch strings.ToLower(mimeType) {
	case "audio/mpeg", "audio/mp3", "audio/x-mpeg":
		return stripID3Tags(data)
	default:
		return data
	}
}

// TTSAudioNormalizer removes provider container headers that are unsafe to
// concatenate across multiple synthesized text segments.
type TTSAudioNormalizer struct {
	mimeType string
	pending  []byte
}

// NewTTSAudioNormalizer creates a streaming audio normalizer for TTS output.
func NewTTSAudioNormalizer(mimeType string) *TTSAudioNormalizer {
	return &TTSAudioNormalizer{mimeType: strings.ToLower(mimeType)}
}

func newTTSAudioNormalizer(mimeType string) *TTSAudioNormalizer {
	return NewTTSAudioNormalizer(mimeType)
}

func (n *TTSAudioNormalizer) Write(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	if !n.isMP3() {
		return data
	}
	n.pending = append(n.pending, data...)
	return n.drain(false)
}

func (n *TTSAudioNormalizer) Flush() []byte {
	if !n.isMP3() {
		return nil
	}
	return n.drain(true)
}

func (n *TTSAudioNormalizer) isMP3() bool {
	switch n.mimeType {
	case "audio/mpeg", "audio/mp3", "audio/x-mpeg":
		return true
	default:
		return false
	}
}

func (n *TTSAudioNormalizer) drain(final bool) []byte {
	var out []byte
	for len(n.pending) > 0 {
		if bytes.HasPrefix(n.pending, []byte("ID3")) {
			size, valid, complete := id3v2TagSizeState(n.pending)
			if valid && !complete {
				if !final {
					return out
				}
				n.pending = nil
				return out
			}
			if !valid {
				if !final && len(n.pending) < 10 {
					return out
				}
				out = append(out, n.pending[0])
				n.pending = n.pending[1:]
				continue
			}
			n.pending = n.pending[size:]
			continue
		}

		idx := bytes.Index(n.pending, []byte("ID3"))
		if idx < 0 {
			keep := 2
			if final {
				out = append(out, n.pending...)
				n.pending = nil
				return out
			}
			if len(n.pending) <= keep {
				return out
			}
			emit := len(n.pending) - keep
			out = append(out, n.pending[:emit]...)
			n.pending = n.pending[emit:]
			return out
		}
		if idx > 0 {
			out = append(out, n.pending[:idx]...)
			n.pending = n.pending[idx:]
			continue
		}
	}
	return out
}

func stripID3Tags(data []byte) []byte {
	data = stripID3v1Tag(data)
	if !bytes.Contains(data, []byte("ID3")) {
		return data
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if size, ok := id3v2TagSize(data[i:]); ok {
			i += size
			continue
		}
		out = append(out, data[i])
		i++
	}
	return out
}

func stripID3v1Tag(data []byte) []byte {
	const id3v1Size = 128
	if len(data) >= id3v1Size && bytes.Equal(data[len(data)-id3v1Size:len(data)-id3v1Size+3], []byte("TAG")) {
		return data[:len(data)-id3v1Size]
	}
	return data
}

func id3v2TagSize(data []byte) (int, bool) {
	size, valid, complete := id3v2TagSizeState(data)
	return size, valid && complete
}

func id3v2TagSizeState(data []byte) (size int, valid bool, complete bool) {
	if len(data) < 10 || !bytes.Equal(data[:3], []byte("ID3")) {
		return 0, false, false
	}
	for _, b := range data[6:10] {
		if b&0x80 != 0 {
			return 0, false, false
		}
	}
	size = int(data[6])<<21 | int(data[7])<<14 | int(data[8])<<7 | int(data[9])
	total := 10 + size
	if data[5]&0x10 != 0 {
		total += 10
	}
	if total > len(data) {
		return total, true, false
	}
	return total, true, true
}
