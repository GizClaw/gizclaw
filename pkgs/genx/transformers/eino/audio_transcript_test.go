package eino

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/cloudwego/eino/schema"
)

func TestAudioTranscriptConfigValidation(t *testing.T) {
	root := audioTranscriptConfig(&componentMapResolver{chat: &fakeChatModel{}})
	if err := ValidateConfig(root); err != nil {
		t.Fatalf("root AudioTranscript: %v", err)
	}
	if !AcceptsAudioInput(root) {
		t.Fatal("AcceptsAudioInput() = false for a root AudioTranscript node")
	}
	if AcceptsAudioInput(chatConfig(&componentMapResolver{chat: &fakeChatModel{}})) {
		t.Fatal("AcceptsAudioInput() = true without AudioTranscript")
	}

	duplicate := audioTranscriptConfig(&componentMapResolver{chat: &fakeChatModel{}})
	duplicate.Graph.State.Fields = append(duplicate.Graph.State.Fields, StateField{Name: "second", Type: StateString, Merge: MergeReplace})
	duplicate.Graph.Nodes = append(duplicate.Graph.Nodes, NodeDefinition{
		ID: "second", Inputs: map[string]Binding{"messages": {From: "messages"}},
		Outputs: map[string]string{"text": "second"}, ChatModel: &ChatModelNode{Model: "chat", AudioTranscript: true},
	})
	duplicate.Graph.Edges = append(duplicate.Graph.Edges, EdgeDefinition{From: "prompt", To: "second"}, EdgeDefinition{From: "second", To: "end"})
	if err := ValidateConfig(duplicate); err == nil || !strings.Contains(err.Error(), "already set") {
		t.Fatalf("duplicate AudioTranscript error = %v", err)
	}

	nested := chatConfig(&componentMapResolver{chat: &fakeChatModel{}})
	child := audioTranscriptConfig(nil).Graph
	child.Name = "child"
	nested.Graph.State.Fields = append(nested.Graph.State.Fields, StateField{Name: "child", Type: StateString, Merge: MergeReplace})
	nested.Graph.Nodes = append(nested.Graph.Nodes, NodeDefinition{
		ID: "child", Inputs: map[string]Binding{"messages": {From: "input.messages"}},
		Outputs: map[string]string{"answer": "child"}, Subgraph: &SubgraphNode{Graph: child},
	})
	nested.Graph.Edges = append(nested.Graph.Edges, EdgeDefinition{From: "start", To: "child"}, EdgeDefinition{From: "child", To: "end"})
	if err := ValidateConfig(nested); err == nil || !strings.Contains(err.Error(), "only supported in the root Graph") {
		t.Fatalf("nested AudioTranscript error = %v", err)
	}
}

func TestAudioTurnPublishesModelTranscript(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// The transcript arrives after the first reply text, as the Doubao chat
	// adapter reports it.
	chat := &fakeChatModel{chunks: []*schema.Message{
		schema.AssistantMessage("等于", nil),
		TranscriptMessage("三加五等于几"),
		schema.AssistantMessage("八。", nil),
	}}
	core, err := New(ctx, audioTranscriptConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	// Without an ASR stage Audio Dock hands the audio route to the Agent.
	dock, err := audiodock.New(audiodock.Config{Agent: core})
	if err != nil {
		t.Fatal(err)
	}
	packets := [][]byte{{0x01, 0x02}, {0x03}}
	output, err := dock.Transform(ctx, audioInput("speech", packets))
	if err != nil {
		t.Fatal(err)
	}
	chunks := drain(t, output)

	var transcript, reply strings.Builder
	var sideband [][]byte
	sidebandBegin, sidebandEnd := false, false
	firstSideband, firstReply := -1, -1
	for index, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel && firstSideband < 0 {
			firstSideband = index
		}
		if text, ok := chunk.Part.(genx.Text); ok && text != "" && chunk.Role == genx.RoleModel && firstReply < 0 {
			firstReply = index
		}
		label := ""
		if chunk.Ctrl != nil {
			label = chunk.Ctrl.Label
			if chunk.Ctrl.Error != "" {
				t.Fatalf("output error = %q", chunk.Ctrl.Error)
			}
		}
		switch {
		case label == genx.HistoryUserAudioLabel:
			if chunk.Ctrl.StreamID != "speech" || chunk.Role != genx.RoleUser {
				t.Fatalf("user audio sideband route = %q role %q", chunk.Ctrl.StreamID, chunk.Role)
			}
			sidebandBegin = sidebandBegin || chunk.IsBeginOfStream()
			sidebandEnd = sidebandEnd || chunk.IsEndOfStream()
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) != 0 {
				sideband = append(sideband, blob.Data)
			}
		case chunk.Role == genx.RoleUser:
			if text, ok := chunk.Part.(genx.Text); ok {
				if label != "transcript" || chunk.Ctrl.StreamID != "speech" {
					t.Fatalf("transcript route label=%q stream=%q", label, chunk.Ctrl.StreamID)
				}
				transcript.WriteString(string(text))
			}
		case chunk.Role == genx.RoleModel:
			if text, ok := chunk.Part.(genx.Text); ok {
				reply.WriteString(string(text))
			}
		}
	}
	if firstSideband < 0 || firstReply < 0 || firstSideband > firstReply {
		t.Fatalf("user audio sideband at %d, first reply text at %d; History needs the user audio first", firstSideband, firstReply)
	}
	if transcript.String() != "三加五等于几" {
		t.Fatalf("transcript = %q", transcript.String())
	}
	if reply.String() != "等于八。" {
		t.Fatalf("reply = %q", reply.String())
	}
	if !sidebandBegin || !sidebandEnd || !slices.EqualFunc(sideband, packets, slices.Equal) {
		t.Fatalf("user audio sideband BOS=%v EOS=%v packets=%v", sidebandBegin, sidebandEnd, sideband)
	}

	chat.mu.Lock()
	inputs := slices.Clone(chat.inputs)
	chat.mu.Unlock()
	if len(inputs) != 1 {
		t.Fatalf("ChatModel calls = %d, want 1", len(inputs))
	}
	var audioParts []string
	for _, message := range inputs[0] {
		switch message.Role {
		case schema.User:
			for _, part := range message.UserInputMultiContent {
				if part.Audio == nil || part.Audio.Base64Data == nil {
					t.Fatalf("user part = %#v", part)
				}
				audioParts = append(audioParts, part.Audio.MIMEType+":"+*part.Audio.Base64Data)
			}
		}
	}
	want := []string{
		"audio/opus:" + base64.StdEncoding.EncodeToString(packets[0]),
		"audio/opus:" + base64.StdEncoding.EncodeToString(packets[1]),
	}
	if !slices.Equal(audioParts, want) {
		t.Fatalf("model input audio=%q, want %q", audioParts, want)
	}

	history, err := core.history.load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var roles []string
	for _, message := range history {
		roles = append(roles, string(message.Role)+":"+message.Content)
		if len(message.UserInputMultiContent) != 0 {
			t.Fatal("History kept the user audio")
		}
	}
	if !slices.Equal(roles, []string{"user:三加五等于几", "assistant:等于八。"}) {
		t.Fatalf("History = %q", roles)
	}
}

func TestAudioTurnWithoutTranscriptKeepsReply(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	chat := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("等于八。", nil)}}
	core, err := New(ctx, audioTranscriptConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(ctx, audioInput("speech", [][]byte{{0x01}}))
	if err != nil {
		t.Fatal(err)
	}
	var reply strings.Builder
	sideband := 0
	for _, chunk := range drain(t, output) {
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("output error = %q", chunk.Ctrl.Error)
		}
		switch {
		case chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel:
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) != 0 {
				sideband++
			}
		case chunk.Role == genx.RoleUser && chunk.Part != nil:
			t.Fatalf("transcript route published without a transcript: %#v", chunk)
		case chunk.Role == genx.RoleModel:
			if text, ok := chunk.Part.(genx.Text); ok {
				reply.WriteString(string(text))
			}
		}
	}
	if reply.String() != "等于八。" || sideband != 1 {
		t.Fatalf("reply=%q user audio packets=%d", reply.String(), sideband)
	}
	history, err := core.history.load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Role != schema.Assistant || history[0].Content != "等于八。" {
		t.Fatalf("History = %#v, want only the assistant reply", history)
	}
}

func TestAudioInputWithoutTranscriptNodeIsNotATurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	chat := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("answer", nil)}}
	core, err := New(ctx, chatConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(ctx, audioInput("speech", [][]byte{{0x01}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range drain(t, output) {
		if chunk.Role == genx.RoleModel {
			t.Fatalf("text-only Graph answered audio input: %#v", chunk)
		}
	}
	chat.mu.Lock()
	defer chat.mu.Unlock()
	if len(chat.inputs) != 0 {
		t.Fatalf("ChatModel calls = %d, want 0", len(chat.inputs))
	}
}

func TestInterruptedAudioInputStartsNoTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	chat := &fakeChatModel{chunks: []*schema.Message{TranscriptMessage("x"), schema.AssistantMessage("y", nil)}}
	core, err := New(ctx, audioTranscriptConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	if err := input.Add(
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true, Error: "interrupted"}},
	); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range drain(t, output) {
		if chunk.Role == genx.RoleModel || chunk.Role == genx.RoleUser && chunk.Part != nil {
			t.Fatalf("interrupted audio produced output: %#v", chunk)
		}
	}
	chat.mu.Lock()
	defer chat.mu.Unlock()
	if len(chat.inputs) != 0 {
		t.Fatalf("ChatModel calls = %d, want 0", len(chat.inputs))
	}
}

func TestAudioInputFailureFailsSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	core, err := New(ctx, audioTranscriptConfig(&componentMapResolver{chat: &fakeChatModel{}}))
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	if err := input.Add(
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true, EndOfStream: true, Error: "microphone failed"}},
	); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err := output.Next()
		if err == nil {
			continue
		}
		if isStreamEnd(err) || errors.Is(err, context.Canceled) {
			t.Fatalf("session ended with %v, want the audio input failure", err)
		}
		if !strings.Contains(err.Error(), "microphone failed") {
			t.Fatalf("session error = %v", err)
		}
		return
	}
}

// audioTranscriptConfig answers through one root transcribing ChatModel node
// that receives the ordered input messages.
func audioTranscriptConfig(resolver ComponentResolver) Config {
	config := chatConfig(resolver)
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "prompt", Inputs: map[string]Binding{"conversation": {From: "input.messages"}},
		Outputs: map[string]string{"messages": "messages"},
		Prompt: &PromptNode{
			Format: PromptFString,
			Messages: []PromptMessage{
				{Role: PromptSystem, Template: "system"},
				{Placeholder: "conversation"},
			},
		},
	}
	config.Graph.Nodes[1].ChatModel.AudioTranscript = true
	return config
}

func audioInput(streamID string, packets [][]byte) genx.Stream {
	input := newInputBuilder()
	for index, packet := range packets {
		_ = input.Add(&genx.MessageChunk{
			Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: index == 0},
		})
	}
	_ = input.Add(&genx.MessageChunk{
		Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true},
	})
	_ = input.Done(genx.Usage{})
	return input.Stream()
}

func TestTranscriptMessageRoundTrip(t *testing.T) {
	text, ok := transcriptOf(TranscriptMessage("你好"))
	if !ok || text != "你好" {
		t.Fatalf("transcriptOf(TranscriptMessage) = %q, %v", text, ok)
	}
	if _, ok := transcriptOf(schema.UserMessage("你好")); ok {
		t.Fatal("plain user message reported as a transcript")
	}
}
