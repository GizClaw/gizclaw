package giztestcmd

import (
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"testing"
)

func integrityChunk(id string, bos, eos bool, data string) *genx.MessageChunk {
	return &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte(data)}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: bos, EndOfStream: eos}}
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
