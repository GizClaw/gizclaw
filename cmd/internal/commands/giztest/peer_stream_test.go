package giztest

import (
	"testing"
	"time"
)

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
