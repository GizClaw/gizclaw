package dashscoperealtime

import (
	"encoding/binary"
	"math"
	"testing"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func dashScopeTestPCM(samples int) []byte {
	pcm := make([]byte, 0, samples*2)
	for i := range samples {
		value := int16(8000 * math.Sin(2*math.Pi*200*float64(i)/outputSampleRate))
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(value))
	}
	return pcm
}

func runDashScopeAudio(t *testing.T, speechRatePercent int, events []*dashscope.RealtimeEvent) []*genx.MessageChunk {
	t.Helper()
	session := newDashScopeToolSession(events)
	transformer := newTransformer(nil, withSpeechRatePercent(speechRatePercent))
	transformer.realtime = &dashScopeFixedOpener{session: session}
	output, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks, err := collectDashScopeToolOutput(output)
	if err != nil {
		t.Fatalf("collect output: %v", err)
	}
	return chunks
}

func dashScopeAudioBytes(t *testing.T, chunks []*genx.MessageChunk) (int, bool) {
	t.Helper()
	var total int
	var eos bool
	for _, chunk := range chunks {
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || chunk.Role != genx.RoleModel {
			continue
		}
		total += len(blob.Data)
		if chunk.IsEndOfStream() {
			eos = true
		}
	}
	return total, eos
}

func TestTransformerStretchesAudioToSpeechRate(t *testing.T) {
	pcm := dashScopeTestPCM(outputSampleRate / 2)
	events := []*dashscope.RealtimeEvent{{Type: dashscope.EventTypeResponseCreated, ResponseID: "response-1"}}
	for offset := 0; offset < len(pcm); offset += 1920 {
		events = append(events, &dashscope.RealtimeEvent{
			Type: dashscope.EventTypeResponseAudioDelta, ResponseID: "response-1", Audio: pcm[offset:min(offset+1920, len(pcm))],
		})
	}
	events = append(events, &dashscope.RealtimeEvent{Type: dashscope.EventTypeResponseAudioDone, ResponseID: "response-1"})

	slow, eos := dashScopeAudioBytes(t, runDashScopeAudio(t, 50, events))
	if !eos || slow != 2*len(pcm) {
		t.Fatalf("50%% audio = %d bytes eos=%v, want %d bytes with EOS", slow, eos, 2*len(pcm))
	}
	normal, eos := dashScopeAudioBytes(t, runDashScopeAudio(t, 0, events))
	if !eos || normal != len(pcm) {
		t.Fatalf("default audio = %d bytes eos=%v, want %d unchanged", normal, eos, len(pcm))
	}
}

func TestDashScopeSpeechRateDropsInterruptedResponse(t *testing.T) {
	rate := newDashScopeSpeechRate(70)
	pcm := dashScopeTestPCM(4800)
	if _, err := rate.audio("response-1", pcm); err != nil {
		t.Fatal(err)
	}
	if _, err := rate.audio("response-2", pcm); err != nil {
		t.Fatal(err)
	}
	if tail := rate.flush("response-1"); tail != nil {
		t.Fatalf("flush(interrupted response) = %d bytes, want none", len(tail))
	}
	if tail := rate.flush("response-2"); len(tail) == 0 {
		t.Fatal("flush(current response) returned no audio")
	}
	if newDashScopeSpeechRate(100) != nil || newDashScopeSpeechRate(0) != nil {
		t.Fatal("normal speech rate should not stretch")
	}
}

func TestNewValidatesSpeechRatePercent(t *testing.T) {
	client := dashscope.NewClient("test")
	for _, tc := range []struct {
		percent int
		format  string
		ok      bool
	}{
		{0, dashscope.AudioFormatMP3, true},
		{100, dashscope.AudioFormatWAV, true},
		{70, "", true},
		{200, dashscope.AudioFormatPCM16, true},
		{49, "", false},
		{201, "", false},
		{70, dashscope.AudioFormatMP3, false},
		{150, dashscope.AudioFormatWAV, false},
	} {
		_, err := New(Config{Client: client, Model: "qwen-omni-turbo-realtime", SpeechRatePercent: tc.percent, OutputAudioFormat: tc.format})
		if (err == nil) != tc.ok {
			t.Fatalf("New(rate %d, format %q) error = %v, want ok=%v", tc.percent, tc.format, err, tc.ok)
		}
	}
}
