package eino

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/schema"
)

func TestTransformStreamsLifecycle(t *testing.T) {
	t.Parallel()
	transformer, err := New(t.Context(), textConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("hello"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if got := joinedText(chunks); got != "hello" {
		t.Fatalf("text = %q", got)
	}
	var streamID string
	var bos, eos bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if streamID == "" {
			streamID = chunk.Ctrl.StreamID
		}
		if streamID != chunk.Ctrl.StreamID {
			t.Fatalf("StreamID changed from %q to %q", streamID, chunk.Ctrl.StreamID)
		}
		if chunk.IsBeginOfStream() {
			bos = true
			if mimeType, ok := chunk.MIMEType(); !ok || mimeType != "text/plain" {
				t.Fatalf("BOS MIME = %q, present=%v; chunk=%#v", mimeType, ok, chunk)
			}
		}
		eos = eos || chunk.IsEndOfStream()
		if chunk.Ctrl.Error != "" {
			t.Fatalf("output error = %q", chunk.Ctrl.Error)
		}
	}
	if streamID == "" || !bos || !eos {
		t.Fatalf("lifecycle stream_id=%q BOS=%v EOS=%v", streamID, bos, eos)
	}
}

func TestOutputRouteBeginCarriesDeclaredMIME(t *testing.T) {
	for _, test := range []struct {
		name          string
		mimeType      string
		wantCanonical string
		wantText      bool
	}{
		{name: "text", mimeType: " Text/Plain ", wantCanonical: "text/plain", wantText: true},
		{name: "parameterized text", mimeType: "text/plain; charset=utf-8", wantCanonical: "text/plain; charset=utf-8"},
		{name: "declared text subtype", mimeType: "text/markdown", wantCanonical: "text/markdown"},
		{name: "blob", mimeType: "application/octet-stream", wantCanonical: "application/octet-stream"},
	} {
		t.Run(test.name, func(t *testing.T) {
			chunk := newOutputRouteBegin("route", test.mimeType)
			if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "route" || !chunk.IsBeginOfStream() || chunk.IsEndOfStream() {
				t.Fatalf("route BOS = %#v", chunk)
			}
			if mimeType, ok := chunk.MIMEType(); !ok || mimeType != test.wantCanonical {
				t.Fatalf("route BOS MIME = %q, present=%v, want %q", mimeType, ok, test.wantCanonical)
			}
			_, isText := chunk.Part.(genx.Text)
			if isText != test.wantText {
				t.Fatalf("route BOS part = %T, want text=%v", chunk.Part, test.wantText)
			}
		})
	}
}

func TestTransformerSupportsConcurrentCalls(t *testing.T) {
	t.Parallel()
	transformer, err := New(t.Context(), textConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	const count = 24
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for range count {
		wait.Go(func() {
			text := genx.NewStreamID()
			output, transformErr := transformer.Transform(t.Context(), textInput(text))
			if transformErr != nil {
				failures <- transformErr
				return
			}
			if got := joinedText(drain(t, output)); got != text {
				failures <- errors.New("cross-run output")
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
}

func TestInitiativeClaimIsAtomicAcrossConcurrentStreams(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Initiative = InitiativeOnReload
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	const count = 32
	var claimed atomic.Int32
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for range count {
		wait.Go(func() {
			ok, claimErr := transformer.claimInitiative(t.Context())
			if claimErr != nil {
				failures <- claimErr
				return
			}
			if ok {
				claimed.Add(1)
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
	if got := claimed.Load(); got != 1 {
		t.Fatalf("initiative claims = %d, want 1", got)
	}
}

func TestInitiativeOnceWhenEmptySkipsExistingHistory(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Initiative = InitiativeOnceWhenEmpty
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	transformer.history.live = []*schema.Message{{Role: schema.User, Content: "existing turn"}}
	claimed, err := transformer.claimInitiative(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("once_when_empty claimed initiative with existing history")
	}
	claimed, err = transformer.claimInitiative(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("once_when_empty retried after existing-history decision")
	}
}

func TestFailedInitiativeCanBeClaimedAgain(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Initiative = InitiativeOnReload
	config.Graph.Nodes[0].Transform = nil
	config.Graph.Nodes[0].Script = &ScriptNode{
		Language: ScriptStarlark,
		Source:   "def run(input):\n  fail(\"initiative failed\")\n",
		Limits: ScriptLimits{
			MaxExecutionSteps: 1_000,
			Timeout:           time.Second,
			MaxInputBytes:     1 << 10,
			MaxOutputBytes:    1 << 10,
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	output, err := transformer.Transform(t.Context(), textInput(""))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, output)

	claimed, err := transformer.claimInitiative(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("failed initiative remained permanently claimed")
	}
}

func TestPeerBOSInterruptsInitiativeWithoutMixingTurns(t *testing.T) {
	t.Parallel()
	chat := newBlockingChatModel()
	config := chatConfig(&componentMapResolver{chat: chat})
	config.Initiative = InitiativeOnReload
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	output, err := transformer.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	var initiativeStreamID string
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("read initiative: %v", nextErr)
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "first" {
			initiativeStreamID = chunk.Ctrl.StreamID
			break
		}
	}
	addTextTurn(t, input, "peer")
	select {
	case <-chat.cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("Peer BOS did not interrupt the initiative")
	}
	close(chat.release)
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	chunks := drain(t, output)
	var sawInterrupted, sawPeer bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.StreamID == initiativeStreamID && chunk.IsEndOfStream() &&
			chunk.Ctrl.Error == "interrupted" {
			sawInterrupted = true
		}
		if chunk.Ctrl.StreamID != initiativeStreamID {
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				sawPeer = true
			}
		}
	}
	if !sawInterrupted || !sawPeer {
		t.Fatalf("interrupted=%v peer_output=%v chunks=%#v", sawInterrupted, sawPeer, chunks)
	}
}

func textConfig() Config {
	return Config{
		Agent:  AgentConfig{ID: "assistant", Name: "Assistant"},
		Limits: Limits{MaxOutputBytes: 1 << 20},
		Graph: GraphDefinition{
			Name: "text",
			State: StateDefinition{Fields: []StateField{{
				Name: "answer", Type: StateString, Merge: MergeReplace,
			}}},
			Nodes: []NodeDefinition{{
				ID: "answer",
				Inputs: map[string]Binding{
					"input": {From: "input.text"},
				},
				Outputs: map[string]string{"text": "answer"},
				Transform: &TransformNode{
					Operation: TransformConcatText, Order: []string{"input"},
				},
			}},
			Edges: []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}},
			Outputs: []OutputDefinition{{
				Node: "answer", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true,
			}},
		},
	}
}

func newInputBuilder() *genx.StreamBuilder {
	return genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
}

func textInput(text string) genx.Stream {
	builder := newInputBuilder()
	_ = builder.Add(
		genx.NewBeginOfStream(genx.NewStreamID()),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text)},
		genx.NewTextEndOfStream(),
	)
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}

func drain(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			return chunks
		}
		if err != nil {
			t.Fatalf("drain Stream: %v", err)
		}
		chunks = append(chunks, chunk)
	}
}

func joinedText(chunks []*genx.MessageChunk) string {
	var result strings.Builder
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
			result.WriteString(string(text))
		}
	}
	return result.String()
}
