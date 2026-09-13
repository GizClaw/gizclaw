package giztestcmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
	var firstID, secondID string
	firstPackets, secondPackets := 0, 0
	interrupted, sent := false, false
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("read response: %v; first=%d second=%d", err, firstPackets, secondPackets)
		}
		all.observe(chunk)
		blob, audio := chunk.Part.(*genx.Blob)
		if !audio || blob.MIMEType != "audio/opus" || chunk.Ctrl == nil {
			continue
		}
		id := chunk.Ctrl.StreamID
		if firstID == "" {
			firstID = id
		}
		if id == firstID {
			if secondID != "" && len(blob.Data) > 0 {
				t.Fatal("old role audio arrived after new role BOS")
			}
			first.observe(chunk)
			if len(blob.Data) > 0 {
				firstPackets++
			}
			if chunk.IsEndOfStream() {
				interrupted = chunk.Ctrl.Error == "interrupted"
			}
		} else {
			if secondID == "" {
				secondID = id
			}
			if id != secondID {
				t.Fatalf("unexpected third audio stream %q", id)
			}
			second.observe(chunk)
			if len(blob.Data) > 0 {
				secondPackets++
				pacing.observe(time.Now(), [][]byte{blob.Data})
			}
			if chunk.IsEndOfStream() {
				if chunk.Ctrl.Error != "" {
					t.Fatalf("role B ended with %q", chunk.Ctrl.Error)
				}
				break
			}
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
	if !sent || !interrupted || firstPackets < 8 || firstPackets >= len(packets["story.fox"]) {
		t.Fatalf("not a mid-speech interruption: sent=%v interrupted=%v packets=%d", sent, interrupted, firstPackets)
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
