package giztestcmd

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func integrityChunk(id string, bos, eos bool, data string) *genx.MessageChunk {
	return &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte(data)}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: bos, EndOfStream: eos}}
}

func TestPeerAudioIntegrityViolationReasons(t *testing.T) {
	var p peerAudioIntegrity
	p.observe(integrityChunk("", true, true, "no route"))
	p.observe(integrityChunk("a", false, true, "missing begin"))
	p.observe(integrityChunk("b", true, false, "first begin"))
	p.observe(integrityChunk("b", true, true, "second begin"))
	p.observe(integrityChunk("a", false, false, "late payload"))
	p.observe(integrityChunk("b", false, true, ""))
	p.observe(integrityChunk("b", true, false, ""))
	want := map[string]int{
		"missing_stream_id":     1,
		"payload_without_begin": 1,
		"end_without_begin":     1,
		"duplicate_begin":       1,
		"payload_after_end":     1,
		"duplicate_end":         1,
		"begin_after_end":       1,
	}
	if p.violations != 7 {
		t.Fatalf("violations = %d, want 7", p.violations)
	}
	reasons, ok := p.summary()["violation_reasons"].(map[string]int)
	if !ok || len(reasons) != len(want) {
		t.Fatalf("violation reasons = %v, want %v", reasons, want)
	}
	for reason, count := range want {
		if reasons[reason] != count {
			t.Errorf("%s = %d, want %d", reason, reasons[reason], count)
		}
	}
}

func TestPeerAudioIntegrity(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		chunks                          []*genx.MessageChunk
		streams, peak, open, violations int
	}{
		{"route EOS", []*genx.MessageChunk{integrityChunk("a", true, false, "a"), {Ctrl: &genx.StreamCtrl{StreamID: "a", EndOfStream: true}}}, 1, 1, 0, 0},
		{"sequential", []*genx.MessageChunk{integrityChunk("a", true, false, "a"), integrityChunk("a", false, true, ""), integrityChunk("b", true, true, "b")}, 2, 1, 0, 0},
		{"overlap", []*genx.MessageChunk{integrityChunk("a", true, false, "a"), integrityChunk("b", true, false, "b"), integrityChunk("a", false, true, ""), integrityChunk("b", false, true, "")}, 2, 2, 0, 0},
		{"late previous turn", []*genx.MessageChunk{integrityChunk("a", true, true, "a"), integrityChunk("b", true, false, "b"), integrityChunk("a", false, false, "late")}, 2, 1, 1, 1},
		{"duplicate child BOS", []*genx.MessageChunk{integrityChunk("a", true, false, "a"), integrityChunk("a", true, true, "b")}, 2, 1, 0, 1},
		{"missing BOS", []*genx.MessageChunk{integrityChunk("a", false, false, "a")}, 0, 0, 0, 1},
		{"missing EOS", []*genx.MessageChunk{integrityChunk("a", true, false, "a")}, 1, 1, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p peerAudioIntegrity
			for _, c := range tc.chunks {
				p.observe(c)
			}
			if p.streams != tc.streams || p.maxActive != tc.peak || len(p.active) != tc.open || p.violations != tc.violations {
				t.Fatalf("summary=%v", p.summary())
			}
		})
	}
	var a, b peerAudioIntegrity
	a.observe(integrityChunk("a", true, true, "voice-a"))
	b.observe(integrityChunk("a", true, true, "voice-b"))
	if a.summary()["sha256"] == b.summary()["sha256"] {
		t.Fatal("different voice payloads have identical evidence")
	}
}
