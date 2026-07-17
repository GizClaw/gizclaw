package genx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func readStreamTerminalError(t *testing.T, stream Stream) error {
	t.Helper()
	for {
		_, err := stream.Next()
		if err != nil {
			return err
		}
	}
}

func TestStreamBuilderTerminalStates(t *testing.T) {
	cases := []struct {
		name   string
		do     func(*StreamBuilder) error
		status Status
	}{
		{name: "done", do: func(sb *StreamBuilder) error { return sb.Done(Usage{PromptTokenCount: 1}) }, status: StatusDone},
		{name: "truncated", do: func(sb *StreamBuilder) error { return sb.Truncated(Usage{GeneratedTokenCount: 5}) }, status: StatusTruncated},
		{name: "blocked", do: func(sb *StreamBuilder) error { return sb.Blocked(Usage{}, "policy") }, status: StatusBlocked},
		{name: "error", do: func(sb *StreamBuilder) error { return sb.Unexpected(Usage{}, errors.New("boom")) }, status: StatusError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sb := NewStreamBuilder((&ModelContextBuilder{}).Build(), 2)
			if err := tc.do(sb); err != nil {
				t.Fatalf("set terminal state failed: %v", err)
			}

			err := readStreamTerminalError(t, sb.Stream())
			var state *State
			if !errors.As(err, &state) {
				t.Fatalf("expected *State error, got: %T %v", err, err)
			}
			if state.Status() != tc.status {
				t.Fatalf("unexpected status: got=%v want=%v", state.Status(), tc.status)
			}
		})
	}
}

func TestGrowableStreamBuilderDoesNotBlockProducerAtInitialCapacity(t *testing.T) {
	sb := NewGrowableStreamBuilder((&ModelContextBuilder{}).Build(), 1)
	const count = 100
	for i := range count {
		if err := sb.Add(&MessageChunk{Part: Text("chunk")}); err != nil {
			t.Fatalf("Add(%d) error = %v", i, err)
		}
	}
	if err := sb.Done(Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	stream := sb.Stream()
	for i := range count {
		if _, err := stream.Next(); err != nil {
			t.Fatalf("Next(%d) error = %v", i, err)
		}
	}
	if err := readStreamTerminalError(t, stream); err == nil {
		t.Fatal("terminal error = nil")
	}
}

func TestStreamBuilderAddBindsToolAndInvoke(t *testing.T) {
	tool := MustNewFuncTool[struct {
		V int `json:"v"`
	}]("adder", "adder", InvokeFunc[struct {
		V int `json:"v"`
	}](func(_ context.Context, _ *FuncCall, arg struct {
		V int `json:"v"`
	}) (any, error) {
		return arg.V + 2, nil
	}))

	var mcb ModelContextBuilder
	mcb.AddTool(tool)
	sb := NewStreamBuilder(mcb.Build(), 4)

	if err := sb.Add(&MessageChunk{
		Role:     RoleModel,
		ToolCall: &ToolCall{ID: "c1", FuncCall: &FuncCall{Name: "adder", Arguments: `{"v":3}`}},
	}); err != nil {
		t.Fatalf("add tool chunk failed: %v", err)
	}
	if err := sb.Done(Usage{}); err != nil {
		t.Fatalf("done failed: %v", err)
	}

	chunk, err := sb.Stream().Next()
	if err != nil {
		t.Fatalf("read chunk failed: %v", err)
	}
	if chunk == nil || chunk.ToolCall == nil || chunk.ToolCall.FuncCall == nil {
		t.Fatalf("unexpected chunk: %#v", chunk)
	}
	if _, err := chunk.ToolCall.Invoke(context.Background()); err != nil {
		t.Fatalf("tool invoke failed after binding: %v", err)
	}
}

func TestStreamBuilderAbortAndUnexpectedStatus(t *testing.T) {
	sb := NewStreamBuilder((&ModelContextBuilder{}).Build(), 2)
	want := errors.New("abort")
	if err := sb.Abort(want); err != nil {
		t.Fatalf("abort failed: %v", err)
	}
	if _, err := sb.Stream().Next(); err == nil || !strings.Contains(err.Error(), "abort") {
		t.Fatalf("expected abort error, got: %v", err)
	}

	sb2 := NewStreamBuilder((&ModelContextBuilder{}).Build(), 2)
	if err := sb2.rb.Add(&StreamEvent{Status: Status(999)}); err != nil {
		t.Fatalf("inject event failed: %v", err)
	}
	if err := sb2.rb.CloseWrite(); err != nil {
		t.Fatalf("close write failed: %v", err)
	}

	if _, err := sb2.Stream().Next(); err == nil || !strings.Contains(err.Error(), "unexpected stream status") {
		t.Fatalf("expected unexpected status error, got: %v", err)
	}
}
