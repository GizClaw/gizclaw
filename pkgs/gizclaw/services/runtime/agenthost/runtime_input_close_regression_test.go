package agenthost

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type inputDrainingCloseProbe struct {
	input           *InputStream
	ctx             context.Context
	entered         chan struct{}
	done            chan struct{}
	canceledAtClose bool
}

func (s *inputDrainingCloseProbe) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (s *inputDrainingCloseProbe) Close() error {
	s.canceledAtClose = s.ctx.Err() != nil
	close(s.entered)
	_, _ = s.input.Next()
	close(s.done)
	return nil
}
func (s *inputDrainingCloseProbe) CloseWithError(error) error { return s.Close() }

type inputDrainProbeTransformer struct{ output *inputDrainingCloseProbe }

func (t inputDrainProbeTransformer) Transform(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
	t.output.ctx = ctx
	return t.output, nil
}

func TestRuntimeStopClosesInputBeforeWaitingForOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	producerCtx, cancelProducer := context.WithCancel(t.Context())
	defer cancelProducer()
	input := NewInputStream(1)
	defer input.Close()
	output := &inputDrainingCloseProbe{input: input, ctx: producerCtx, entered: make(chan struct{}), done: make(chan struct{})}
	run := &runtime{cancel: cancelProducer, input: input, output: output, done: output.done}
	result := make(chan error, 1)
	go func() { result <- run.stop(ctx) }()
	select {
	case <-output.entered:
	case <-ctx.Done():
		t.Fatal("output close did not begin")
	}
	if !output.canceledAtClose {
		t.Error("output close began before producer cancellation")
	}
	if err := input.Push(ctx, nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("input still accepted data while output close waited: %v", err)
	}
	_ = input.Close()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("stop failed to retire a reader waiting for input")
	}
}

func TestWorkspaceQuiescenceClosesInputBeforeWaitingForOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	input := NewInputStream(1)
	defer input.Close()
	output := &inputDrainingCloseProbe{input: input, entered: make(chan struct{}), done: make(chan struct{})}
	registry := NewRuntimeRegistry()
	generation := &agentGeneration{registry: registry, transforms: make(map[*runtimeTransform]struct{})}
	registry.runtimes[runtimeKey("workspace")] = &workspaceRuntime{current: generation, generations: map[*agentGeneration]struct{}{generation: {}}}
	agent := &quiescingAgent{Agent: NewTransformerAgent(inputDrainProbeTransformer{output: output}), generation: generation}
	if _, err := agent.Transform(ctx, input); err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{})
	go func() { registry.Quiesce("workspace"); close(finished) }()
	select {
	case <-output.entered:
	case <-ctx.Done():
		t.Fatal("quiescence did not close output")
	}
	if !output.canceledAtClose {
		t.Error("output close began before producer cancellation")
	}
	if err := input.Push(ctx, nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("quiescing input still accepted data while output close waited: %v", err)
	}
	_ = input.Close()
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("quiescence failed to release the input reader")
	}
}
