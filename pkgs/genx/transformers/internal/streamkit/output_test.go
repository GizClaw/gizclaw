package streamkit

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestOutputGrowsWithoutDownstreamPull(t *testing.T) {
	output := NewOutput(OutputConfig{InitialCapacity: 1})
	producerDone := make(chan error, 1)
	go func() {
		for _, text := range []string{"one", "two", "three"} {
			if err := output.Push(&genx.MessageChunk{Part: genx.Text(text)}); err != nil {
				producerDone <- err
				return
			}
		}
		producerDone <- output.Close()
	}()

	select {
	case err := <-producerDone:
		if err != nil {
			t.Fatalf("producer error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("producer blocked waiting for pull")
	}

	for _, want := range []genx.Text{"one", "two", "three"} {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if got := chunk.Part.(genx.Text); got != want {
			t.Fatalf("Next() text = %q, want %q", got, want)
		}
	}
	if _, err := output.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("Next() terminal error = %v, want EOF", err)
	}
}

func TestOutputLimitIsObservableAndDiscardsQueue(t *testing.T) {
	output := NewOutput(OutputConfig{MaxBytes: 4})
	if err := output.Push(&genx.MessageChunk{Part: genx.Text("1234")}); err != nil {
		t.Fatalf("first Push() error = %v", err)
	}
	err := output.Push(&genx.MessageChunk{Part: genx.Text("5")})
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("overflow Push() error = %v, want ErrOutputLimit", err)
	}
	if _, err := output.Next(); !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("Next() error = %v, want ErrOutputLimit", err)
	}
}

func TestOutputDiscardAndPullObservation(t *testing.T) {
	var observed []string
	output := NewOutput(OutputConfig{Observe: func(chunk *genx.MessageChunk) {
		observed = append(observed, string(chunk.Part.(genx.Text)))
	}})
	for _, text := range []string{"keep", "discard", "last"} {
		if err := output.Push(&genx.MessageChunk{Part: genx.Text(text)}); err != nil {
			t.Fatalf("Push(%q) error = %v", text, err)
		}
	}
	if removed := output.Discard(func(chunk *genx.MessageChunk) bool {
		return chunk.Part == genx.Text("discard")
	}); removed != 1 {
		t.Fatalf("Discard() = %d, want 1", removed)
	}
	_ = output.Close()
	for range 2 {
		if _, err := output.Next(); err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}
	if got := observed; len(got) != 2 || got[0] != "keep" || got[1] != "last" {
		t.Fatalf("observed = %v", got)
	}
}

func TestOutputConcurrentInvocationsAreIndependent(t *testing.T) {
	outputs := []*Output{NewOutput(OutputConfig{}), NewOutput(OutputConfig{})}
	var wg sync.WaitGroup
	for i, output := range outputs {
		wg.Add(1)
		go func(i int, output *Output) {
			defer wg.Done()
			_ = output.Push(&genx.MessageChunk{Part: genx.Text(rune('a' + i))})
			_ = output.Close()
		}(i, output)
	}
	wg.Wait()
	for i, output := range outputs {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("output %d Next() error = %v", i, err)
		}
		if got, want := chunk.Part.(genx.Text), genx.Text(rune('a'+i)); got != want {
			t.Fatalf("output %d text = %q, want %q", i, got, want)
		}
	}
}

func TestOutputDeferredObservationAndErrorClose(t *testing.T) {
	var observed []*genx.MessageChunk
	output := NewOutput(OutputConfig{Observe: func(chunk *genx.MessageChunk) {
		observed = append(observed, chunk)
	}})
	output.DeferOutputObservation()
	chunk := &genx.MessageChunk{Part: genx.Text("delivered")}
	if err := output.Push(chunk); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if got, err := output.Next(); err != nil || got != chunk {
		t.Fatalf("Next() = (%#v, %v)", got, err)
	}
	if len(observed) != 0 {
		t.Fatalf("automatic observations = %d, want 0", len(observed))
	}
	output.ObserveOutput(chunk)
	if len(observed) != 1 || observed[0] != chunk {
		t.Fatalf("observed = %#v", observed)
	}
	wantErr := errors.New("provider failed")
	if err := output.CloseWithError(wantErr); err != nil {
		t.Fatalf("CloseWithError() error = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, wantErr) {
		t.Fatalf("Next() error = %v, want provider error", err)
	}
	if err := output.Push(chunk); !errors.Is(err, wantErr) {
		t.Fatalf("Push() error = %v, want provider error", err)
	}
	select {
	case <-output.Done():
	default:
		t.Fatal("Done() remained open")
	}
}

func TestOutputCloseRejectsNewChunksAfterDraining(t *testing.T) {
	output := NewOutput(OutputConfig{})
	if err := output.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := output.Push(&genx.MessageChunk{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Push() error = %v, want closed pipe", err)
	}
}
