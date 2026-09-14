package giztestcmd

import (
	"context"
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
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

type voiceCase struct {
	mode apitypes.WorkspaceInputMode
	long bool
}

func runVoiceGiztest(t *testing.T, kind, fixture, fault string, options ...voiceCase) {
	t.Helper()
	var config voiceCase
	if len(options) > 0 {
		config = options[0]
	}
	mode := config.mode
	if mode == "" {
		mode = apitypes.WorkspaceInputModePushToTalk
	}
	root := "../../../../tests/gizclaw-e2e/testdata/" + fixture
	workflowFile := "workflow.json"
	if kind == "flowcraft" {
		workflowFile = "flowcraft.json"
	}
	data, err := os.ReadFile(filepath.Join(root, workflowFile))
	if err != nil {
		t.Fatal(err)
	}
	packets := map[string][][]byte{}
	for i, voice := range []string{"story.fox", "story.bird", "story.default", "story.owl", "story.bear"} {
		packets[voice] = voiceTonePackets(t, 300+200*i)
		if config.long {
			packets[voice] = voiceTonePacketsCount(t, 300+200*i, 160)
		}
	}
	provider := &voiceFixtureProvider{packets: packets, fault: fault}
	// Distinct per-turn payloads detect stale TTS even for consecutive identical roles.
	if kind == "flowcraft" {
		for turn := range 8 {
			voices := make(map[string][][]byte)
			for i, role := range []string{"fox", "bird", "default", "owl", "bear"} {
				voices["story."+role] = voiceTonePacketsCount(t, 300+200*i+3*turn, len(packets["story."+role]))
			}
			provider.turnPackets = append(provider.turnPackets, voices)
		}
	}
	if config.long {
		provider.reply = strings.Repeat("A long character reply with a distinct ending. ", 64)
		if kind == "eino" {
			var workflow map[string]any
			if err := json.Unmarshal(data, &workflow); err != nil {
				t.Fatal(err)
			}
			answer := workflow["graph"].(map[string]any)["nodes"].([]any)[1].(map[string]any)
			answer["type"] = "script"
			answer["language"] = "starlark"
			answer["entrypoint"] = "run"
			answer["source"] = "def run(input):\n  return {\"value\": " + fmt.Sprintf("%q", provider.reply) + "}\n"
			answer["limits"] = map[string]any{"timeout": "100ms", "max_execution_steps": 1000, "max_input_bytes": 4096, "max_output_bytes": 4096}
			data, err = json.Marshal(workflow)
			if err != nil {
				t.Fatal(err)
			}
		}
		provider.delays = []time.Duration{15 * time.Millisecond, 25 * time.Millisecond, 20 * time.Millisecond}
	}
	if config.mode != "" {
		provider.recognize = make(map[string]string)
		for voice, frames := range packets {
			provider.recognize[string(frames[0])] = strings.TrimPrefix(voice, "story.")
		}
		var workflow map[string]any
		if err := json.Unmarshal(data, &workflow); err != nil {
			t.Fatal(err)
		}
		workflow["voice_adapter"].(map[string]any)["asr_model"] = "fixture-asr"
		data, err = json.Marshal(workflow)
		if err != nil {
			t.Fatal(err)
		}
	}
	resources := voiceFixtureResources{}
	service := peergenx.New(peergenx.Service{Models: resources, Voices: resources, Credentials: resources, ProviderTenants: resources, Builder: provider})
	agent := newVoiceFixtureAgent(t, kind, data, service, mode)
	defer agent.(io.Closer).Close()
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 128)
	output, err := agent.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	transport := &voiceFixtureStream{input: input, output: output}
	testDriver := &voiceFixtureDriver{driver: newDriver(false, nil), stream: transport}
	document := "multi-turn.giztest.yaml"
	if config.long {
		document = "long-reply.giztest.yaml"
	}
	doc, err := giztest.LoadDocument(filepath.Join(root, document), testDriver)
	if err != nil {
		t.Fatal(err)
	}
	if config.mode != "" {
		testDriver.audio = make(map[string][]byte)
		for voice, frames := range packets {
			testDriver.audio[strings.TrimPrefix(voice, "story.")] = frames[0]
		}
		testDriver.audio["unknown"] = packets["story.default"][0]
		for i := range doc.Steps {
			doc.Steps[i].PeerStream.Mode = string(config.mode)
			doc.Steps[i].PeerStream.Pacing = "1ms"
		}
	}
	for _, name := range []string{"fox", "bird", "default", "owl", "bear"} {
		spec, exists := doc.Variables[name+"_digest"]
		if !exists {
			continue
		}
		spec.Value = voicePacketDigest(packets["story."+name])
		doc.Variables[name+"_digest"] = spec
	}
	if len(provider.turnPackets) > 0 {
		for i := range doc.Steps {
			role := doc.Steps[i].PeerStream.Input.(string)
			if role == "unknown" {
				role = "default"
			}
			assertion := doc.Steps[i].Expect["/audio_integrity/sha256"]
			assertion.Equals = voicePacketDigest(provider.turnPackets[i]["story."+role])
			doc.Steps[i].Expect["/audio_integrity/sha256"] = assertion
		}
	}
	report := giztest.Run(ctx, []*giztest.Document{doc}, giztest.Options{Driver: testDriver, Parallel: 1, Out: io.Discard})
	encoded, _ := json.Marshal(report)
	if len(report.Tasks) != 1 {
		t.Fatalf("report=%s", encoded)
	}
	if fault != "" {
		if report.Tasks[0].Status == "passed" {
			t.Fatalf("fault %s escaped assertions: %s", fault, encoded)
		}
		want := "/audio_integrity/sha256"
		if fault == "truncate" {
			want = "TTS ended without EOS"
		}
		if fault == "stall" {
			want = "/audio_pacing/"
		}
		if !strings.Contains(report.Tasks[0].Error, want) {
			t.Fatalf("fault %s failed for unexpected reason: %s", fault, encoded)
		}
		t.Logf("%s rejected: %s", fault, report.Tasks[0].Error)
		return
	}
	if report.Tasks[0].Status != "passed" {
		t.Fatalf("report=%s", encoded)
	}
	if provider.calls.Load() != int32(len(doc.Steps)) || provider.peak.Load() != 1 {
		t.Fatalf("TTS calls=%d peak=%d", provider.calls.Load(), provider.peak.Load())
	}
	summary := transport.audioSummary()
	if summary["violations"] != 0 || summary["max_active"] != 1 || summary["open"] != 0 || summary["streams"] != len(doc.Steps) {
		t.Fatalf("cross-turn audio=%v", summary)
	}
	t.Logf("completed turns; distinct voices including default; one Agent invocation; TTS calls=%d peak=%d", provider.calls.Load(), provider.peak.Load())
}

func TestFlowcraftMultiVoiceGiztest(t *testing.T) {
	for _, fault := range []string{"", "wrong-voice", "stall", "overlap"} {
		name := fault
		if name == "" {
			name = "success"
		}
		t.Run(name, func(t *testing.T) { runVoiceGiztest(t, "flowcraft", "multi-role-voices", fault) })
	}
}

func TestMultiRoleVoiceModesGiztest(t *testing.T) {
	for _, kind := range []string{"eino", "flowcraft"} {
		for _, mode := range []apitypes.WorkspaceInputMode{apitypes.WorkspaceInputModePushToTalk, apitypes.WorkspaceInputModeRealtime} {
			t.Run(kind+"/"+string(mode), func(t *testing.T) {
				fixture := "eino-voices"
				if kind == "flowcraft" {
					fixture = "multi-role-voices"
				}
				runVoiceGiztest(t, kind, fixture, "", voiceCase{mode: mode})
			})
		}
	}
}
func TestMultiRoleLongVoiceGiztest(t *testing.T) {
	for _, kind := range []string{"eino", "flowcraft"} {
		for _, fault := range []string{"", "stall", "truncate"} {
			t.Run(kind+"/"+fault, func(t *testing.T) {
				fixture := "eino-voices"
				if kind == "flowcraft" {
					fixture = "multi-role-voices"
				}
				runVoiceGiztest(t, kind, fixture, fault, voiceCase{long: true})
			})
		}
	}
}
