package giztest

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestAudioInputChunksKeepRealtimeOpen(t *testing.T) {
	for _, tc := range []struct {
		mode    string
		wantEOS bool
	}{
		{mode: "push-to-talk", wantEOS: true},
		{mode: "realtime", wantEOS: false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			chunks := audioInputChunks(tc.mode, "turn", "audio/opus", [][]byte{{1, 2, 3}})
			last := chunks[len(chunks)-1]
			if got := last.IsEndOfStream(); got != tc.wantEOS {
				t.Fatalf("last chunk EndOfStream = %t, want %t", got, tc.wantEOS)
			}
		})
	}
}

func TestPeerStreamTerminalErrorIsNotInterruption(t *testing.T) {
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{Error: "provider quota exceeded"}}
	if got := peerStreamTerminalError(chunk); got != "provider quota exceeded" {
		t.Fatalf("terminal error classification = %q", got)
	}
	chunk.Ctrl.Error = "interrupted"
	if got := peerStreamTerminalError(chunk); got != "" {
		t.Fatalf("interruption classified as terminal error %q", got)
	}
}

func TestPeerStreamAudioCaptureMaxBytes(t *testing.T) {
	vars := &variables{values: map[string]value{
		"audio": {spec: VariableSpec{Direction: "output", Type: "audio", MaxBytes: 4096}},
	}}
	for _, tc := range []struct {
		name string
		step Step
		want int
	}{
		{name: "streamed without buffering", step: Step{}, want: 0},
		{name: "explicit audio capture", step: Step{Capture: map[string]string{"audio": "/audio"}}, want: 4096},
		{name: "unrelated capture", step: Step{Capture: map[string]string{"audio": "/text"}}, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := peerStreamAudioCaptureMaxBytes(tc.step, vars)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("capture max bytes = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDecodeOpusPacketsRejectsEmptyInput(t *testing.T) {
	if _, err := decodeOpusPackets(nil); err == nil {
		t.Fatal("empty Opus input accepted")
	}
}

func TestDecodeOpusPacketsCopiesRawPacket(t *testing.T) {
	input := []byte{1, 2, 3}
	packets, err := decodeOpusPackets(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 9
	if packets[0][0] != 1 {
		t.Fatal("packet aliases caller input")
	}
}

func TestFirstHistoryName(t *testing.T) {
	result := map[string]any{"items": []any{map[string]any{"name": "history-1"}}}
	if got := firstHistoryName(result); got != "history-1" {
		t.Fatalf("firstHistoryName() = %q, want history-1", got)
	}
	if got := firstHistoryName(map[string]any{}); got != "" {
		t.Fatalf("firstHistoryName(empty) = %q", got)
	}
}

func TestStreamIDMatchesDerivedRealtimeSegment(t *testing.T) {
	for _, tc := range []struct {
		actual   string
		expected string
		want     bool
	}{
		{actual: "turn", expected: "turn", want: true},
		{actual: "turn:rt:2", expected: "turn", want: true},
		{actual: "turn-2:rt:1", expected: "turn-1", want: false},
		{actual: "turn", expected: "", want: false},
	} {
		if got := streamIDMatches(tc.actual, tc.expected); got != tc.want {
			t.Errorf("streamIDMatches(%q, %q) = %t, want %t", tc.actual, tc.expected, got, tc.want)
		}
	}
}

func TestAppendRealtimeTailSilence(t *testing.T) {
	input := [][]byte{{1, 2, 3}}
	packets, err := appendRealtimeTailSilence(input, 60*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 4 {
		t.Fatalf("packet count = %d, want 4", len(packets))
	}
	if len(packets[1]) == 0 || len(packets[2]) == 0 || len(packets[3]) == 0 {
		t.Fatal("encoded silence contains an empty Opus packet")
	}
	if got := len(input); got != 1 {
		t.Fatalf("input packet slice mutated to length %d", got)
	}
}
