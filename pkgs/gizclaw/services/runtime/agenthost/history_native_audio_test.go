package agenthost

import (
	"context"
	"fmt"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestNativeAudioHistoryWithoutTranscriptIsAvailableBeforeSessionEnds(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprintf("direct-model-%t", direct), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			history := newTestWorkspaceHistory(t, newTestObjectStore(t))
			config := genxeino.Config{
				Agent: genxeino.AgentConfig{ID: "audio-only", Name: "audio-only"}, Components: finiteHistoryModel{}, AudioInput: direct,
				Graph: genxeino.GraphDefinition{
					Name: "audio-only", State: genxeino.StateDefinition{Fields: []genxeino.StateField{{Name: "answer", Type: genxeino.StateString, Merge: genxeino.MergeReplace}}},
					Nodes:   []genxeino.NodeDefinition{{ID: "chat", ChatModel: &genxeino.ChatModelNode{Model: "chat"}, Inputs: map[string]genxeino.Binding{"messages": {From: "input.messages"}}, Outputs: map[string]string{"text": "answer"}}},
					Edges:   []genxeino.EdgeDefinition{{From: "start", To: "chat"}, {From: "chat", To: "end"}},
					Outputs: []genxeino.OutputDefinition{{Node: "chat", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true}},
				},
			}
			core, err := genxeino.New(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			dock, err := audiodock.New(audiodock.Config{Agent: core, AgentAudioInput: direct, ASR: historyAudioASR(func(context.Context, []*genx.Blob) (string, error) { return "", nil })})
			if err != nil {
				t.Fatal(err)
			}
			agent := wrapHistoryAgent(NewTransformerAgent(dock), history)
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			output, err := agent.Transform(withHistoryGearID(ctx, "native-peer"), input.Stream())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationVoIP)
			if err != nil {
				t.Fatal(err)
			}
			defer encoder.Close()
			packet, err := encoder.Encode(make([]int16, 320), 320)
			if err != nil {
				t.Fatal(err)
			}
			for _, chunk := range []*genx.MessageChunk{
				{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true}},
				{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true}},
				{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true}},
				{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true}},
			} {
				if err := input.Add(chunk); err != nil {
					t.Fatal(err)
				}
			}
			for {
				chunk, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				if direct && chunk.Role == genx.RoleModel && chunk.IsEndOfStream() && chunk.Ctrl.ResponseEpochEnd || !direct && chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" && chunk.IsEndOfStream() {
					break
				}
			}
			// The input is intentionally still open: turn completion must make
			// audio-only History durable and replayable without session teardown.
			page, err := history.ListEntries(ctx, "", "", 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Entries) == 0 || page.Entries[0].Type != historyEntryTypeGear || page.Entries[0].TurnID != "speech" || page.Entries[0].Text != "" || len(page.Entries[0].Assets) != 1 || !page.Entries[0].ReplayAvailable {
				t.Fatalf("audio-only History unavailable in the open session: %+v", page.Entries)
			}
			reader, err := history.ReadAsset(ctx, page.Entries[0].Assets[0].Name)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			asset, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			packets, err := historyOpusFramesFromOgg(asset)
			if err != nil || len(packets) != 1 || !slices.Equal(packets[0], packet) {
				t.Fatalf("audio-only replay changed: packets=%v error=%v", packets, err)
			}
		})
	}
}

// Exercise the actual AudioDock -> Eino -> AgentHost -> SQL History path.
// There is no Giztest client, matcher, prompt, or live model in this regression.
func TestNativeAudioHistoryKeepsTurnsBeforeTheirReplies(t *testing.T) {
	for _, replies := range []int{1, 2} {
		t.Run(fmt.Sprintf("reply-records-%d", replies), func(t *testing.T) {
			testNativeAudioHistoryOrder(t, replies, false)
		})
	}
}

func TestNativeAudioNextTurnWaitsForPriorTranscript(t *testing.T) {
	testNativeAudioHistoryOrder(t, 2, true)
}

func testNativeAudioHistoryOrder(t *testing.T, replyRecords int, overlap bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	reader := &nativeHistoryReader{HistoryReader: history, secondRead: make(chan struct{}, 1)}
	chat := &nativeHistoryModel{requests: make(chan []*schema.Message, 2)}
	questions := []string{"first spoken question", "second spoken question"}
	release := []chan struct{}{make(chan struct{}), make(chan struct{})}
	graph := genxeino.GraphDefinition{
		Name:    "native-history",
		State:   genxeino.StateDefinition{Fields: []genxeino.StateField{{Name: "answer", Type: genxeino.StateString, Merge: genxeino.MergeReplace}}},
		Nodes:   []genxeino.NodeDefinition{{ID: "chat", ChatModel: &genxeino.ChatModelNode{Model: "chat"}, Inputs: map[string]genxeino.Binding{"messages": {From: "input.messages"}}, Outputs: map[string]string{"text": "answer"}}},
		Edges:   []genxeino.EdgeDefinition{{From: "start", To: "chat"}},
		Outputs: []genxeino.OutputDefinition{{Node: "chat", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true}},
	}
	last := "chat"
	for index := 1; index < replyRecords; index++ {
		id := fmt.Sprintf("reply-part-%d", index)
		graph.State.Fields = append(graph.State.Fields, genxeino.StateField{Name: id, Type: genxeino.StateString, Merge: genxeino.MergeReplace})
		graph.Nodes = append(graph.Nodes, genxeino.NodeDefinition{ID: id,
			Inputs: map[string]genxeino.Binding{"source": {From: "answer"}}, Outputs: map[string]string{"text": id},
			Transform: &genxeino.TransformNode{Operation: genxeino.TransformConcatText, Order: []string{"source"}},
		})
		graph.Edges = append(graph.Edges, genxeino.EdgeDefinition{From: last, To: id})
		graph.Outputs = append(graph.Outputs, genxeino.OutputDefinition{Node: id, Field: id, Name: id, MIMEType: "text/plain"})
		last = id
	}
	graph.Edges = append(graph.Edges, genxeino.EdgeDefinition{From: last, To: "end"})
	transcription := 0
	transcribe := func(ctx context.Context, audio []*genx.Blob) (string, error) {
		index := transcription
		transcription++
		if index >= len(questions) || len(audio) != 1 {
			return "", fmt.Errorf("unexpected audio turn %d with %d parts", index, len(audio))
		}
		select {
		case <-release[index]:
			return questions[index], nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	core, err := genxeino.New(ctx, genxeino.Config{
		Agent:      genxeino.AgentConfig{ID: "native-history", Name: "native-history"},
		AudioInput: true, Components: chat, History: &genxeino.HistoryConfig{Store: reader, Scope: "demo", Limit: 50},

		Graph: graph,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	dock, err := audiodock.New(audiodock.Config{Agent: core, AgentAudioInput: true, ASR: historyAudioASR(transcribe)})
	if err != nil {
		t.Fatal(err)
	}
	agent := wrapHistoryAgent(NewTransformerAgent(dock), history)
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	output, err := agent.Transform(withHistoryGearID(ctx, "native-peer"), input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationVoIP)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	packet, err := encoder.Encode(make([]int16, 320), 320)
	if err != nil {
		t.Fatal(err)
	}
	sendTurn := func(turn int) {
		t.Helper()
		id := fmt.Sprintf("speech-%d", turn)
		marker := fmt.Sprintf("after-input-end-%d", turn)
		for _, chunk := range []*genx.MessageChunk{
			{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}},
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}},
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}},
			{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}},
			{Ctrl: &genx.StreamCtrl{StreamID: marker, Label: marker}},
		} {
			if err := input.Add(chunk); err != nil {
				t.Fatal(err)
			}
		}
	}
	var expected []*schema.Message
	for turn, question := range questions {
		id := fmt.Sprintf("speech-%d", turn)
		marker := fmt.Sprintf("after-input-end-%d", turn)
		if !overlap || turn == 0 {
			sendTurn(turn)
		}
		// Final transcription is unavailable until the ordinary route EOS has
		// crossed the input router, exactly the ordering seen in real speech.
		completedReplies := 0
		transcriptDone := false
		var request []*schema.Message
		for completedReplies < replyRecords || !transcriptDone {
			chunk, err := output.Next()
			if err != nil {
				t.Fatal(err)
			}
			if chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" && chunk.IsEndOfStream() {
				transcriptDone = true
			}
			if chunk.Ctrl != nil && chunk.Ctrl.StreamID == marker && !overlap {
				close(release[turn])
			}
			if chunk.Role == genx.RoleModel && chunk.IsEndOfStream() && chunk.Ctrl != nil && chunk.Ctrl.ResponseEpochEnd {
				completedReplies++
			}
			if overlap && turn == 0 && completedReplies == replyRecords && request == nil {
				select {
				case request = <-chat.requests:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				// B arrives after all replies to A, while A's ASR remains blocked.
				sendTurn(1)
				close(release[1])
				select {
				case <-reader.secondRead:
				case <-ctx.Done():
					t.Fatal("next turn did not reach its History query")
				}
				select {
				case next := <-chat.requests:
					t.Fatalf("next model request bypassed pending transcription: %+v", next)
				case <-time.After(50 * time.Millisecond):
				}
				close(release[0])
			}
		}
		if request == nil {
			select {
			case request = <-chat.requests:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
		expected = append(expected, schema.UserMessage(question))
		if len(request) != len(expected) {
			t.Errorf("turn %d model has %d messages, want %d: %+v", turn, len(request), len(expected), request)
		} else {
			for index, want := range expected {
				if index == len(expected)-1 {
					if request[index].Role != schema.User || len(request[index].UserInputMultiContent) == 0 {
						t.Errorf("current turn did not receive raw audio: %+v", request[index])
					}
					continue
				}
				if request[index].Role != want.Role || request[index].Content != want.Content {
					t.Errorf("turn %d model message %d = %s:%q, want %s:%q", turn, index, request[index].Role, request[index].Content, want.Role, want.Content)
				}
			}
		}
		for range replyRecords {
			expected = append(expected, schema.AssistantMessage("allowed", nil))
		}
		page, err := history.ListEntries(ctx, "", "", 10)
		if err != nil {
			t.Fatal(err)
		}
		// The expected count comes only from this fixture's declared outputs.
		// There is no one-reply-per-turn restriction: distinct reply routes
		// must remain distinct, in order before the next user's input.
		wantRows := (1 + replyRecords) * (turn + 1)
		if len(page.Entries) != wantRows {
			t.Errorf("turn %d stored %d History rows, want %d declared records: %+v", turn, len(page.Entries), wantRows, page.Entries)
		}
		for index, entry := range page.Entries {
			t.Logf("turn %d History %d: type=%s text=%q input=%s created=%s assets=%d", turn, index, entry.Type, entry.Text, entry.TurnID, entry.CreatedAt.Format(time.RFC3339Nano), len(entry.Assets))
		}
		if len(page.Entries) == wantRows {
			start := (1 + replyRecords) * turn
			user := page.Entries[start]
			if user.Type != historyEntryTypeGear || user.Text != question || user.TurnID != id || len(user.Assets) != 1 {
				t.Errorf("turn %d lost correlated user text/audio: %+v", turn, user)
			}
			for index, reply := range page.Entries[start+1:] {
				if reply.Type != historyEntryTypeAgent || reply.Name != graph.Outputs[index].Name || reply.Text != "allowed" || reply.CreatedAt.Before(user.CreatedAt) {
					t.Errorf("turn %d reply record %d out of order or changed: %+v", turn, index, reply)
				}
			}
		}
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	for {
		_, err := output.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

type nativeHistoryReader struct {
	memory.HistoryReader
	secondRead chan struct{}
}

func (r *nativeHistoryReader) Query(ctx context.Context, query memory.HistoryQuery) (memory.HistoryPage, error) {
	if query.ExcludeTurnID == "speech-1" {
		select {
		case r.secondRead <- struct{}{}:
		default:
		}
	}
	return r.HistoryReader.Query(ctx, query)
}

type nativeHistoryModel struct {
	finiteHistoryModel
	requests chan []*schema.Message
}

func (m *nativeHistoryModel) ResolveChatModel(context.Context, string) (model.BaseChatModel, error) {
	return m, nil
}

func (m *nativeHistoryModel) Stream(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	select {
	case m.requests <- slices.Clone(input):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return m.finiteHistoryModel.Stream(ctx, input, options...)
}

// historyAudioASR is an independent recognizer fixture. AudioDock, Eino and
// AgentHost History own all routing and persistence exercised by these tests.
func historyAudioASR(transcribe func(context.Context, []*genx.Blob) (string, error)) genx.Transformer {
	return historyTransformerFunc(func(ctx context.Context, input genx.Stream) (genx.Stream, error) {
		output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
		go func() {
			defer input.Close()
			defer output.Done(genx.Usage{})
			var audio []*genx.Blob
			for {
				chunk, err := input.Next()
				if err != nil {
					return
				}
				if blob, ok := chunk.Part.(*genx.Blob); ok {
					next := chunk.Clone()
					next.Name = "transcript"
					next.Ctrl.Label = genx.HistoryUserAudioLabel
					_ = output.Add(next)
					if len(blob.Data) != 0 {
						audio = append(audio, blob)
					}
				}
				if !chunk.IsEndOfStream() || len(audio) == 0 {
					continue
				}
				text, err := transcribe(ctx, audio)
				if err != nil {
					_ = output.Abort(err)
					return
				}
				_ = output.Add(&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: chunk.Ctrl.StreamID, Label: "transcript", BeginOfStream: true, EndOfStream: true}})
				audio = nil
			}
		}()
		return output.Stream(), nil
	})
}
