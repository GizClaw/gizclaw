package minimaxtts

import (
	"bytes"
	"context"
	"encoding/hex"
	"math"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/minimax-go"
)

func TestNormalizeFormatAndOggOpusSampleRate(t *testing.T) {
	if normalizeFormat("") != "mp3" || normalizeFormat(" OGG ") != FormatOggOpus || normalizeFormat("ogg_opus") != FormatOggOpus || normalizeFormat("wav") != "wav" {
		t.Fatal("normalizeFormat() aliases are incorrect")
	}
	for configured, want := range map[int]int{8000: 8000, 16000: 16000, 24000: 24000, 22050: 24000, 32000: 24000, 44100: 24000} {
		if got := (&Transformer{sampleRate: configured}).oggOpusSampleRate(); got != want {
			t.Errorf("oggOpusSampleRate(%d) = %d, want %d", configured, got, want)
		}
	}
}

func TestSynthesizeOggOpusTranscodesProviderPCM(t *testing.T) {
	// 200 ms of a 440 Hz tone at the 24 kHz fallback rate, split across two
	// provider frames so the encoder sees partial Opus frames.
	pcm := sinePCM16(24000, 440, 4800)
	audioHex := []string{hex.EncodeToString(pcm[:3000]), hex.EncodeToString(pcm[3000:])}
	requestBody := make(chan map[string]any, 1)
	server := newMiniMaxTTSTestServerWithAudio(t, requestBody, audioHex)
	defer server.Close()

	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "test", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("minimax.NewClient() error = %v", err)
	}
	transformer, err := New(Config{Client: client, VoiceID: "voice", Format: "ogg", SampleRate: 32000})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if transformer.mimeType() != "audio/ogg" {
		t.Fatalf("mimeType() = %q, want audio/ogg", transformer.mimeType())
	}
	var encoded []byte
	emissions := 0
	if err := transformer.synthesize(context.Background(), "hello", streamkit.TTSMeta{}, "audio/ogg", func(data []byte) error {
		if len(data) > 0 {
			emissions++
		}
		encoded = append(encoded, data...)
		return nil
	}); err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	if emissions < 2 {
		t.Fatalf("ogg emissions = %d, want incremental output", emissions)
	}
	if !bytes.HasPrefix(encoded, []byte("OggS")) {
		t.Fatalf("encoded output does not start with an Ogg page: %x", encoded[:min(len(encoded), 8)])
	}

	body := <-requestBody
	audioSetting, ok := body["audio_setting"].(map[string]any)
	if !ok || audioSetting["format"] != "pcm" || audioSetting["sample_rate"] != float64(24000) {
		t.Fatalf("audio_setting = %#v, want pcm at 24000 Hz", body["audio_setting"])
	}

	var head []byte
	packets := 0
	for packet, err := range ogg.Packets(bytes.NewReader(encoded)) {
		if err != nil {
			t.Fatalf("decode ogg: %v", err)
		}
		if codecconv.IsOpusHeadPacket(packet.Data) {
			head = packet.Data
			continue
		}
		if codecconv.IsOpusTagsPacket(packet.Data) || len(packet.Data) == 0 {
			continue
		}
		packets++
	}
	sampleRate, channels, err := codecconv.ParseOpusHeadPacket(head)
	if err != nil {
		t.Fatalf("ParseOpusHeadPacket() error = %v", err)
	}
	if sampleRate != 24000 || channels != 1 {
		t.Fatalf("OpusHead = %d Hz %d ch, want 24000 Hz mono", sampleRate, channels)
	}
	// 200 ms at 20 ms per Opus frame yields 10 audio packets.
	if packets != 10 {
		t.Fatalf("opus audio packets = %d, want 10", packets)
	}
}

func TestSynthesizeOggOpusKeepsOpusCompatibleSampleRate(t *testing.T) {
	pcm := sinePCM16(16000, 440, 1600)
	requestBody := make(chan map[string]any, 1)
	server := newMiniMaxTTSTestServerWithAudio(t, requestBody, []string{hex.EncodeToString(pcm)})
	defer server.Close()

	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "test", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("minimax.NewClient() error = %v", err)
	}
	transformer, err := New(Config{Client: client, VoiceID: "voice", Format: FormatOggOpus, SampleRate: 16000})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var encoded []byte
	if err := transformer.synthesize(context.Background(), "hello", streamkit.TTSMeta{}, "audio/ogg", func(data []byte) error {
		encoded = append(encoded, data...)
		return nil
	}); err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	body := <-requestBody
	audioSetting, _ := body["audio_setting"].(map[string]any)
	if audioSetting["format"] != "pcm" || audioSetting["sample_rate"] != float64(16000) {
		t.Fatalf("audio_setting = %#v, want pcm at 16000 Hz", body["audio_setting"])
	}
	opusPackets := 0
	for _, err := range codecconv.OggOpusPackets(bytes.NewReader(encoded)) {
		if err != nil {
			t.Fatalf("OggOpusPackets() error = %v", err)
		}
		opusPackets++
	}
	if opusPackets != 5 {
		t.Fatalf("opus audio packets = %d, want 5 for 100 ms", opusPackets)
	}
}

func TestSynthesizeOggOpusEmitsNothingForEmptyProviderAudio(t *testing.T) {
	server := newMiniMaxTTSTestServerWithAudio(t, nil, nil)
	defer server.Close()

	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "test", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("minimax.NewClient() error = %v", err)
	}
	transformer, err := New(Config{Client: client, VoiceID: "voice", Format: FormatOggOpus})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	emitted := 0
	if err := transformer.synthesize(context.Background(), "hello", streamkit.TTSMeta{}, "audio/ogg", func(data []byte) error {
		emitted += len(data)
		return nil
	}); err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	if emitted != 0 {
		t.Fatalf("empty provider audio emitted %d ogg bytes, want 0", emitted)
	}
}

func sinePCM16(sampleRate int, frequency float64, samples int) []byte {
	out := make([]byte, samples*2)
	for index := range samples {
		value := int16(math.Sin(2*math.Pi*frequency*float64(index)/float64(sampleRate)) * 12000)
		out[index*2] = byte(value)
		out[index*2+1] = byte(uint16(value) >> 8)
	}
	return out
}
