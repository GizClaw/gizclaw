package agenthost

import (
	"encoding/binary"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
)

const (
	// audioLeadAudibleLevel is the sample magnitude, about -42 dBFS, at or
	// above which decoded route audio counts as audible. Provider TTS opens
	// every response with a low-level lead-in (measured peaks up to about
	// 210) before speech, whose onsets measure 300 and above.
	audioLeadAudibleLevel = 256
	// audioLeadPreroll is the quiet audio kept ahead of the first audible
	// sample, so a soft onset below the level is not clipped.
	audioLeadPreroll = 60 * time.Millisecond
	// audioLeadMaxTrim bounds how much quiet audio a route may drop. Audio
	// that stays quiet longer is deliberate and is played from there on.
	audioLeadMaxTrim = 2 * time.Second
)

// audioLeadTrimmer drops the quiet lead-in ahead of a route's first audible
// sample. The listener hears nothing in it, yet the device must play it
// before the reply starts, and the downlink paces it like any other audio, so
// every provider response would otherwise start that much later. Only the
// route's start is trimmed: quiet audio after the first audible sample is part
// of the reply's pacing between sentences and plays unchanged.
type audioLeadTrimmer struct {
	done    bool
	format  pcm.Format
	held    []byte
	dropped time.Duration
}

// trim returns the chunks to write for chunk. Until the route's first
// audible sample it holds quiet PCM back, keeping only the preroll.
func (t *audioLeadTrimmer) trim(chunk pcm.Chunk) []pcm.Chunk {
	if t.done {
		return []pcm.Chunk{chunk}
	}
	data, ok := chunk.(*pcm.DataChunk)
	if !ok || (len(t.held) > 0 && data.Format() != t.format) {
		// Only decoded PCM can be inspected, and held audio cannot be joined
		// across a format change, so either ends the lead-in as it stands.
		t.done = true
		return append(t.release(), chunk)
	}
	t.format = data.Format()
	frameBytes := t.format.Channels() * 2
	onset := audibleFrameOffset(data.Data, frameBytes)
	if onset < 0 {
		t.held = append(t.held, data.Data...)
		t.keepPreroll()
		if t.dropped >= audioLeadMaxTrim {
			t.done = true
			return t.release()
		}
		return nil
	}
	t.done = true
	t.held = append(t.held, data.Data[:onset]...)
	t.keepPreroll()
	out := make([]byte, 0, len(t.held)+len(data.Data)-onset)
	out = append(out, t.held...)
	out = append(out, data.Data[onset:]...)
	t.held = nil
	return []pcm.Chunk{t.format.DataChunk(out)}
}

// keepPreroll drops held audio older than the preroll, never dropping more
// than audioLeadMaxTrim in total.
func (t *audioLeadTrimmer) keepPreroll() {
	frameBytes := t.format.Channels() * 2
	keep := int(t.format.BytesInDuration(audioLeadPreroll))
	keep -= keep % frameBytes
	budget := int(t.format.BytesInDuration(audioLeadMaxTrim - t.dropped))
	budget -= budget % frameBytes
	if excess := min(len(t.held)-keep, budget); excess > 0 {
		t.dropped += t.format.Duration(int64(excess))
		t.held = append(t.held[:0], t.held[excess:]...)
	}
}

// release returns the held audio for writing and stops holding.
func (t *audioLeadTrimmer) release() []pcm.Chunk {
	if len(t.held) == 0 {
		return nil
	}
	held := t.held
	t.held = nil
	return []pcm.Chunk{t.format.DataChunk(held)}
}

// audibleFrameOffset returns the byte offset of the first frame holding an
// audible sample in little-endian 16-bit PCM, or -1 when every sample is
// quiet.
func audibleFrameOffset(data []byte, frameBytes int) int {
	for offset := 0; offset+1 < len(data); offset += 2 {
		sample := int16(binary.LittleEndian.Uint16(data[offset:]))
		if sample >= audioLeadAudibleLevel || sample <= -audioLeadAudibleLevel {
			return offset - offset%frameBytes
		}
	}
	return -1
}
