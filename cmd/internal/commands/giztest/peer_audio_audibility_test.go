package giztestcmd

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

var (
	audibleOpusOnce    sync.Once
	audibleOpusPackets [][]byte
	audibleOpusErr     error
)

// testAudibleOpus returns a real 20 ms Opus packet carrying a tone well above
// the audibility threshold, so first-audio tests exercise the same decode path
// live downlink audio takes.
func testAudibleOpus(t *testing.T) []byte {
	t.Helper()
	return testAudibleOpusPackets(t, 1)[0]
}

// testQuietOpusPackets encodes low-level noise like the lead-in provider TTS
// output opens with: below the audibility threshold but never exactly zero.
func testQuietOpusPackets(t *testing.T, count int) [][]byte {
	t.Helper()
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder() error = %v", err)
	}
	defer encoder.Close()
	frame := make([]int16, 320)
	for i := range frame {
		frame[i] = int16((i*37)%121 - 60)
	}
	packets := make([][]byte, count)
	for i := range packets {
		if packets[i], err = encoder.Encode(frame, len(frame)); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}
	return packets
}

func testAudibleOpusPackets(t *testing.T, count int) [][]byte {
	t.Helper()
	audibleOpusOnce.Do(func() {
		encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
		if err != nil {
			audibleOpusErr = err
			return
		}
		defer encoder.Close()
		frame := make([]int16, 320)
		for i := range frame {
			frame[i] = 8000
			if i%32 >= 16 {
				frame[i] = -8000
			}
		}
		for range 8 {
			packet, err := encoder.Encode(frame, len(frame))
			if err != nil {
				audibleOpusErr = err
				return
			}
			audibleOpusPackets = append(audibleOpusPackets, packet)
		}
	})
	if audibleOpusErr != nil {
		t.Fatalf("encode audible Opus: %v", audibleOpusErr)
	}
	if count > len(audibleOpusPackets) {
		t.Fatalf("audible Opus packets = %d, want at most %d", count, len(audibleOpusPackets))
	}
	return audibleOpusPackets[:count]
}

func TestPeerAudioAudibilityClassifiesSilenceAndSpeech(t *testing.T) {
	silent, err := appendRealtimeTailSilence(nil, 60*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	var audibility peerAudioAudibility
	defer audibility.Close()

	class := audibility.classify(assistantBlob("s1", silent[0], false))
	if class.audible || class.silentPrefix != 20*time.Millisecond || class.digitalSilencePrefix != 20*time.Millisecond {
		t.Fatalf("mixer silence class = %+v, want 20ms of digital silence", class)
	}
	oggSilence, _ := testOggOpus(t)
	class = audibility.classify(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: oggSilence}, Ctrl: &genx.StreamCtrl{StreamID: "s2", Label: "assistant"}})
	if class.audible || class.silentPrefix != 40*time.Millisecond || class.digitalSilencePrefix != 40*time.Millisecond {
		t.Fatalf("Ogg silence class = %+v, want 40ms of digital silence", class)
	}
	quiet := testQuietOpusPackets(t, 3)
	for _, packet := range quiet {
		class = audibility.classify(assistantBlob("s3", packet, false))
		if class.audible || class.silentPrefix != 20*time.Millisecond || class.digitalSilencePrefix != 0 {
			t.Fatalf("quiet lead-in class = %+v, want 20ms of silence that is not digital silence", class)
		}
	}
	class = audibility.classify(assistantBlob("s1", testAudibleOpus(t), false))
	if !class.audible || class.silentPrefix != 0 {
		t.Fatalf("speech class = %+v, want audible without silent prefix", class)
	}
	class = audibility.classify(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/L16; rate=16000", Data: []byte{0, 0}}})
	if !class.audible {
		t.Fatalf("undecodable audio class = %+v, want audible", class)
	}
	if class = audibility.classify(assistantText("s1", "hello", false)); class.audible || class.silentPrefix != 0 {
		t.Fatalf("text class = %+v, want no audio", class)
	}
}

// Silence the downlink sends ahead of the reply is time the listener still
// waits, so first_audio_ms must land on the first audible frame and the
// silence ahead of it must be reported, split into digital silence and the
// provider's quiet lead-in. The tail of an earlier reply on another stream is
// not charged to this one.
func TestInvokePeerStreamFirstAudioSkipsLeadingSilence(t *testing.T) {
	silent, err := appendRealtimeTailSilence(nil, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	const silenceGap = 80 * time.Millisecond
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		for _, packet := range testQuietOpusPackets(t, 5) {
			stream.in <- assistantBlob("earlier-reply", packet, false)
		}
		stream.in <- assistantText("s1", "hello", false)
		for _, packet := range silent {
			stream.in <- assistantBlob("s1", packet, false)
		}
		for _, packet := range testQuietOpusPackets(t, 3) {
			stream.in <- assistantBlob("s1", packet, false)
		}
		time.Sleep(silenceGap)
		for _, packet := range testAudibleOpusPackets(t, 2) {
			stream.in <- assistantBlob("s1", packet, false)
		}
		finishAssistantTurn(stream, "s1")
	}()
	started := time.Now()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{Mode: "text"}, stream)
	if err != nil {
		t.Fatal(err)
	}
	object := result.assertion.(map[string]any)
	if got := object["leading_silence_ms"]; got != int64(260) {
		t.Fatalf("leading_silence_ms = %#v, want 260", got)
	}
	if got := object["leading_digital_silence_ms"]; got != int64(200) {
		t.Fatalf("leading_digital_silence_ms = %#v, want 200", got)
	}
	if result.evidence["leading_silence_ms"] != int64(260) || result.evidence["leading_digital_silence_ms"] != int64(200) {
		t.Fatalf("evidence lead = %#v / %#v, want 260 / 200", result.evidence["leading_silence_ms"], result.evidence["leading_digital_silence_ms"])
	}
	firstAudio := object["first_audio_ms"].(int64)
	if firstAudio < silenceGap.Milliseconds() || firstAudio > time.Since(started).Milliseconds() {
		t.Fatalf("first_audio_ms = %d, want the first audible frame after %s of silence", firstAudio, silenceGap)
	}
}

func TestListenPeerStreamReportsLeadingSilence(t *testing.T) {
	silent, err := appendRealtimeTailSilence(nil, 40*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	stream := newFakeRelayStream()
	for _, packet := range silent {
		stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "remote", Label: "participant"}}
	}
	result, err := listenPeerStream(context.Background(), stream, listenStep("100ms"), 0)
	if err != nil {
		t.Fatal(err)
	}
	object := result.assertion.(map[string]any)
	if object["first_audio_ms"] != int64(0) || object["leading_silence_ms"] != int64(40) || object["leading_digital_silence_ms"] != int64(40) || object["packets"] != 2 {
		t.Fatalf("silent listen result = %#v, want no first audio and 40ms leading silence", object)
	}
}
