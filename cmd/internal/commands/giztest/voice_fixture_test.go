package giztestcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	einoagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/eino"
	flowcraftagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func voiceTonePackets(t *testing.T, frequency int) [][]byte {
	return voiceTonePacketsCount(t, frequency, 40)
}
func voiceTonePacketsCount(t *testing.T, frequency, count int) [][]byte {
	t.Helper()
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	packets := make([][]byte, count)
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
	audio  map[string][]byte
}

func (d *voiceFixtureDriver) Open(context.Context, *giztest.Document, *giztest.Variables) (giztest.Session, error) {
	session := newPeerStreamSession("peer", d.stream)
	session.startReader()
	return &voiceFixtureSession{session: session, audio: d.audio}, nil
}

type voiceFixtureSession struct {
	session *peerStreamSession
	audio   map[string][]byte
}

func (*voiceFixtureSession) Fingerprints() map[string]string { return nil }
func (s *voiceFixtureSession) CloseStreams() error           { return s.session.Close() }
func (*voiceFixtureSession) Close()                          {}
func (s *voiceFixtureSession) Execute(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	input, err := req.Vars.Resolve(req.Step.PeerStream.Input)
	if err != nil {
		return giztest.StepResult{}, err
	}
	if s.audio != nil {
		input = s.audio[input.(string)]
	}
	result, err := invokePeerStreamOnStream(ctx, nil, nil, s.session.stream, s.session, "", req.Step, input, 0, nil)
	return result.stepResult(), err
}

type voiceFixtureStream struct {
	input     *genx.StreamBuilder
	output    genx.Stream
	mu        sync.Mutex
	integrity peerAudioIntegrity
}

func (s *voiceFixtureStream) Push(_ context.Context, c *genx.MessageChunk) error {
	return s.input.Add(c)
}
func (s *voiceFixtureStream) Next() (*genx.MessageChunk, error) {
	c, err := s.output.Next()
	if err == nil {
		s.mu.Lock()
		s.integrity.observe(c)
		s.mu.Unlock()
	}
	return c, err
}
func (*voiceFixtureStream) Close() error { return nil }

type voiceFixtureProvider struct {
	turnPackets         []map[string][][]byte
	packets             map[string][][]byte
	fault               string
	delays              []time.Duration
	reply               string
	recognize           map[string]string
	calls, active, peak atomic.Int32
}

func (p *voiceFixtureProvider) BuildGenerator(context.Context, peergenx.GeneratorConfig) (genx.Generator, error) {
	return voiceFixtureGenerator{reply: p.reply}, nil
}
func (p *voiceFixtureProvider) BuildTransformer(_ context.Context, c peergenx.TransformerConfig) (genx.Transformer, error) {
	if c.Model != nil && c.Model.Kind == apitypes.ModelKindAsr {
		return voiceFixtureASR{provider: p, realtime: c.Params["emit_interim"] == true || c.Params["emit_interim"] == "true"}, nil
	}
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
	return voiceFixtureTTS{provider: p, packets: packets, voice: voice}, nil
}

type voiceFixtureTTS struct {
	voice    string
	provider *voiceFixtureProvider
	packets  [][]byte
}

func (f voiceFixtureTTS) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	call := f.provider.calls.Add(1)
	if len(f.provider.turnPackets) > 0 {
		if int(call) > len(f.provider.turnPackets) {
			return nil, errors.New("unexpected extra TTS invocation")
		}
		f.packets = f.provider.turnPackets[call-1][f.voice]
	}
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
		var text strings.Builder
		for {
			chunk, err := input.Next()
			if chunk != nil {
				if part, ok := chunk.Part.(genx.Text); ok {
					text.WriteString(string(part))
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = output.Stream().CloseWithError(err)
				return
			}
		}
		if f.provider.reply != "" && text.Len() < len(f.provider.reply) {
			_ = output.Stream().CloseWithError(errors.New("long reply text was truncated before TTS"))
			return
		}
		id := genx.NewStreamID()
		otherID := genx.NewStreamID()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for i, packet := range f.packets {
			if f.provider.fault == "truncate" && i == len(f.packets)/2 {
				_ = output.Done(genx.Usage{})
				return
			}
			if len(f.provider.delays) > 0 {
				ticker.Reset(f.provider.delays[i%len(f.provider.delays)])
			}
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

func newVoiceFixtureAgent(t *testing.T, kind string, data []byte, service *peergenx.Service, mode apitypes.WorkspaceInputMode) agenthost.Agent {
	t.Helper()
	var parameters apitypes.WorkspaceParameters
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"agent_type":%q,"input":%q}`, kind, mode)), &parameters); err != nil {
		t.Fatal(err)
	}
	spec := agenthost.Spec{Workspace: apitypes.Workspace{Id: "voice-fixture", Name: "voice-fixture", Parameters: &parameters}, Workflow: apitypes.Workflow{Id: "voices"}}
	var agent agenthost.Agent
	var err error
	switch kind {
	case "eino":
		var public apitypes.EinoWorkflowSpec
		if err := json.Unmarshal(data, &public); err != nil {
			t.Fatal(err)
		}
		spec.Workflow.Spec = apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &public}
		agent, err = (einoagent.Factory{GenX: service}).NewAgent(t.Context(), spec)
	case "flowcraft":
		var public apitypes.FlowcraftWorkflowSpec
		if err := json.Unmarshal(data, &public); err != nil {
			t.Fatal(err)
		}
		spec.Workflow.Spec = apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &public}
		agent, err = (flowcraftagent.Factory{GenX: service}).NewAgent(t.Context(), spec)
	default:
		t.Fatalf("unknown workflow %q", kind)
	}
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

// Graph nodes still select the publisher and voice; only language generation is fake.
type voiceFixtureGenerator struct{ reply string }

func (g voiceFixtureGenerator) GenerateStream(_ context.Context, _ string, model genx.ModelContext) (genx.Stream, error) {
	b := genx.NewGrowableStreamBuilder(model, 8)
	if err := b.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(g.text())}); err != nil {
		return nil, err
	}
	if err := b.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return b.Stream(), nil
}
func (voiceFixtureGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("unexpected tool invocation")
}
func (voiceFixtureResources) GetModel(_ context.Context, req adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	kind := apitypes.ModelKindLlm
	if req.Id == "fixture-asr" {
		kind = apitypes.ModelKindAsr
	}
	return adminhttp.GetModel200JSONResponse(apitypes.Model{Id: req.Id, Kind: kind, Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: "volc-main"}}), nil
}

// ASR recognizes a fixture packet, never a requested role outside the input stream.
// Push-to-talk requires EOS; realtime emits its final transcript while input remains open.
type voiceFixtureASR struct {
	provider *voiceFixtureProvider
	realtime bool
}

func (a voiceFixtureASR) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	b := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	go func() {
		defer stop()
		defer input.Close()
		defer b.Done(genx.Usage{})
		var id, text string
		emit := func() error {
			return b.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true, EndOfStream: true}})
		}
		for {
			c, err := input.Next()
			if err != nil {
				return
			}
			if c.IsBeginOfStream() {
				id = c.Ctrl.StreamID
				text = ""
			}
			if blob, ok := c.Part.(*genx.Blob); ok && text == "" {
				text = a.provider.recognize[string(blob.Data)]
				if text != "" && a.realtime {
					if emit() != nil {
						return
					}
				}
			}
			if c.IsEndOfStream() && text != "" && !a.realtime {
				if emit() != nil {
					return
				}
			}
		}
	}()
	return b.Stream(), nil
}

func (g voiceFixtureGenerator) text() string {
	if g.reply != "" {
		return g.reply
	}
	return "A deterministic character reply."
}

func (s *voiceFixtureStream) audioSummary() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integrity.summary()
}
