package giztestcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// The provider fixtures are local; Factory, Eino, AudioDock, per-task response
// history and the Giztest operations are the same code used by real clients.
type conversationMemory struct{}

func (conversationMemory) SupportsDirectFactObservation() bool { return true }
func (conversationMemory) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}
func (conversationMemory) Recall(context.Context, memory.Query) (memory.RecallResult, error) {
	return memory.RecallResult{}, nil
}
func (conversationMemory) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, memory.ErrUnsupported
}
func (conversationMemory) Delete(context.Context, memory.DeleteRequest) error {
	return memory.ErrUnsupported
}

type conversationProviders struct {
	tail   chan struct{}
	packet []byte
}

func (p *conversationProviders) BuildGenerator(context.Context, peergenx.GeneratorConfig) (genx.Generator, error) {
	return conversationGenerator{p}, nil
}
func (p *conversationProviders) BuildTransformer(_ context.Context, c peergenx.TransformerConfig) (genx.Transformer, error) {
	if c.Model != nil && c.Model.Kind == apitypes.ModelKindAsr {
		return conversationASR{realtime: c.Params["emit_interim"] == true || c.Params["emit_interim"] == "true"}, nil
	}
	return conversationTTS{p.packet}, nil
}

type conversationGenerator struct{ p *conversationProviders }

func (g conversationGenerator) GenerateStream(ctx context.Context, _ string, mc genx.ModelContext) (genx.Stream, error) {
	text := textRegressionUserText(mc)
	b := genx.NewGrowableStreamBuilder(mc, 16)
	if text != "开始" {
		_ = b.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("reply " + text + "。")})
		_ = b.Done(genx.Usage{})
		return b.Stream(), nil
	}
	_ = b.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("opening start。")})
	go func() {
		select {
		case <-g.p.tail:
		case <-ctx.Done():
			_ = b.Stream().CloseWithError(ctx.Err())
			return
		}
		_ = b.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("earlier opening tail。")})
		_ = b.Done(genx.Usage{})
	}()
	return b.Stream(), nil
}
func (conversationGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("local fixture has no tools")
}

type conversationTTS struct{ packet []byte }

func (p conversationTTS) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	b := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	id := genx.NewStreamID()
	go func() {
		defer input.Close()
		first := true
		for {
			c, e := input.Next()
			if e != nil {
				if errors.Is(e, io.EOF) {
					_ = b.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}})
					_ = b.Done(genx.Usage{})
				} else {
					_ = b.Stream().CloseWithError(e)
				}
				return
			}
			if c == nil {
				continue
			}
			if text, ok := c.Part.(genx.Text); ok && strings.TrimSpace(string(text)) != "" {
				e = b.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: p.packet}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: first}})
				first = false
				if e != nil {
					return
				}
			}
			if ctx.Err() != nil {
				_ = b.Stream().CloseWithError(ctx.Err())
				return
			}
		}
	}()
	return b.Stream(), nil
}

type conversationASR struct{ realtime bool }

func (a conversationASR) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	b := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	go func() {
		defer stop()
		defer b.Done(genx.Usage{})
		id := ""
		n := 0
		hasAudio, emitted := false, false
		for {
			c, e := input.Next()
			if e != nil {
				return
			}
			if c == nil {
				continue
			}
			if c.IsBeginOfStream() {
				id = c.Ctrl.StreamID
				n++
				hasAudio, emitted = false, false
			}
			if a, ok := c.Part.(*genx.Blob); ok && len(a.Data) > 0 {
				hasAudio = true
			}
			if hasAudio && !emitted && (a.realtime || c.IsEndOfStream()) {
				_ = b.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(fmt.Sprintf("speech-%d", n)), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "transcript", BeginOfStream: true, EndOfStream: true}})
				emitted = true
			}
		}
	}()
	return b.Stream(), nil
}

// The broker models the SDK's connection-owned producer and local subscriptions.
// Subscription Close cannot cancel the Agent; a new input BOS can interrupt it.
type conversationBroker struct {
	inputIDs       []string
	ownershipError string

	openingID           string
	releasing           bool
	releaseOnce         sync.Once
	ctx                 context.Context
	input               *genx.StreamBuilder
	output              genx.Stream
	providers           *conversationProviders
	mu                  sync.Mutex
	subs                map[*conversationSub]struct{}
	opens               int
	tailText, tailAudio bool
	tailReady           chan struct{}
	tailReadyOnce       sync.Once
	trace               []string
	done                chan struct{}
}
type conversationSub struct {
	broker *conversationBroker
	queue  chan *genx.MessageChunk
	done   chan struct{}
	once   sync.Once
}

func (b *conversationBroker) record(s string) {
	b.mu.Lock()
	b.trace = append(b.trace, s)
	b.mu.Unlock()
}
func (b *conversationBroker) pump() {
	defer close(b.done)
	for {
		c, e := b.output.Next()
		if e != nil {
			return
		}
		if c == nil {
			continue
		}
		owner, id := "", ""
		if c.Ctrl != nil {
			owner = c.Ctrl.ResponseEpoch.InputStreamID()
			id = c.Ctrl.StreamID
		}
		kind := "control"
		payload := false
		if text, ok := c.Part.(genx.Text); ok {
			kind = "text"
			payload = strings.TrimSpace(string(text)) != ""
		}
		if a, ok := c.Part.(*genx.Blob); ok {
			kind = "audio"
			payload = len(a.Data) > 0
		}
		b.mu.Lock()
		b.trace = append(b.trace, fmt.Sprintf("output owner=%s id=%s kind=%s payload=%t eos=%t", owner, id, kind, payload, c.IsEndOfStream()))
		if text, ok := c.Part.(genx.Text); ok && c.Role == genx.RoleModel {
			for turn := 1; turn <= 2; turn++ {
				if strings.Contains(string(text), fmt.Sprintf("reply speech-%d", turn)) && len(b.inputIDs) > turn && owner != b.inputIDs[turn] {
					b.ownershipError = fmt.Sprintf("turn %d output belongs to %q, want %q", turn, owner, b.inputIDs[turn])
				}
			}
		}

		subs := make([]*conversationSub, 0, len(b.subs))
		for s := range b.subs {
			subs = append(subs, s)
		}
		if b.releasing && owner == b.openingID {
			if text, ok := c.Part.(genx.Text); ok && strings.Contains(string(text), "earlier opening tail") {
				b.tailText = true
			}
			if kind == "audio" && payload {
				b.tailAudio = true
			}
		}
		ready := b.tailText && b.tailAudio
		b.mu.Unlock()
		for _, s := range subs {
			select {
			case s.queue <- c.Clone():
			case <-s.done:
			case <-b.ctx.Done():
				return
			}
		}
		if ready {
			b.tailReadyOnce.Do(func() { close(b.tailReady) })
		}
	}
}
func (b *conversationBroker) open() (peerStream, error) {
	s := &conversationSub{broker: b, queue: make(chan *genx.MessageChunk, 64), done: make(chan struct{})}
	b.mu.Lock()
	b.opens++
	n := b.opens
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	b.record(fmt.Sprintf("subscription open %d", n))

	return s, nil
}
func (s *conversationSub) Push(ctx context.Context, c *genx.MessageChunk) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if c != nil && c.Ctrl != nil && c.IsBeginOfStream() {
		b := s.broker
		b.mu.Lock()
		if b.openingID == "" {
			b.openingID = c.Ctrl.StreamID
		}
		if len(b.inputIDs) == 0 || b.inputIDs[len(b.inputIDs)-1] != c.Ctrl.StreamID {
			b.inputIDs = append(b.inputIDs, c.Ctrl.StreamID)
		}

		replacement := c.Ctrl.StreamID != b.openingID
		b.mu.Unlock()
		if replacement {
			b.releaseOnce.Do(func() {
				b.mu.Lock()
				b.releasing = true
				b.mu.Unlock()
				close(b.providers.tail)
			})
			select {
			case <-b.tailReady:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	if c != nil && c.Ctrl != nil && (c.IsBeginOfStream() || c.IsEndOfStream()) {
		s.broker.record(fmt.Sprintf("input id=%s bos=%t eos=%t", c.Ctrl.StreamID, c.IsBeginOfStream(), c.IsEndOfStream()))
	}
	return s.broker.input.Add(c)
}

func (s *conversationSub) Next() (*genx.MessageChunk, error) {
	select {
	case c := <-s.queue:
		return c, nil
	case <-s.done:
		return nil, io.EOF
	case <-s.broker.ctx.Done():
		return nil, s.broker.ctx.Err()
	}
}
func (s *conversationSub) Close() error {
	s.once.Do(func() {
		s.broker.mu.Lock()
		delete(s.broker.subs, s)
		s.broker.mu.Unlock()
		close(s.done)
		s.broker.record("subscription closed locally")
	})
	return nil
}

// The memory nodes keep this graph dependent on final ASR text, as a
// conversational graph with recall and asynchronous observation would be.
const conversationWorkflow = `{
 "voice_adapter":{"default_voice":"fixture.voice"},
 "conversation":{"starts":"peer"},
 "graph":{
  "name":"consecutive-turns",
  "state":{"fields":[
   {"name":"observation","type":"string","merge":"replace"},
   {"name":"recalled","type":"string","merge":"replace"},
   {"name":"messages","type":"messages","merge":"replace"},
   {"name":"answer","type":"string","merge":"replace"}
  ]},
  "nodes":[
   {"id":"capture","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"observation"}},
   {"id":"recall","type":"memory_recall","query_from":"input.text","output":"recalled","top_k":5},
   {"id":"prompt","type":"prompt","format":"f_string","inputs":{"history":{"from":"input.messages"}},"outputs":{"messages":"messages"},"messages":[{"placeholder":"history"}]},
   {"id":"model","type":"chat_model","model":"fixture-model","inputs":{"messages":{"from":"messages"}},"outputs":{"text":"answer"}},
   {"id":"observe","type":"memory_observe","facts":[{"text_from":"observation"},{"text_from":"answer"}],"wait_for_completion":false}
  ],
  "edges":[{"from":"start","to":"capture"},{"from":"capture","to":"recall"},{"from":"recall","to":"prompt"},{"from":"prompt","to":"model"},{"from":"model","to":"observe"},{"from":"observe","to":"end"}],
  "outputs":[{"node":"model","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]
 }
}`

func TestPeerStreamConsecutiveTurnsKeepResponseOwnership(t *testing.T) {
	for _, mode := range []apitypes.WorkspaceInputMode{apitypes.WorkspaceInputModePushToTalk, apitypes.WorkspaceInputModeRealtime} {
		for _, retained := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/retained=%t", mode, retained), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				providers := &conversationProviders{tail: make(chan struct{}), packet: testAudibleOpus(t)}
				resources := voiceFixtureResources{}
				service := peergenx.New(peergenx.Service{Models: resources, Voices: resources, Credentials: resources, ProviderTenants: resources, Builder: providers})
				spec := newVoiceFixtureSpec(t, "eino", []byte(conversationWorkflow), mode, "fixture-asr")
				spec.Memory = conversationMemory{}
				agent, err := voiceFixtureFactory(t, "eino", service).NewAgent(ctx, spec)
				if err != nil {
					t.Fatal(err)
				}
				defer agent.(io.Closer).Close()
				input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 128)
				output, err := agent.Transform(ctx, input.Stream())
				if err != nil {
					t.Fatal(err)
				}
				broker := &conversationBroker{ctx: ctx, input: input, output: output, providers: providers, subs: map[*conversationSub]struct{}{}, tailReady: make(chan struct{}), done: make(chan struct{})}
				go broker.pump()
				defer func() {
					cancel()
					_ = output.Close()
					_ = input.Stream().Close()
					select {
					case <-broker.done:
					case <-time.After(time.Second):
						t.Error("output pump did not stop")
					}
					if t.Failed() {
						broker.mu.Lock()
						trace := strings.Join(broker.trace, "\n")
						broker.mu.Unlock()
						t.Log(trace)
					}
				}()
				sessions := newPeerStreamSessions()
				defer sessions.Close()
				opening := giztest.Step{ID: "opening", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Completion: "first_response", KeepOpen: retained, Session: "conversation", FirstTextTimeout: "1s", FirstAudioTimeout: "1s"}}
				if _, err := invokePeerStreamWithSessions(ctx, nil, broker.open, sessions, opening, "开始", 0); err != nil {
					t.Fatal(err)
				}
				audio, _ := testOggOpus(t)
				for turn := 1; turn <= 2; turn++ {
					step := giztest.Step{ID: fmt.Sprintf("turn-%d", turn), Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: string(mode), Completion: "first_response", KeepOpen: retained, Session: "conversation", HoldBeforeAudio: "0ms", Pacing: "1ms", FirstTextTimeout: "1s", FirstAudioTimeout: "1s"}}
					result, err := invokePeerStreamWithSessions(ctx, nil, broker.open, sessions, step, audio, 0)
					if err != nil {
						t.Fatalf("turn %d: %v; %v", turn, err, result.evidence)
					}
					object := result.assertion.(map[string]any)
					want := fmt.Sprintf("reply speech-%d。", turn)
					if object["reply"] != want {
						t.Fatalf("turn %d reply=%q, want %q", turn, object["reply"], want)
					}
					if object["audio_bytes"] != len(providers.packet) {
						t.Fatalf("turn %d audio contains an old response: %v", turn, object["audio_bytes"])
					}
				}

				broker.mu.Lock()
				ownershipError := broker.ownershipError
				broker.mu.Unlock()
				if ownershipError != "" {
					t.Fatal(ownershipError)
				}
				wantOpens := 3
				if retained {
					wantOpens = 1
				}
				if broker.opens != wantOpens {
					t.Fatalf("opened %d streams, want %d", broker.opens, wantOpens)
				}
			})
		}
	}
}
