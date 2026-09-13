package giztestcmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
)

// TestMultiRoleVoiceInterrupt covers rapid input through the barge-in contract:
// new input BOS cancels the active reply rather than queueing its remaining audio.
func TestMultiRoleVoiceInterrupt(t *testing.T) {
	for _, kind := range []string{"eino", "flowcraft"} {
		for _, mode := range []apitypes.WorkspaceInputMode{apitypes.WorkspaceInputModePushToTalk, apitypes.WorkspaceInputModeRealtime} {
			t.Run(kind+"/"+string(mode), func(t *testing.T) { runVoiceInterrupt(t, kind, mode) })
		}
	}
}

func runVoiceInterrupt(t *testing.T, kind string, mode apitypes.WorkspaceInputMode) {
	t.Helper()
	packets := map[string][][]byte{"story.fox": voiceTonePacketsCount(t, 300, 160), "story.bird": voiceTonePackets(t, 500), "story.default": voiceTonePackets(t, 700)}
	provider := &voiceFixtureProvider{packets: packets, recognize: map[string]string{string(packets["story.fox"][0]): "fox", string(packets["story.bird"][0]): "bird"}}
	root, file := "eino-voices", "workflow.json"
	if kind == "flowcraft" {
		root, file = "multi-role-voices", "flowcraft.json"
	}
	data, err := os.ReadFile(filepath.Join("../../../../tests/gizclaw-e2e/testdata", root, file))
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]any
	if err := json.Unmarshal(data, &public); err != nil {
		t.Fatal(err)
	}
	public["voice_adapter"].(map[string]any)["asr_model"] = "fixture-asr"
	data, err = json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	resources := voiceFixtureResources{}
	service := peergenx.New(peergenx.Service{Models: resources, Voices: resources, Credentials: resources, ProviderTenants: resources, Builder: provider})
	agent := newVoiceFixtureAgent(t, kind, data, service, mode)
	defer agent.(io.Closer).Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 128)
	output, err := agent.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	push := func(role string) {
		t.Helper()
		for _, chunk := range audioInputChunks(string(mode), genx.NewStreamID(), "audio/opus", [][]byte{packets["story."+role][0]}) {
			if err := input.Add(chunk); err != nil {
				t.Fatal(err)
			}
		}
	}
	push("fox")
	var all, first, second peerAudioIntegrity
	var pacing peerAudioPacing
	var lifecycle voiceInterruptLifecycle
	firstPackets, secondPackets := 0, 0
	sent := false
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("read response: %v; first=%d second=%d", err, firstPackets, secondPackets)
		}
		if err := lifecycle.observe(chunk); err != nil {
			t.Fatal(err)
		}
		all.observe(chunk)
		blob, audio := chunk.Part.(*genx.Blob)
		if !audio || blob.MIMEType != "audio/opus" || chunk.Ctrl == nil {
			if lifecycle.complete() {
				break
			}
			continue
		}
		id := chunk.Ctrl.StreamID
		if id == lifecycle.firstID {
			first.observe(chunk)
			if len(blob.Data) > 0 {
				firstPackets++
			}
		} else {
			second.observe(chunk)
			if len(blob.Data) > 0 {
				secondPackets++
				pacing.observe(time.Now(), [][]byte{blob.Data})
			}
		}
		if lifecycle.complete() {
			break
		}
		// Synchronize to real downlink delivery, not a sleep that could race startup.
		if firstPackets == 8 && !sent {
			if provider.active.Load() != 1 {
				t.Fatal("replacement did not arrive during active TTS")
			}
			sent = true
			push("bird")
		}
	}
	if !sent || !lifecycle.complete() || firstPackets < 8 || firstPackets >= len(packets["story.fox"]) {
		t.Fatalf("not a mid-speech interruption: sent=%v interrupted=%v packets=%d", sent, lifecycle.complete(), firstPackets)
	}
	if secondPackets != 40 || second.summary()["sha256"] != voicePacketDigest(packets["story.bird"]) {
		t.Fatalf("role B lost or mixed packets: %v packets=%d", second.summary(), secondPackets)
	}
	if first.summary()["sha256"] != voicePacketDigest(packets["story.fox"][:firstPackets]) {
		t.Fatal("role A prefix corrupted")
	}
	if all.streams != 2 || all.maxActive != 1 || len(all.active) != 0 || all.violations != 0 {
		t.Fatalf("cross-role lifecycle: %v", all.summary())
	}
	if provider.calls.Load() != 2 {
		t.Fatalf("TTS calls=%d", provider.calls.Load())
	}
	if pacing.summary()["underruns"] != 0 || pacing.summary()["max_interval_ms"].(float64) > 150 {
		t.Fatalf("role B stalled: %v", pacing.summary())
	}
	t.Logf("interrupted A after %d/160 packets; B received 40/40 with exact digest, EOS and no overlap", firstPackets)
}

func voicePacketDigest(packets [][]byte) string {
	h := sha256.New()
	for _, packet := range packets {
		_, _ = h.Write(packet)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// voiceInterruptLifecycle tracks each reply by StreamID and each route by MIME type.
type voiceInterruptLifecycle struct {
	firstID, secondID string
	started           [2]map[string]bool
	ended             [2]map[string]bool
}

func (s *voiceInterruptLifecycle) complete() bool {
	return s.ended[1]["text/plain"] && s.ended[1]["audio/opus"]
}

func (s *voiceInterruptLifecycle) observe(chunk *genx.MessageChunk) error {
	if chunk.Ctrl == nil || chunk.Ctrl.Label != "assistant" {
		return nil
	}
	route, _ := chunk.MIMEType()
	if route != "text/plain" && route != "audio/opus" {
		return nil
	}
	id := chunk.Ctrl.StreamID
	if id == "" {
		return fmt.Errorf("assistant %s has empty StreamID", route)
	}
	if s.firstID == "" {
		s.firstID = id
	}
	turn := 0
	if id == s.firstID {
		if s.secondID != "" {
			return fmt.Errorf("A %s chunk arrived after B started", route)
		}
	} else {
		turn = 1
		if !s.ended[0]["text/plain"] || !s.ended[0]["audio/opus"] {
			return fmt.Errorf("B started before A text/plain and audio/opus interrupted EOS")
		}
		if s.secondID == "" {
			s.secondID = id
		}
		if id != s.secondID {
			return fmt.Errorf("unexpected third response %q", id)
		}
	}
	if s.started[turn] == nil {
		s.started[turn] = make(map[string]bool)
		s.ended[turn] = make(map[string]bool)
	}
	if s.ended[turn][route] {
		return fmt.Errorf("%s %s chunk after EOS", id, route)
	}
	if !s.started[turn][route] && !chunk.IsBeginOfStream() {
		return fmt.Errorf("%s %s missing BOS", id, route)
	}
	if s.started[turn][route] && chunk.IsBeginOfStream() {
		return fmt.Errorf("%s %s duplicate BOS", id, route)
	}
	s.started[turn][route] = true
	if chunk.IsEndOfStream() {
		want := ""
		if turn == 0 {
			want = "interrupted"
		}
		if chunk.Ctrl.Error != want {
			return fmt.Errorf("%s %s EOS error=%q, want %q", id, route, chunk.Ctrl.Error, want)
		}
		s.ended[turn][route] = true
	}
	return nil
}

func TestMultiRoleVoiceInterruptAssertions(t *testing.T) {
	for _, tc := range []struct {
		name                                  string
		omitATextEOS, lateAText, omitBTextEOS bool
		want                                  string
	}{
		{name: "valid"},
		{name: "missing A text interrupted EOS", omitATextEOS: true, want: "before A"},
		{name: "late A text", lateAText: true, want: "after B started"},
		{name: "missing B text EOS", omitBTextEOS: true, want: "incomplete B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var chunks []*genx.MessageChunk
			add := func(id, route string, bos, eos bool, failure string) {
				var part genx.Part = genx.Text("")
				if route == "audio/opus" {
					part = &genx.Blob{MIMEType: route}
				}
				chunks = append(chunks, &genx.MessageChunk{Part: part, Ctrl: &genx.StreamCtrl{
					StreamID: id, Label: "assistant", BeginOfStream: bos, EndOfStream: eos, Error: failure,
				}})
			}
			for _, route := range []string{"text/plain", "audio/opus"} {
				add("A", route, true, false, "")
			}
			if !tc.omitATextEOS {
				add("A", "text/plain", false, true, "interrupted")
			}
			add("A", "audio/opus", false, true, "interrupted")
			add("B", "text/plain", true, false, "")
			if tc.lateAText {
				add("A", "text/plain", false, false, "")
			}
			add("B", "audio/opus", true, false, "")
			add("B", "audio/opus", false, true, "")
			if !tc.omitBTextEOS {
				add("B", "text/plain", false, true, "")
			}
			var state voiceInterruptLifecycle
			var err error
			for _, chunk := range chunks {
				if err = state.observe(chunk); err != nil {
					break
				}
			}
			if err == nil && !state.complete() {
				err = fmt.Errorf("incomplete B routes")
			}
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}
