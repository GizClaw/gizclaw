package asttranslate

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestInterruptibleOutputStreamsAvailableTextAndAudioWithoutDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		output := newInterruptibleOutput()
		defer output.Close()
		chunks := []*genx.MessageChunk{
			{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Part: genx.Text("Hello"), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant"}},
			{Role: genx.RoleModel, Part: genx.Text(", world."), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant"}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant"}},
		}
		start := time.Now()
		for _, want := range chunks {
			if err := output.push(want); err != nil {
				t.Fatal(err)
			}
			got, err := output.Next()
			if err != nil || got != want {
				t.Fatalf("Next() = %v, %v; want available chunk %v", got, err, want)
			}
			if elapsed := time.Since(start); elapsed != 0 {
				t.Fatalf("available AST output was delayed by %s before response EOS", elapsed)
			}
		}
	})
}

func TestObservedInputNormalEOFSurvivesProviderCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := streamkit.NewOutput(streamkit.OutputConfig{})
		_ = source.Close()
		observed := newObservedInputStream(context.Background(), source, nil)
		synctest.Wait()
		_ = observed.CloseWithError(context.Canceled)
		if chunk, err := observed.Next(); chunk != nil || !errors.Is(err, io.EOF) || !observed.closedNormally() {
			t.Fatalf("provider cleanup replaced normal input EOF: chunk=%v err=%v normal=%v", chunk, err, observed.closedNormally())
		}
	})
}

func TestInterruptibleTransformerCloseCancelsSilentProvider(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := streamkit.NewOutput(streamkit.OutputConfig{})
		inner := streamkit.NewOutput(streamkit.OutputConfig{})
		var providerContext context.Context
		transformer := interruptibleTransformer{Transformer: transformFunc(func(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
			providerContext = ctx
			context.AfterFunc(ctx, func() { _ = inner.CloseWithError(context.Cause(ctx)) })
			return inner, nil
		})}
		output, err := transformer.Transform(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		_ = output.Close()
		synctest.Wait()
		if !errors.Is(context.Cause(providerContext), context.Canceled) {
			t.Fatal("closing silent AST output did not cancel provider work")
		}
	})
}

func TestInterruptibleOutputRejectsOldEpochFirstObservedAfterReplacement(t *testing.T) {
	output := newInterruptibleOutput()
	defer output.Close()
	output.interrupt("input-1")
	oldEpoch := genx.NewResponseEpoch("input-1")
	output.interrupt("input-2")
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "late", Label: "assistant", BeginOfStream: true, ResponseEpoch: oldEpoch}},
		{Role: genx.RoleModel, Part: genx.Text("stale"), Ctrl: &genx.StreamCtrl{StreamID: "late", Label: "assistant", ResponseEpoch: oldEpoch}},
	} {
		if err := output.push(chunk); err != nil {
			t.Fatal(err)
		}
	}
	next := &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "replacement", Label: "assistant", BeginOfStream: true, ResponseEpoch: genx.NewResponseEpoch("input-2")}}
	if err := output.push(next); err != nil {
		t.Fatal(err)
	}
	if got, err := output.Next(); err != nil || got != next {
		t.Fatalf("late old response escaped after replacement input: %v, %v", got, err)
	}
}

func TestInterruptibleOutputPreservesEpochOnInterruption(t *testing.T) {
	output := newInterruptibleOutput()
	defer output.Close()
	output.interrupt("input-1")
	epoch := genx.NewResponseEpoch("input-1")
	chunk := &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "response", Label: "assistant", BeginOfStream: true, ResponseEpoch: epoch}}
	if err := output.push(chunk); err != nil {
		t.Fatal(err)
	}
	output.DeferOutputObservation()
	if _, err := output.Next(); err != nil {
		t.Fatal(err)
	}
	output.interrupt("input-2")
	for _, terminal := range output.queue {
		if terminal.Ctrl.ResponseEpoch != epoch {
			t.Fatalf("interruption lost old response ownership: %v", terminal)
		}
	}
}
