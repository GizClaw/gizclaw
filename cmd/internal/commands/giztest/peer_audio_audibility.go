package giztestcmd

import (
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// peerAudioAudiblePeak is the decoded sample peak, about -42 dBFS, at or above
// which an Opus frame counts as audible. Provider TTS output opens with a
// low-level lead-in before speech (measured peaks up to about 210), and speech
// onsets measure 300 and above, so the threshold sits between the two.
const peerAudioAudiblePeak = 256

// peerAudioDecodeRate decodes at the downlink mixer rate. Opus decodes any
// packet at any supported rate, so audibility does not depend on the rate the
// sender encoded at.
const peerAudioDecodeRate = int(opus.SampleRate16K)

// peerAudioMaxFrameSamples bounds one decoded Opus packet, which is at most
// 120 ms.
const peerAudioMaxFrameSamples = peerAudioDecodeRate * 120 / 1000

// peerAudioClass describes the audio one received chunk carries.
type peerAudioClass struct {
	// audible reports that the chunk carries at least one audible frame.
	audible bool
	// silentPrefix is the audio before the chunk's first audible frame, or
	// all of its audio when none is audible.
	silentPrefix time.Duration
	// digitalSilencePrefix is the part of silentPrefix that decodes to exact
	// digital silence. It is what the receiver observed, not a claim about
	// who produced it: the downlink mixer fills a track that has no buffered
	// audio with digital silence, and a provider could emit it too.
	digitalSilencePrefix time.Duration
}

// peerAudioAudibility classifies received audio as audible or silent, so
// first-audio timings measure the first sound a listener hears rather than
// the first packet, which may be silence. It keeps one decoder per stream
// because Opus decoding carries state from packet to packet. It is owned by
// one reader goroutine.
type peerAudioAudibility struct {
	decoders map[string]*opus.Decoder
}

func (a *peerAudioAudibility) classify(chunk *genx.MessageChunk) peerAudioClass {
	if chunk == nil {
		return peerAudioClass{}
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil || len(blob.Data) == 0 {
		return peerAudioClass{}
	}
	if !relayOpusMIME(blob.MIMEType) {
		// Only Opus can be decoded here. Any other audio payload is counted
		// as audible, which is how every payload was counted before silence
		// could be told apart.
		return peerAudioClass{audible: true}
	}
	packets, err := decodeOpusPackets(blob.Data)
	if err != nil {
		// The operation loop reports the malformed payload; it is not
		// proven silent, so it is not hidden from the first-audio clock.
		return peerAudioClass{audible: true}
	}
	streamID := ""
	if chunk.Ctrl != nil {
		streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
	}
	decoder := a.decoder(streamID)
	var class peerAudioClass
	for _, packet := range packets {
		if decoder == nil {
			class.audible = true
			return class
		}
		peak, ok := peerAudioPacketPeak(decoder, packet)
		if !ok || peak >= peerAudioAudiblePeak {
			class.audible = true
			return class
		}
		duration := time.Duration(codecconv.OpusPacketRTPTicks(packet)) * time.Second / 48000
		class.silentPrefix += duration
		if peak == 0 {
			class.digitalSilencePrefix += duration
		}
	}
	return class
}

func (a *peerAudioAudibility) decoder(streamID string) *opus.Decoder {
	if decoder := a.decoders[streamID]; decoder != nil {
		return decoder
	}
	decoder, err := opus.NewDecoder(peerAudioDecodeRate, 1)
	if err != nil {
		return nil
	}
	if a.decoders == nil {
		a.decoders = make(map[string]*opus.Decoder)
	}
	a.decoders[streamID] = decoder
	return decoder
}

func (a *peerAudioAudibility) Close() {
	for streamID, decoder := range a.decoders {
		_ = decoder.Close()
		delete(a.decoders, streamID)
	}
}

// peerAudioPacketPeak decodes packet and returns its peak sample magnitude.
// ok is false for a packet that cannot be decoded, which is counted as
// audible for the same reason a malformed payload is.
func peerAudioPacketPeak(decoder *opus.Decoder, packet []byte) (peak int, ok bool) {
	samples, err := decoder.Decode(packet, peerAudioMaxFrameSamples, false)
	if err != nil {
		return 0, false
	}
	for _, sample := range samples {
		peak = max(peak, int(sample), -int(sample))
	}
	return peak, true
}

// peerAudioLead tracks, per stream, the silence received ahead of that
// stream's first audible frame. Keeping streams apart stops the tail of an
// earlier reply, still arriving from the downlink buffer, from being charged
// to the reply that produces the first audible frame.
type peerAudioLead struct {
	streams map[string]peerAudioClass
	first   *peerAudioClass
}

// observe records one received chunk and reports whether it carries the first
// audible frame across all streams.
func (l *peerAudioLead) observe(chunk *genx.MessageChunk, class peerAudioClass) bool {
	if l.first != nil {
		return false
	}
	streamID := ""
	if chunk != nil && chunk.Ctrl != nil {
		streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
	}
	if l.streams == nil {
		l.streams = make(map[string]peerAudioClass)
	}
	lead := l.streams[streamID]
	lead.silentPrefix += class.silentPrefix
	lead.digitalSilencePrefix += class.digitalSilencePrefix
	l.streams[streamID] = lead
	if !class.audible {
		return false
	}
	l.first = &lead
	return true
}

// silence returns the silence, and the digital silence within it, ahead of
// the first audible frame on its stream, or the most seen on any one stream
// while none is audible yet.
func (l *peerAudioLead) silence() (silence, digital time.Duration) {
	if l.first != nil {
		return l.first.silentPrefix, l.first.digitalSilencePrefix
	}
	for _, lead := range l.streams {
		if lead.silentPrefix > silence {
			silence, digital = lead.silentPrefix, lead.digitalSilencePrefix
		}
	}
	return silence, digital
}

// fields returns the lead metrics under their result keys.
func (l *peerAudioLead) fields() map[string]any {
	silence, digital := l.silence()
	return map[string]any{"leading_silence_ms": silence.Milliseconds(), "leading_digital_silence_ms": digital.Milliseconds()}
}
