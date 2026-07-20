package streamkit

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestSameConfiguredTransformerRunsIndependentConcurrentInvocations(t *testing.T) {
	release := make(chan struct{})
	transformer := invocationTransformer{release: release}
	contexts := make([]context.Context, 2)
	cancels := make([]context.CancelFunc, 2)
	outputs := make([]genx.Stream, 2)
	var wg sync.WaitGroup
	for index := range outputs {
		contexts[index], cancels[index] = context.WithCancel(context.Background())
		wg.Go(func() {
			output, err := transformer.Transform(contexts[index], nil)
			if err != nil {
				t.Errorf("Transform(%d) error = %v", index, err)
				return
			}
			outputs[index] = output
		})
	}
	wg.Wait()
	defer cancels[1]()

	firstChunks := make([]*genx.MessageChunk, len(outputs))
	for index, output := range outputs {
		if output == nil {
			t.Fatalf("Transform(%d) returned nil output", index)
		}
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("output %d initial Next() error = %v", index, err)
		}
		firstChunks[index] = chunk
	}
	if firstChunks[0].Ctrl.StreamID == firstChunks[1].Ctrl.StreamID {
		t.Fatalf("concurrent invocations shared StreamID %q", firstChunks[0].Ctrl.StreamID)
	}

	cancels[0]()
	interrupted, err := outputs[0].Next()
	if err != nil {
		t.Fatalf("cancelled output Next() error = %v", err)
	}
	if interrupted.Ctrl == nil || !interrupted.Ctrl.EndOfStream || interrupted.Ctrl.Error != context.Canceled.Error() {
		t.Fatalf("cancelled output EOS = %#v", interrupted.Ctrl)
	}

	close(release)
	continued, err := outputs[1].Next()
	if err != nil {
		t.Fatalf("independent output Next() error = %v", err)
	}
	if continued.Part != genx.Text("continued") {
		t.Fatalf("independent output part = %#v", continued.Part)
	}
	if _, err := outputs[1].Next(); err != nil {
		t.Fatalf("independent output EOS error = %v", err)
	}
	if _, err := outputs[1].Next(); err != io.EOF {
		t.Fatalf("independent output terminal error = %v, want EOF", err)
	}
}

type invocationTransformer struct {
	release <-chan struct{}
}

func (t invocationTransformer) Transform(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
	invocation := NewInvocation(ctx, OutputConfig{})
	response, err := invocation.StartResponse(ResponseConfig{Role: genx.RoleUser, Label: "source"}, "text/plain")
	if err != nil {
		return nil, err
	}
	if err := invocation.Emit(response, &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("initial")}); err != nil {
		return nil, err
	}
	go func() {
		<-t.release
		if err := invocation.Emit(response, &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("continued")}); err != nil {
			_ = invocation.Close()
			return
		}
		_ = invocation.FinishResponse(response, "")
		_ = invocation.Close()
	}()
	return invocation.Output(), nil
}
