package eino

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
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func TestGraphExecutionPropagatesEveryNodeFailure(t *testing.T) {
	t.Parallel()
	nodeErr := errors.New("adversarial node failure")
	badChild := childTextGraph("bad-child", nil)
	badChild.Nodes[0].Script = &ScriptNode{
		Language: ScriptStarlark,
		Source:   "def run(input):\n  return 1 // 0\n",
		Limits: ScriptLimits{
			MaxExecutionSteps: 1_000,
			Timeout:           time.Second,
			MaxInputBytes:     1 << 10,
			MaxOutputBytes:    1 << 10,
		},
	}
	tests := []struct {
		name   string
		config func() Config
		want   string
	}{
		{
			name: "lambda",
			config: func() Config {
				config := textConfig()
				config.Lambdas = staticLambdaResolver{resolved: ResolvedLambda{
					Lambda: compose.InvokableLambda(
						func(context.Context, map[string]any) (map[string]any, error) {
							return nil, nodeErr
						},
					),
					Inputs:  map[string]StateType{"value": StateString},
					Outputs: map[string]StateType{"value": StateString},
				}}
				config.Graph.Nodes[0] = NodeDefinition{
					ID:      "answer",
					Inputs:  map[string]Binding{"value": {From: "input.text"}},
					Outputs: map[string]string{"value": "answer"},
					Lambda:  &LambdaRefNode{Lambda: "failing"},
				}
				return config
			},
			want: nodeErr.Error(),
		},
		{
			name: "chat model open",
			config: func() Config {
				return chatConfig(&componentMapResolver{
					chat: &adversarialChatModel{openErr: nodeErr},
				})
			},
			want: nodeErr.Error(),
		},
		{
			name: "chat model receive",
			config: func() Config {
				return chatConfig(&componentMapResolver{
					chat: &adversarialChatModel{receiveErr: nodeErr},
				})
			},
			want: nodeErr.Error(),
		},
		{
			name: "retriever",
			config: func() Config {
				config := textConfig()
				config.Components = &componentMapResolver{
					retriever: adversarialRetriever{err: nodeErr},
				}
				config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{
					Name: "documents", Type: StateDocuments, Merge: MergeReplace,
				})
				config.Graph.Nodes = []NodeDefinition{
					{
						ID:      "retrieve",
						Outputs: map[string]string{"documents": "documents"},
						Retriever: &RetrieverNode{
							Retriever: "failing",
							Query:     Binding{From: "input.text"},
							TopK:      2,
						},
					},
					config.Graph.Nodes[0],
				}
				config.Graph.Edges = []EdgeDefinition{
					{From: "start", To: "retrieve"},
					{From: "retrieve", To: "answer"},
					{From: "answer", To: "end"},
				}
				return config
			},
			want: nodeErr.Error(),
		},
		{
			name: "script",
			config: func() Config {
				config := textConfig()
				config.Graph.Nodes[0].Transform = nil
				config.Graph.Nodes[0].Script = badChild.Nodes[0].Script
				return config
			},
			want: "division by zero",
		},
		{
			name: "subgraph",
			config: func() Config {
				config := textConfig()
				config.Graph.Nodes[0] = NodeDefinition{
					ID:       "answer",
					Inputs:   map[string]Binding{"text": {From: "input.text"}},
					Outputs:  map[string]string{"answer": "answer"},
					Subgraph: &SubgraphNode{Graph: badChild},
				}
				return config
			},
			want: "division by zero",
		},
		{
			name: "race all fail",
			config: func() Config {
				config := textConfig()
				config.Graph.Nodes[0] = NodeDefinition{
					ID:      "answer",
					Inputs:  map[string]Binding{"text": {From: "input.text"}},
					Outputs: map[string]string{"answer": "answer"},
					Race: &RaceNode{
						Branches: []RaceBranch{
							{ID: "one", Graph: badChild},
							{ID: "two", Graph: badChild},
						},
						Winner:         RaceWinnerDefinition{Mode: RaceFirstSuccess},
						MaxConcurrency: 2,
					},
				}
				return config
			},
			want: "Race has no winner",
		},
		{
			name: "race predicate misses",
			config: func() Config {
				goodChild := childTextGraph("good-child", &TransformNode{
					Operation: TransformSelect,
				})
				goodChild.Nodes[0].Inputs = map[string]Binding{"value": {From: "input.text"}}
				goodChild.Nodes[0].Outputs = map[string]string{"value": "answer"}
				config := textConfig()
				config.Graph.Nodes[0] = NodeDefinition{
					ID:      "answer",
					Inputs:  map[string]Binding{"text": {From: "input.text"}},
					Outputs: map[string]string{"answer": "answer"},
					Race: &RaceNode{
						Branches: []RaceBranch{{ID: "only", Graph: goodChild}},
						Winner: RaceWinnerDefinition{
							Mode: RacePredicate,
							When: &Predicate{
								Field: "answer",
								Op:    PredicateEqual,
								Value: "never",
							},
						},
						MaxConcurrency: 1,
					},
				}
				return config
			},
			want: "Race has no winner",
		},
		{
			name: "batch child",
			config: func() Config {
				child := GraphDefinition{
					Name: "batch-child",
					State: StateDefinition{Fields: []StateField{
						{Name: "item", Type: StateString, Merge: MergeReplace},
						{Name: "answer", Type: StateString, Merge: MergeReplace},
					}},
					Nodes: []NodeDefinition{{
						ID:      "answer",
						Inputs:  map[string]Binding{"value": {From: "item"}},
						Outputs: map[string]string{"value": "answer"},
						Lambda:  &LambdaRefNode{Lambda: "batch"},
					}},
					Edges: []EdgeDefinition{
						{From: "start", To: "answer"},
						{From: "answer", To: "end"},
					},
					Outputs: []OutputDefinition{{
						Node: "answer", Field: "answer", Name: "answer",
						MIMEType: "text/plain", Primary: true,
					}},
				}
				config := textConfig()
				config.Lambdas = staticLambdaResolver{resolved: ResolvedLambda{
					Lambda: compose.InvokableLambda(
						func(_ context.Context, input map[string]any) (map[string]any, error) {
							if input["value"] == "bad" {
								return nil, nodeErr
							}
							return input, nil
						},
					),
					Inputs:  map[string]StateType{"value": StateString},
					Outputs: map[string]StateType{"value": StateString},
				}}
				config.Graph.State.Fields = []StateField{
					{Name: "items", Type: StateList, Merge: MergeReplace},
					{Name: "results", Type: StateList, Merge: MergeReplace},
					{Name: "answer", Type: StateString, Merge: MergeReplace},
				}
				config.Graph.Nodes = []NodeDefinition{
					{
						ID:      "items",
						Outputs: map[string]string{"items": "items"},
						Script:  constantListScript([]string{"ok", "bad", "late"}),
					},
					{
						ID:      "answer",
						Outputs: map[string]string{"items": "results"},
						Batch: &BatchNode{
							Items: Binding{From: "items"}, Graph: child, MaxConcurrency: 2,
						},
					},
					{
						ID:      "publish",
						Inputs:  map[string]Binding{"items": {From: "results"}},
						Outputs: map[string]string{"text": "answer"},
						Script: &ScriptNode{
							Language: ScriptStarlark,
							Source:   "def run(input):\n  return {\"text\": \"|\".join(input[\"items\"])}\n",
							Limits: ScriptLimits{
								MaxExecutionSteps: 1_000,
								Timeout:           time.Second,
								MaxInputBytes:     1 << 10,
								MaxOutputBytes:    1 << 10,
							},
						},
					},
				}
				config.Graph.Edges = []EdgeDefinition{
					{From: "start", To: "items"},
					{From: "items", To: "answer"},
					{From: "answer", To: "publish"},
					{From: "publish", To: "end"},
				}
				config.Graph.Outputs = []OutputDefinition{{
					Node: "publish", Field: "answer", Name: "assistant",
					MIMEType: "text/plain", Primary: true,
				}}
				return config
			},
			want: nodeErr.Error(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			transformer, err := New(t.Context(), test.config())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			output, err := transformer.Transform(t.Context(), textInput("adversarial"))
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if got := streamFailure(t, output); !strings.Contains(got, test.want) {
				t.Fatalf("terminal failure = %q, want containing %q", got, test.want)
			}
		})
	}
}

func TestGraphExecutionRejectsMalformedInputLifecycles(t *testing.T) {
	t.Parallel()
	inputErr := errors.New("upstream exploded")
	tests := []struct {
		name  string
		input func() genx.Stream
		want  string
	}{
		{
			name: "upstream stream error",
			input: func() genx.Stream {
				builder := newInputBuilder()
				if err := builder.Abort(inputErr); err != nil {
					t.Fatalf("Abort() error = %v", err)
				}
				return builder.Stream()
			},
			want: inputErr.Error(),
		},
		{
			name: "text terminal error",
			input: func() genx.Stream {
				streamID := genx.NewStreamID()
				eos := genx.NewTextEndOfStream()
				eos.Ctrl.StreamID = streamID
				eos.Ctrl.Error = inputErr.Error()
				return inputFromChunks(t,
					genx.NewBeginOfStream(streamID),
					&genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text("partial"),
						Ctrl: &genx.StreamCtrl{StreamID: streamID},
					},
					eos,
				)
			},
			want: "input text Stream failed",
		},
		{
			name: "part terminal error",
			input: func() genx.Stream {
				streamID := genx.NewStreamID()
				return inputFromChunks(t,
					genx.NewBeginOfStream(streamID),
					&genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text("partial"),
						Ctrl: &genx.StreamCtrl{StreamID: streamID},
					},
					&genx.MessageChunk{
						Role: genx.RoleUser,
						Part: &genx.Blob{MIMEType: "image/png", Data: []byte{1}},
						Ctrl: &genx.StreamCtrl{
							StreamID:    streamID,
							EndOfStream: true,
							Error:       inputErr.Error(),
						},
					},
				)
			},
			want: "input part Stream failed",
		},
		{
			name: "mismatched stream ID",
			input: func() genx.Stream {
				firstID := genx.NewStreamID()
				secondID := genx.NewStreamID()
				return inputFromChunks(t,
					&genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text("first"),
						Ctrl: &genx.StreamCtrl{StreamID: firstID},
					},
					&genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text("second"),
						Ctrl: &genx.StreamCtrl{StreamID: secondID, EndOfStream: true},
					},
				)
			},
			want: "does not match active StreamID",
		},
		{
			name: "unsupported multimodal graph",
			input: func() genx.Stream {
				streamID := genx.NewStreamID()
				return inputFromChunks(t,
					&genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text("image"),
						Ctrl: &genx.StreamCtrl{StreamID: streamID},
					},
					&genx.MessageChunk{
						Role: genx.RoleUser,
						Part: &genx.Blob{MIMEType: "image/png", Data: []byte{1, 2, 3}},
						Ctrl: &genx.StreamCtrl{StreamID: streamID},
					},
					&genx.MessageChunk{
						Role: genx.RoleUser,
						Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true},
					},
				)
			},
			want: "multimodal input is unsupported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			transformer, err := New(t.Context(), textConfig())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			output, err := transformer.Transform(t.Context(), test.input())
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if got := streamFailure(t, output); !strings.Contains(got, test.want) {
				t.Fatalf("stream failure = %q, want containing %q", got, test.want)
			}
		})
	}
}

func TestGraphExecutionCloseWithErrorAndContextCancellation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cancel func(context.CancelCauseFunc, genx.Stream) error
	}{
		{
			name: "downstream close with error",
			cancel: func(_ context.CancelCauseFunc, output genx.Stream) error {
				return output.CloseWithError(errors.New("consumer failed"))
			},
		},
		{
			name: "transform context cancellation",
			cancel: func(cancel context.CancelCauseFunc, _ genx.Stream) error {
				cancel(errors.New("caller canceled"))
				return nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			component := newBlockingChatModel()
			transformer, err := New(t.Context(), chatConfig(&componentMapResolver{chat: component}))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(io.EOF)
			output, err := transformer.Transform(ctx, textInput("cancel"))
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			for {
				chunk, nextErr := output.Next()
				if nextErr != nil {
					t.Fatalf("Next() error = %v", nextErr)
				}
				if text, ok := chunk.Part.(genx.Text); ok && text == "first" {
					break
				}
			}
			if err := test.cancel(cancel, output); err != nil {
				t.Fatalf("cancel action error = %v", err)
			}
			select {
			case <-component.cancelled:
			case <-time.After(5 * time.Second):
				t.Fatal("component was not cancelled")
			}
		})
	}
}

func TestGraphExecutionNewTurnInterruptsActiveGraph(t *testing.T) {
	t.Parallel()
	component := &interruptingChatModel{
		firstCancelled: make(chan struct{}),
	}
	transformer, err := New(t.Context(), chatConfig(&componentMapResolver{chat: component}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	addTextTurn(t, input, "first")
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "first" {
			break
		}
	}
	addTextTurn(t, input, "second")
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	select {
	case <-component.firstCancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("first Graph run was not interrupted")
	}
	if got := joinedText(drain(t, output)); got != "second" {
		t.Fatalf("remaining output = %q, want second turn only", got)
	}
}

func streamFailure(t *testing.T, stream genx.Stream) string {
	t.Helper()
	var failure string
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			return failure
		}
		if err != nil {
			return err.Error()
		}
		if chunk != nil && chunk.IsEndOfStream() && chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			failure = chunk.Ctrl.Error
		}
	}
}

func inputFromChunks(t *testing.T, chunks ...*genx.MessageChunk) genx.Stream {
	t.Helper()
	builder := newInputBuilder()
	if err := builder.Add(chunks...); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	return builder.Stream()
}

func addTextTurn(t *testing.T, builder *genx.StreamBuilder, text string) {
	t.Helper()
	streamID := genx.NewStreamID()
	eos := genx.NewTextEndOfStream()
	eos.Ctrl.StreamID = streamID
	if err := builder.Add(
		genx.NewBeginOfStream(streamID),
		&genx.MessageChunk{
			Role: genx.RoleUser,
			Part: genx.Text(text),
			Ctrl: &genx.StreamCtrl{StreamID: streamID},
		},
		eos,
	); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
}

func constantListScript(values []string) *ScriptNode {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = fmt.Sprintf("%q", value)
	}
	return &ScriptNode{
		Language: ScriptStarlark,
		Source:   fmt.Sprintf("def run(input):\n  return {\"items\": [%s]}\n", strings.Join(quoted, ", ")),
		Limits: ScriptLimits{
			MaxExecutionSteps: 1_000,
			Timeout:           time.Second,
			MaxInputBytes:     1 << 10,
			MaxOutputBytes:    1 << 10,
		},
	}
}

type adversarialChatModel struct {
	openErr    error
	receiveErr error
}

func (chat *adversarialChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := chat.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.Recv()
}

func (chat *adversarialChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	if chat.openErr != nil {
		return nil, chat.openErr
	}
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Send(nil, chat.receiveErr)
	writer.Close()
	return reader, nil
}

type adversarialRetriever struct {
	err error
}

func (store adversarialRetriever) Retrieve(
	context.Context,
	string,
	...retriever.Option,
) ([]*schema.Document, error) {
	return nil, store.err
}

type interruptingChatModel struct {
	mu             sync.Mutex
	calls          int
	firstCancelled chan struct{}
}

func (chat *interruptingChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := chat.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			return schema.ConcatMessages(chunks)
		}
		if recvErr != nil {
			return nil, recvErr
		}
		chunks = append(chunks, chunk)
	}
}

func (chat *interruptingChatModel) Stream(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	chat.mu.Lock()
	chat.calls++
	call := chat.calls
	chat.mu.Unlock()
	if call > 1 {
		return schema.StreamReaderFromArray([]*schema.Message{
			schema.AssistantMessage("second", nil),
		}), nil
	}
	reader, writer := schema.Pipe[*schema.Message](0)
	go func() {
		defer writer.Close()
		if writer.Send(schema.AssistantMessage("first", nil), nil) {
			return
		}
		<-ctx.Done()
		close(chat.firstCancelled)
	}()
	return reader, nil
}
