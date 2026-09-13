package giztestcmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	einoagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// The real Giztest runner, CLI peer_stream receiver, Eino Factory, Starlark
// selector, and AudioDock run unchanged. Only resources/provider and the Peer
// transport are in-memory; no server, credentials, network or LLM is needed.
func TestEinoMultiVoiceGiztest(t *testing.T) {
	for _, fault := range []string{"", "wrong-voice", "stall", "overlap"} {
		name := fault
		if name == "" {
			name = "success"
		}
		t.Run(name, func(t *testing.T) {
			root := "../../../../tests/gizclaw-e2e/testdata/eino-voices"
			data, err := os.ReadFile(filepath.Join(root, "workflow.json"))
			if err != nil {
				t.Fatal(err)
			}
			var public apitypes.EinoWorkflowSpec
			if err := json.Unmarshal(data, &public); err != nil {
				t.Fatal(err)
			}
			packets := map[string][][]byte{}
			for i, voice := range []string{"story.fox", "story.bird", "story.default"} {
				packets[voice] = voiceTonePackets(t, 300+200*i)
			}
			provider := &voiceFixtureProvider{packets: packets, fault: fault}
			resources := voiceFixtureResources{}
			service := peergenx.New(peergenx.Service{Voices: resources, Credentials: resources, ProviderTenants: resources, Builder: provider})
			agent, err := (einoagent.Factory{GenX: service}).NewAgent(t.Context(), agenthost.Spec{Workspace: apitypes.Workspace{Id: "voice-fixture"}, Workflow: apitypes.Workflow{Id: "voices", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &public}}})
			if err != nil {
				t.Fatal(err)
			}
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
			testDriver := &voiceFixtureDriver{driver: newDriver(false, nil), stream: transport}
			doc, err := giztest.LoadDocument(filepath.Join(root, "multi-turn.giztest.yaml"), testDriver)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"fox", "bird", "default"} {
				h := sha256.New()
				for _, p := range packets["story."+name] {
					_, _ = h.Write(p)
				}
				spec := doc.Variables[name+"_digest"]
				spec.Value = hex.EncodeToString(h.Sum(nil))
				doc.Variables[name+"_digest"] = spec
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
			if provider.calls.Load() != 4 || provider.peak.Load() != 1 {
				t.Fatalf("TTS calls=%d peak=%d", provider.calls.Load(), provider.peak.Load())
			}
			if transport.integrity.violations != 0 || transport.integrity.maxActive != 1 || len(transport.integrity.active) != 0 || transport.integrity.streams != 4 {
				t.Fatalf("cross-turn audio=%v", transport.integrity.summary())
			}
			t.Logf("four turns; three distinct voices including default; one Agent invocation; TTS calls=%d peak=%d", provider.calls.Load(), provider.peak.Load())
		})
	}
}

func voiceTonePackets(t *testing.T, frequency int) [][]byte {
	t.Helper()
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	packets := make([][]byte, 40)
	for i := range packets {
		frame := make([]int16, 320)
		for j := range frame {
			frame[j] = int16(12000 * math.Sin(2*math.Pi*float64(frequency)*float64(i*320+j)/16000))
		}
		packets[i], err = encoder.Encode(frame, len(frame))
		if err != nil {
			t.Fatal(err)
		}
	}
	return packets
}

// The fixture session keeps the production receiver's reader as well as the
// Agent invocation alive, so no competing reader can steal a later turn's BOS.
type voiceFixtureDriver struct {
	*driver
	stream *voiceFixtureStream
}

func (d *voiceFixtureDriver) Open(context.Context, *giztest.Document, *giztest.Variables) (giztest.Session, error) {
	session := newPeerStreamSession("peer", d.stream)
	session.startReader()
	return &voiceFixtureSession{session: session}, nil
}

type voiceFixtureSession struct{ session *peerStreamSession }

func (*voiceFixtureSession) Fingerprints() map[string]string { return nil }
func (s *voiceFixtureSession) CloseStreams() error           { return s.session.Close() }
func (*voiceFixtureSession) Close()                          {}
func (s *voiceFixtureSession) Execute(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	input, err := req.Vars.Resolve(req.Step.PeerStream.Input)
	if err != nil {
		return giztest.StepResult{}, err
	}
	result, err := invokePeerStreamOnStream(ctx, nil, nil, s.session.stream, s.session, "", req.Step, input, 0, nil)
	return result.stepResult(), err
}

type voiceFixtureStream struct {
	input     *genx.StreamBuilder
	output    genx.Stream
	integrity peerAudioIntegrity
}

func (s *voiceFixtureStream) Push(_ context.Context, c *genx.MessageChunk) error {
	return s.input.Add(c)
}
func (s *voiceFixtureStream) Next() (*genx.MessageChunk, error) {
	c, err := s.output.Next()
	if err == nil {
		s.integrity.observe(c)
	}
	return c, err
}
func (*voiceFixtureStream) Close() error { return nil }

type voiceFixtureProvider struct {
	packets             map[string][][]byte
	fault               string
	calls, active, peak atomic.Int32
}

func (*voiceFixtureProvider) BuildGenerator(context.Context, peergenx.GeneratorConfig) (genx.Generator, error) {
	return nil, errors.New("unexpected generator")
}
func (p *voiceFixtureProvider) BuildTransformer(_ context.Context, c peergenx.TransformerConfig) (genx.Transformer, error) {
	if c.Voice == nil {
		return nil, errors.New("expected voice")
	}
	voice := c.Voice.Id
	if p.fault == "wrong-voice" {
		voice = "story.default"
	}
	packets, ok := p.packets[voice]
	if !ok {
		return nil, fmt.Errorf("unexpected voice %q", voice)
	}
	return voiceFixtureTTS{provider: p, packets: packets}, nil
}

type voiceFixtureTTS struct {
	provider *voiceFixtureProvider
	packets  [][]byte
}

func (f voiceFixtureTTS) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	f.provider.calls.Add(1)
	active := f.provider.active.Add(1)
	for old := f.provider.peak.Load(); active > old; old = f.provider.peak.Load() {
		if f.provider.peak.CompareAndSwap(old, active) {
			break
		}
	}
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 128)
	go func() {
		defer f.provider.active.Add(-1)
		defer input.Close()
		for {
			_, err := input.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = output.Stream().CloseWithError(err)
				return
			}
		}
		id := genx.NewStreamID()
		otherID := genx.NewStreamID()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for i, packet := range f.packets {
			if f.provider.fault == "stall" && i == 30 {
				timer := time.NewTimer(900 * time.Millisecond)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					_ = output.Stream().CloseWithError(ctx.Err())
					return
				}
			}
			select {
			case <-ctx.Done():
				_ = output.Stream().CloseWithError(ctx.Err())
				return
			case <-ticker.C:
			}
			if f.provider.fault != "overlap" || i != 5 {
				if err := output.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: i == 0}}); err != nil {
					return
				}
			}
			if f.provider.fault == "overlap" && i == 5 {
				_ = output.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: f.provider.packets["story.bird"][i]}, Ctrl: &genx.StreamCtrl{StreamID: otherID, BeginOfStream: true}})
			}
			if f.provider.fault == "overlap" && i == 8 {
				_ = output.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: otherID, EndOfStream: true}})
			}
		}
		_ = output.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}})
		_ = output.Done(genx.Usage{})
	}()
	return output.Stream(), nil
}

type voiceFixtureResources struct{}

func (voiceFixtureResources) GetVoice(_ context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	return adminhttp.GetVoice200JSONResponse(apitypes.Voice{
		Id: request.Id,
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKindVolcTenant,
			Id:   "volc-main",
		},
	}), nil
}

func (voiceFixtureResources) GetCredential(_ context.Context, request adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	return adminhttp.GetCredential200JSONResponse(apitypes.Credential{Id: request.Id}), nil
}

func (voiceFixtureResources) GetVolcTenant(_ context.Context, request adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
	return adminhttp.GetVolcTenant200JSONResponse(apitypes.VolcTenant{Id: request.Id, CredentialId: "voice-credential"}), nil
}

func (voiceFixtureResources) GetDeepSeekTenant(context.Context, adminhttp.GetDeepSeekTenantRequestObject) (adminhttp.GetDeepSeekTenantResponseObject, error) {
	return nil, errors.New("unexpected DeepSeek tenant lookup")
}

func (voiceFixtureResources) GetOpenAITenant(context.Context, adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	return nil, errors.New("unexpected OpenAI tenant lookup")
}

func (voiceFixtureResources) GetGeminiTenant(context.Context, adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	return nil, errors.New("unexpected Gemini tenant lookup")
}

func (voiceFixtureResources) GetDashScopeTenant(context.Context, adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	return nil, errors.New("unexpected DashScope tenant lookup")
}

func (voiceFixtureResources) GetMiniMaxTenant(context.Context, adminhttp.GetMiniMaxTenantRequestObject) (adminhttp.GetMiniMaxTenantResponseObject, error) {
	return nil, errors.New("unexpected MiniMax tenant lookup")
}
