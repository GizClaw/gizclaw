//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/google/jsonschema-go/jsonschema"
)

const (
	dashScopeAPIKeyEnv        = "GIZCLAW_GENX_E2E_DASHSCOPE_API_KEY"
	realtimeToolName          = "lookup_verification_token"
	realtimeToolResponseToken = "GIZCLAW_REALTIME_TOOL_OK"
	realtimeToolInstructions  = "For every user request, you must call lookup_verification_token exactly once with key \"realtime\". After receiving its result, reply with exactly the returned token and nothing else."
)

type realtimeE2EToolCall struct {
	name      string
	arguments string
}

// realtimeE2EToolInvoker deliberately implements ToolInvoker directly. These
// provider tests must exercise the runtime abstraction rather than Toolkit.
type realtimeE2EToolInvoker struct {
	mu    sync.Mutex
	calls []realtimeE2EToolCall
}

func (*realtimeE2EToolInvoker) ResolveTools(context.Context) ([]genx.ToolDefinition, error) {
	return []genx.ToolDefinition{{
		Name:        realtimeToolName,
		Description: "Returns the fixed verification token for realtime provider testing.",
		Argument: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"key": {Type: "string"},
			},
			Required:             []string{"key"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
	}}, nil
}

func (invoker *realtimeE2EToolInvoker) InvokeTool(
	ctx context.Context,
	name string,
	arguments json.RawMessage,
) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var input struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return nil, fmt.Errorf("decode realtime e2e tool arguments: %w", err)
	}
	if name != realtimeToolName || input.Key != "realtime" {
		return nil, fmt.Errorf("unexpected realtime e2e tool call %q(%s)", name, arguments)
	}
	invoker.mu.Lock()
	invoker.calls = append(invoker.calls, realtimeE2EToolCall{
		name: name, arguments: string(arguments),
	})
	invoker.mu.Unlock()
	return json.RawMessage(`{"token":"` + realtimeToolResponseToken + `"}`), nil
}

func (invoker *realtimeE2EToolInvoker) snapshot() []realtimeE2EToolCall {
	invoker.mu.Lock()
	defer invoker.mu.Unlock()
	return append([]realtimeE2EToolCall(nil), invoker.calls...)
}

func pushDashScopeToolTurn(
	ctx context.Context,
	input *genx.RealtimeStream,
	streamID string,
	packets [][]byte,
) error {
	for index, packet := range packets {
		chunk := &genx.MessageChunk{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: duplexInputMIME, Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: streamID},
		}
		if index == 0 {
			chunk.Ctrl.BeginOfStream = true
		}
		if index == len(packets)-1 {
			chunk.Ctrl.EndOfStream = true
		}
		if err := input.Push(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

func waitForRealtimeToolContinuation(
	t *testing.T,
	ctx context.Context,
	output genx.Stream,
	feedDone <-chan error,
) string {
	t.Helper()
	var response strings.Builder
	for {
		select {
		case err := <-feedDone:
			feedDone = nil
			if err != nil {
				t.Fatalf("feed realtime tool input: %v", err)
			}
		default:
		}

		chunk, err := output.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.Fatalf("realtime output closed before tool continuation; response=%q", response.String())
			}
			t.Fatalf("read realtime tool output: %v; response=%q", err, response.String())
		}
		if chunk == nil {
			continue
		}
		if chunk.ToolCall != nil || chunk.Role == genx.RoleTool {
			t.Fatalf("internal tool control leaked to public output: %#v", chunk)
		}
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("realtime output error: %s", chunk.Ctrl.Error)
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			response.WriteString(string(text))
			if strings.Contains(response.String(), realtimeToolResponseToken) {
				if feedDone != nil {
					select {
					case err := <-feedDone:
						if err != nil {
							t.Fatalf("feed realtime tool input: %v", err)
						}
					case <-ctx.Done():
						t.Fatalf(
							"wait realtime input completion: %v; response=%q",
							ctx.Err(),
							response.String(),
						)
					}
				}
				return response.String()
			}
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait realtime tool continuation: %v; response=%q", err, response.String())
		}
	}
}

func assertSingleRealtimeToolCall(t *testing.T, invoker *realtimeE2EToolInvoker) {
	t.Helper()
	calls := invoker.snapshot()
	if len(calls) != 1 ||
		calls[0].name != realtimeToolName ||
		!strings.Contains(calls[0].arguments, `"key"`) {
		t.Fatalf("realtime tool calls = %#v, want one %s call", calls, realtimeToolName)
	}
}
