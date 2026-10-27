package agenthost

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestHistoryInterruptSupersedesUndeliveredDurableTerminal(t *testing.T) {
	for _, mimeType := range []string{"audio/opus", "text/plain"} {
		for _, control := range []bool{false, true} {
			for _, production := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/control=%t/production=%t", mimeType, control, production), func(t *testing.T) {
					testHistoryInterruptedDurableTerminal(t, mimeType, control, production)
				})
			}
		}
	}
}

func TestCaptureHistoryCancelsQueuedSuccessfulEOSOnReplacement(t *testing.T) {
	for _, tc := range []struct{ delivered, control bool }{{false, false}, {false, true}, {true, false}, {true, true}} {
		t.Run(fmt.Sprintf("delivered=%t/control=%t", tc.delivered, tc.control), func(t *testing.T) {
			history := newTestWorkspaceHistory(t, newTestObjectStore(t))
			input, source := NewInputStream(16), NewInputStream(16)
			upstream := newRecordingObservationStream(source)
			upstream.abandoned = make(chan *genx.MessageChunk, 16)
			gate := newCaptureOutput()
			agent := wrapHistoryAgent(captureOutputAgent{Agent: historyTestAgent{output: upstream}, mode: apitypes.WorkspaceInputModeRealtime}, history)
			out, err := agent.Transform(context.WithValue(t.Context(), captureOutputKey{}, gate), input)
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			output := out.(*historyOutputStream)
			output.DeferOutputObservation()
			produced := make(chan struct{})
			ready := make(chan struct{})
			output.SetOutputProductionObserver(func(chunk *genx.MessageChunk) {
				if chunk.Ctrl.StreamID == "response" && chunk.IsEndOfStream() && !historyInterruptedChunk(chunk) {
					close(produced)
				}
				if chunk.Ctrl.StreamID == "replacement-response" && chunk.IsBeginOfStream() {
					close(ready)
				}
			})
			admitCapture(t, gate, input, captureTestInput("first", "realtime", false))
			first, _ := input.Next()
			body := captureTestChunk(first.Ctrl.ResponseEpoch, "first")
			body.Ctrl.BeginOfStream = true
			eos := captureTestChunk(first.Ctrl.ResponseEpoch, "")
			eos.Ctrl.EndOfStream = true
			if tc.control {
				eos.Part = nil
			}
			_ = source.Push(t.Context(), body)
			_ = source.Push(t.Context(), eos)
			got, err := output.Next()
			if err != nil || !got.IsBeginOfStream() {
				t.Fatalf("first body=%v, %v", got, err)
			}
			output.ObserveOutput(got)
			waitForHistoryAppend(t, produced)
			inserted := make(chan struct{})
			if err := output.output.recorder.persistence.forwardAfterWrites(func() error { close(inserted); return nil }); err != nil {
				t.Fatal(err)
			}
			waitForHistoryAppend(t, inserted)
			if tc.delivered {
				got, err := output.Next()
				if err != nil || !got.IsEndOfStream() || historyInterruptedChunk(got) {
					t.Fatalf("normal terminal=%v, %v", got, err)
				}
				output.ObserveOutput(got)
			}
			admitCapture(t, gate, input, captureTestInput("replacement", "realtime", false))
			current, _ := input.Next()
			marker := captureTestChunk(current.Ctrl.ResponseEpoch, "replacement")
			marker.Ctrl.StreamID, marker.Ctrl.BeginOfStream = "replacement-response", true
			_ = source.Push(t.Context(), marker)
			// Ensure the interruption has crossed History before reading the
			// buffer; the prior successful EOS must still be cancellable here.
			waitForHistoryAppend(t, ready)
			got, err = output.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !tc.delivered {
				if !historyInterruptedChunk(got) || got.Ctrl.StreamID != "response" {
					t.Fatalf("queued success survived replacement: %v", got)
				}
				output.ObserveOutput(got)
				got, err = output.Next()
			}
			if err != nil || got.Ctrl.StreamID != "replacement-response" || !got.IsBeginOfStream() {
				t.Fatalf("replacement body=%v, %v", got, err)
			}
			output.ObserveOutput(got)
		})
	}
}

func testHistoryInterruptedDurableTerminal(t *testing.T, mimeType string, control, production bool) {
	t.Helper()
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	storeRelease := release
	if production {
		storeRelease = make(chan struct{})
		close(storeRelease)
	}
	store := &gatedOutputHistoryStore{HistoryStore: history.Records, entered: make(chan struct{}), release: storeRelease}
	history.Records = store
	chunk := func(begin, end bool, cause string) *genx.MessageChunk {
		var part genx.Part = genx.Text("")
		if begin {
			part = genx.Text("answer")
		}
		if mimeType == "audio/opus" {
			blob := &genx.Blob{MIMEType: mimeType}
			if begin {
				blob.Data = []byte{0xf8, 0xff, 0xfe}
			}
			part = blob
		}
		return &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: part, Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", BeginOfStream: begin, EndOfStream: end, Error: cause}}
	}
	question := &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("question"), Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript", BeginOfStream: true, EndOfStream: true}}
	successful := chunk(false, true, "")
	interrupt := chunk(false, true, "interrupted")
	if control {
		interrupt.Part = nil
	}
	builder := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	upstream := newRecordingObservationStream(builder.Stream())
	upstream.abandoned = make(chan *genx.MessageChunk, 16)
	agent := wrapHistoryAgent(historyTestAgent{output: upstream}, history)
	output, err := agent.Transform(t.Context(), historyStreamFromChunks())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		unblock()
		_ = output.Close()
	}()
	entered := store.entered
	if production {
		entered = make(chan struct{})
		if !output.(OutputProductionObserver).SetOutputProductionObserver(func(c *genx.MessageChunk) {
			if c.Ctrl != nil && c.Ctrl.StreamID == "reply" && c.IsEndOfStream() && !historyInterruptedChunk(c) {
				close(entered)
				<-release
			}
		}) {
			t.Fatal("production observer unavailable")
		}
	}
	if err := builder.Add(question, chunk(true, false, ""), successful, interrupt); err != nil {
		t.Fatal(err)
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	interrupted := make(chan struct{})
	result := make(chan int, 1)
	go func() {
		count := 0
		for {
			c, err := output.Next()
			if c != nil && c.Ctrl != nil && c.Ctrl.StreamID == "reply" && c.IsEndOfStream() {
				count++
				if historyInterruptedChunk(c) {
					close(interrupted)
				}
			}
			if err != nil {
				result <- count
				return
			}
		}
	}()
	waitForHistoryAppend(t, entered)
	select {
	case <-interrupted:
	case <-time.After(time.Second):
		t.Fatal("interruption waited for History persistence or production callback")
	}
	select {
	case abandoned := <-upstream.abandoned:
		if abandoned != successful {
			t.Fatal("abandoned a different upstream chunk")
		}
	case <-time.After(time.Second):
		t.Fatal("interruption retained the pending successful EOS observation")
	}
	unblock()
	select {
	case count := <-result:
		if count != 1 {
			t.Fatalf("terminal count=%d, want one interruption and no late successful EOS", count)
		}
	case <-time.After(time.Second):
		t.Fatal("output did not finish")
	}
	for len(upstream.observed) > 0 {
		if <-upstream.observed == successful {
			t.Fatal("superseded EOS was observed as delivered")
		}
	}
}
