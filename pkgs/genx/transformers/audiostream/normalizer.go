package audiostream

import (
	"bytes"
	"strings"
)

// Normalizer makes chunks of one audio MIME stream safe to concatenate while
// preserving that stream's codec and MIME type. Formats that need no special
// handling are passed through unchanged.
//
// The current format-specific handling removes ID3v2 metadata from MP3 byte
// streams. Additional formats may be handled without changing callers.
type Normalizer struct {
	mimeType string
	pending  []byte
}

// NewNormalizer creates an audio stream normalizer for mimeType.
func NewNormalizer(mimeType string) *Normalizer {
	return &Normalizer{mimeType: normalizeMIME(mimeType)}
}

// Normalize consumes the next audio chunk and returns bytes that are ready to
// append to the same output stream. The returned bytes retain the input codec
// and MIME type.
func (n *Normalizer) Normalize(data []byte) []byte {
	if n == nil || len(data) == 0 {
		return data
	}
	if !n.isMP3() {
		return data
	}
	n.pending = append(n.pending, data...)
	return n.drainMP3(false)
}

// Flush returns any buffered audio bytes that remain at the end of the input
// stream. Incomplete trailing format metadata is discarded.
func (n *Normalizer) Flush() []byte {
	if n == nil || !n.isMP3() {
		return nil
	}
	return n.drainMP3(true)
}

func (n *Normalizer) isMP3() bool {
	switch n.mimeType {
	case "audio/mpeg", "audio/mp3", "audio/x-mpeg":
		return true
	default:
		return false
	}
}

func (n *Normalizer) drainMP3(final bool) []byte {
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
			const signatureOverlap = 2
			if final {
				out = append(out, n.pending...)
				n.pending = nil
				return out
			}
			if len(n.pending) <= signatureOverlap {
				return out
			}
			emit := len(n.pending) - signatureOverlap
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

func normalizeMIME(mimeType string) string {
	mimeType, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(mimeType)), ";")
	return strings.TrimSpace(mimeType)
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
