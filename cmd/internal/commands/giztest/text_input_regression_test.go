package giztestcmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gorilla/websocket"
)

// The provider is local, but the SDK websocket, Transformer, RealtimeStream,
// Giztest YAML loader and production peer_stream operation are real.
func TestDeviceTextInputGiztest(t *testing.T) {
	for _, workflow := range []string{"doubao-ptt", "doubao-realtime", "doubao-external-tts", "eino-story", "flowcraft-adventure"} {
		for _, timestamp := range []string{"zero", "unix_ms"} {
			for _, afterAudio := range []bool{false, true} {
				if afterAudio && (workflow == "eino-story" || workflow == "flowcraft-adventure") {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/after_audio=%t", workflow, timestamp, afterAudio), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
					defer cancel()
					transformer := textRegressionTransformer(t, ctx, workflow)
					input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
					defer input.Close()
					output, err := transformer.Transform(ctx, input)
					if err != nil {
						t.Fatal(err)
					}
					stream := &textRegressionStream{input: input, Stream: output, verifyRoute: workflow == "doubao-ptt" || workflow == "doubao-realtime"}
					defer stream.Close()
					doc, err := giztest.LoadDocument(filepath.Join("..", "..", "..", "..", "tests", "gizclaw-e2e", "testdata", "text-input", timestamp+".giztest.yaml"), newDriver(false, nil))
					if err != nil {
						t.Fatal(err)
					}
					session := newPeerStreamSession("peer", stream)
					defer session.Close()
					session.startReader()
					if afterAudio {
						mode := "push-to-talk"
						if workflow == "doubao-realtime" {
							mode = "realtime"
						}
						noAudio := false
						audio, _ := testOggOpus(t)
						audioStep := giztest.Step{ID: "audio_first", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: mode, Label: "demo-home", RequireAudio: &noAudio, Pacing: "0ms", Completion: "first_response", FirstTextTimeout: "2s"}}
						if _, err := invokePeerStreamOnStream(ctx, nil, nil, stream, session, "voice-1", audioStep, audio, 0, nil); err != nil {
							t.Fatalf("audio baseline: %v", err)
						}
					}
					step := doc.Steps[0]
					if workflow == "doubao-external-tts" {
						requireAudio := true
						step.PeerStream.RequireAudio = &requireAudio
						step.PeerStream.FirstAudioTimeout = "2s"
					}
					result, err := invokePeerStreamOnStream(ctx, nil, nil, stream, session, "demo-1", step, step.PeerStream.Input, 0, nil)
					if err != nil {
						t.Fatalf("giztest text reply: %v; evidence=%v", err, result.evidence)
					}
				})
			}
		}
	}
}

type textRegressionStream struct {
	genx.Stream
	input       *genx.RealtimeStream
	verifyRoute bool
	expected    atomic.Value
}

func (s *textRegressionStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if chunk.IsBeginOfStream() {
		s.expected.Store(chunk.Ctrl.StreamID)
	}
	return s.input.Push(ctx, chunk)
}

func (s *textRegressionStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if err != nil || chunk == nil || !s.verifyRoute {
		return chunk, err
	}
	if text, ok := chunk.Part.(genx.Text); ok && len(text) > 0 && chunk.Role == genx.RoleModel {
		expected, _ := s.expected.Load().(string)
		if chunk.Ctrl == nil || !streamIDMatches(chunk.Ctrl.SourceStreamID, expected) {
			return nil, fmt.Errorf("assistant reply does not own input %q", expected)
		}
	}
	return chunk, nil
}

// Dialogue event frames contain an event ID, length-prefixed session/connect
// ID, and length-prefixed JSON. This fixture never synthesizes ASREnded for text.
func textRegressionProvider(t *testing.T) *httptest.Server {
	t.Helper()
	listener := &textPipeListener{connections: make(chan net.Conn), done: make(chan struct{})}
	previous := websocket.DefaultDialer
	dialer := *previous
	dialer.Proxy = nil
	dialer.NetDialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		select {
		case listener.connections <- server:
			return client, nil
		case <-ctx.Done():
			client.Close()
			server.Close()
			return nil, ctx.Err()
		}
	}
	websocket.DefaultDialer = &dialer
	t.Cleanup(func() { websocket.DefaultDialer = previous })
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		sessionID := ""
		realtime := false
		audioReplied := false
		turn := 0
		send := func(event uint32, payload string) error {
			data := []byte{0x11, 0x94, 0x10, 0}
			data = binary.BigEndian.AppendUint32(data, event)
			id := sessionID
			if event == 50 {
				id = "connection"
			}
			data = binary.BigEndian.AppendUint32(data, uint32(len(id)))
			data = append(data, id...)
			data = binary.BigEndian.AppendUint32(data, uint32(len(payload)))
			data = append(data, payload...)
			return conn.WriteMessage(websocket.BinaryMessage, data)
		}
		reply := func() error {
			turn++
			payload := fmt.Sprintf(`{"text":"local assistant reply","question_id":"question-%d","reply_id":"reply-%d"}`, turn, turn)
			for _, event := range []uint32{350, 550, 359, 559} {
				if err := send(event, payload); err != nil {
					return err
				}
			}
			return nil
		}
		audioReply := func() error {
			if err := send(451, fmt.Sprintf(`{"text":"spoken question","question_id":"question-%d"}`, turn+1)); err != nil {
				return err
			}
			if err := send(459, fmt.Sprintf(`{"question_id":"question-%d"}`, turn+1)); err != nil {
				return err
			}
			return reply()
		}
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if len(data) < 8 {
				t.Error("short provider frame")
				return
			}
			event := binary.BigEndian.Uint32(data[4:8])
			switch event {
			case 1:
				err = send(50, `{}`)
			case 100:
				if len(data) < 12 {
					t.Error("short session frame")
					return
				}
				n := int(binary.BigEndian.Uint32(data[8:12]))
				if n > len(data)-12 {
					t.Error("short session ID")
					return
				}
				realtime = strings.Contains(string(data[12+n:]), "keep_alive")
				sessionID = string(data[12 : 12+n])
				err = send(150, `{"dialog_id":"local-dialog"}`)
			case 501:
				err = reply()
			case 200:
				if realtime && !audioReplied {
					audioReplied = true
					err = audioReply()
				}
			case 400:
				err = audioReply()
			case 515:
			case 102, 2:
				return
			default:
				t.Error(fmt.Sprintf("unexpected provider input event %d", event))
				return
			}
			if err != nil {
				return
			}
		}
	})}}
	server.Start()
	return server
}

// An in-memory listener exercises HTTP/WebSocket without host port permissions.
type textPipeListener struct {
	connections chan net.Conn
	done        chan struct{}
	once        sync.Once
}

func (l *textPipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.connections:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}
func (l *textPipeListener) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *textPipeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80} }

func textRegressionTransformer(t *testing.T, ctx context.Context, workflow string) genx.Transformer {
	t.Helper()
	if workflow == "eino-story" {
		tr, err := eino.New(ctx, eino.Config{
			Agent: eino.AgentConfig{ID: "story", Name: "Story"}, Components: textStoryProvider{},
			Graph: eino.GraphDefinition{Name: "story",
				State: eino.StateDefinition{Fields: []eino.StateField{{Name: "messages", Type: eino.StateMessages, Merge: eino.MergeReplace}, {Name: "answer", Type: eino.StateString, Merge: eino.MergeReplace}}},
				Nodes: []eino.NodeDefinition{
					{ID: "prompt", Inputs: map[string]eino.Binding{"text": {From: "input.text"}}, Outputs: map[string]string{"messages": "messages"}, Prompt: &eino.PromptNode{Format: eino.PromptFString, Messages: []eino.PromptMessage{{Role: eino.PromptSystem, Template: "你是小剧场的旁白，根据用户选择继续故事。"}, {Role: eino.PromptUser, Template: "{text}"}}}},
					{ID: "model", Inputs: map[string]eino.Binding{"messages": {From: "messages"}}, Outputs: map[string]string{"text": "answer"}, ChatModel: &eino.ChatModelNode{Model: "story"}},
				},
				Edges:   []eino.EdgeDefinition{{From: "start", To: "prompt"}, {From: "prompt", To: "model"}, {From: "model", To: "end"}},
				Outputs: []eino.OutputDefinition{{Node: "model", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return tr
	}
	if workflow == "flowcraft-adventure" {
		tr, err := flowcraft.New(flowcraft.Config{ID: "adventure", Name: "Adventure", Models: textRegressionGenerator{},
			Graph: flowgraph.GraphDefinition{Name: "adventure", Entry: "narrator", Nodes: []flowgraph.NodeDefinition{{ID: "narrator", Type: "llm", Config: map[string]any{"model": "chat"}}}}, PublishNodes: []string{"narrator"},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tr.Close() })
		return tr
	}
	provider := textRegressionProvider(t)
	t.Cleanup(provider.Close)
	client := doubaospeech.NewClient("test-app", doubaospeech.WithAPIKey("test-key"), doubaospeech.WithWebSocketURL("ws"+strings.TrimPrefix(provider.URL, "http")))
	mode := doubaorealtime.ModePushToTalk
	if workflow == "doubao-realtime" {
		mode = doubaorealtime.ModeRealtime
	}
	output := doubaorealtime.OutputAudio
	if workflow == "doubao-external-tts" {
		output = doubaorealtime.OutputText
	}
	tr, err := doubaorealtime.New(doubaorealtime.Config{Client: client, Model: "SC20", Mode: mode, Output: output})
	if err != nil {
		t.Fatal(err)
	}
	if workflow != "doubao-external-tts" {
		return tr
	}
	dock, err := audiodock.New(audiodock.Config{Agent: tr, TTS: textRegressionTTS{audio: testAudibleOpus(t)}, ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) { return "voice/local", nil }})
	if err != nil {
		t.Fatal(err)
	}
	return dock
}

type textRegressionGenerator struct{}

func (textRegressionGenerator) GenerateStream(_ context.Context, _ string, mc genx.ModelContext) (genx.Stream, error) {
	b := genx.NewStreamBuilder(mc, 8)
	if err := b.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("local adventure reply")}); err != nil {
		return nil, err
	}
	if err := b.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return b.Stream(), nil
}
func (textRegressionGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("unexpected tool invocation")
}

type textRegressionTTS struct{ audio []byte }

func (tts textRegressionTTS) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	output := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	go func() {
		defer output.Close()
		for {
			chunk, err := input.Next()
			if err != nil {
				return
			}
			if chunk == nil {
				continue
			}
			if text, ok := chunk.Part.(genx.Text); ok && len(text) > 0 {
				ctrl := *chunk.Ctrl
				ctrl.BeginOfStream = true
				ctrl.EndOfStream = true
				_ = output.Push(ctx, &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: tts.audio}, Ctrl: &ctrl})
			}
		}
	}()
	return output, nil
}

// Live files are opt-in and use the normal E2E seeded workflow catalog.
func TestDeviceTextInputLive(t *testing.T) {
	if os.Getenv("GIZCLAW_TEXT_INPUT_LIVE") != "1" {
		t.Skip("set GIZCLAW_TEXT_INPUT_LIVE=1 with the E2E endpoint and registration token")
	}
	for _, name := range []string{"GIZCLAW_TEST_ENDPOINT", "GIZCLAW_TEST_REGISTRATION_TOKEN"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			t.Fatalf("missing %s", name)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()
	command := NewCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"run", "--parallel", "1", filepath.Join("..", "..", "..", "..", "tests", "gizclaw-e2e", "testdata", "text-input", "live")})
	if err := command.ExecuteContext(ctx); err != nil {
		t.Fatalf("live Giztest: %v\n%s", err, output.String())
	}
	t.Log(output.String())
}

func TestDeviceTextInputDocuments(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "tests", "gizclaw-e2e", "testdata", "text-input", "live", "*.giztest.yaml"))
	if err != nil || len(files) != 10 {
		t.Fatalf("live fixtures: %d, %v", len(files), err)
	}
	for _, file := range files {
		if _, err := giztest.LoadDocument(file, newDriver(false, nil)); err != nil {
			t.Errorf("%s: %v", filepath.Base(file), err)
		}
	}
}

// The typed Eino prompt and ChatModel node run normally; only the external
// model call is replaced, and it rejects a missing or partial user message.
type textStoryProvider struct{}

func (textStoryProvider) ResolveChatModel(context.Context, string) (einomodel.BaseChatModel, error) {
	return textStoryProvider{}, nil
}
func (textStoryProvider) ResolveRetriever(context.Context, string) (retriever.Retriever, error) {
	return nil, errors.New("unexpected retriever")
}
func (textStoryProvider) Generate(_ context.Context, input []*schema.Message, _ ...einomodel.Option) (*schema.Message, error) {
	if len(input) != 2 || input[1].Role != schema.User || input[1].Content != "请回答这条文字消息" {
		return nil, errors.New("story model did not receive the complete user input")
	}
	return schema.AssistantMessage("小剧场的旁白回应了你的选择。", nil), nil
}
func (p textStoryProvider) Stream(ctx context.Context, input []*schema.Message, options ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := p.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}
