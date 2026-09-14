package giztestcmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func TestSpeakerSegmentsGiztest(t *testing.T) {
	for _, kind := range []string{"eino", "flowcraft"} {
		t.Run(kind, func(t *testing.T) {
			root := "../../../../tests/gizclaw-e2e/testdata/speaker-segments"
			data, err := os.ReadFile(filepath.Join(root, kind+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var spec map[string]json.RawMessage
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatal(err)
			}
			packets := map[string][][]byte{}
			for i, voice := range []string{"story.default", "story.fox", "story.bird"} {
				packets[voice] = voiceTonePackets(t, 300+200*i)
			}
			provider := &voiceFixtureProvider{packets: packets}
			resources := voiceFixtureResources{}
			service := peergenx.New(peergenx.Service{Models: resources, Voices: resources, Credentials: resources, ProviderTenants: resources, Builder: provider})
			agent := newVoiceFixtureAgent(t, kind, spec[kind], service, apitypes.WorkspaceInputModePushToTalk)
			defer agent.(io.Closer).Close()
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 128)
			output, err := agent.Transform(ctx, input.Stream())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			transport := &voiceFixtureStream{input: input, output: output}
			driver := &voiceFixtureDriver{driver: newDriver(false, nil), stream: transport}
			doc, err := giztest.LoadDocument(filepath.Join(root, "segments.giztest.yaml"), driver)
			if err != nil {
				t.Fatal(err)
			}
			var expected [][]byte
			for _, voice := range []string{"story.default", "story.fox", "story.bird", "story.fox", "story.default"} {
				expected = append(expected, packets[voice]...)
			}
			variable := doc.Variables["digest"]
			variable.Value = voicePacketDigest(expected)
			doc.Variables["digest"] = variable
			report := giztest.Run(ctx, []*giztest.Document{doc}, giztest.Options{Driver: driver, Parallel: 1, Out: io.Discard})
			encoded, _ := json.Marshal(report)
			if len(report.Tasks) != 1 || report.Tasks[0].Status != "passed" {
				t.Fatalf("report=%s", encoded)
			}
			if provider.calls.Load() != 5 {
				t.Fatalf("TTS calls=%d", provider.calls.Load())
			}
			t.Logf("five ordered segments; stripped text; single audio stream; underruns=0")
		})
	}
}
