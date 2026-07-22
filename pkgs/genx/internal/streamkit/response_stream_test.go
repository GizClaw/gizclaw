package streamkit

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestResponseStreamAssignsFreshIDsToModelOutput(t *testing.T) {
	source := NewOutput(OutputConfig{})
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: genx.Text("transcript"), Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
	} {
		if err := source.Push(chunk); err != nil {
			t.Fatalf("Push() error = %v", err)
		}
	}
	_ = source.Close()
	stream, err := NewResponseStream(source)
	if err != nil {
		t.Fatalf("NewResponseStream() error = %v", err)
	}
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
	if chunks[0].Ctrl.StreamID != "turn-1" || chunks[1].Ctrl.StreamID != "turn-1" {
		t.Fatalf("user route IDs = %q / %q", chunks[0].Ctrl.StreamID, chunks[1].Ctrl.StreamID)
	}
	responseID := chunks[2].Ctrl.StreamID
	if responseID == "" || responseID == "turn-1" {
		t.Fatalf("model response ID = %q", responseID)
	}
	if chunks[3].Ctrl.StreamID != responseID {
		t.Fatalf("model audio ID = %q, want %q", chunks[3].Ctrl.StreamID, responseID)
	}
}

func TestResponseStreamRotatesReusedCompletedRoute(t *testing.T) {
	source := NewOutput(OutputConfig{})
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "reused"}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reused", EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "reused"}},
	} {
		_ = source.Push(chunk)
	}
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	first, _ := stream.Next()
	firstEOS, _ := stream.Next()
	second, _ := stream.Next()
	if first.Ctrl.StreamID != firstEOS.Ctrl.StreamID {
		t.Fatalf("first response IDs = %q and %q", first.Ctrl.StreamID, firstEOS.Ctrl.StreamID)
	}
	if second.Ctrl.StreamID == first.Ctrl.StreamID {
		t.Fatalf("reused provider route kept response ID %q", second.Ctrl.StreamID)
	}
}

func TestResponseStreamKeepsInterruptedRoutesOnResponseID(t *testing.T) {
	source := NewOutput(OutputConfig{})
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true, Error: "interrupted"}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true, Error: "interrupted"}},
	} {
		if err := source.Push(chunk); err != nil {
			t.Fatalf("Push() error = %v", err)
		}
	}
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	var responseID string
	for i := 0; i < 5; i++ {
		chunk, err := stream.Next()
		if err != nil {
			t.Fatalf("Next(%d) error = %v", i, err)
		}
		if i == 0 {
			responseID = chunk.Ctrl.StreamID
		}
		if chunk.Ctrl.StreamID != responseID {
			t.Fatalf("chunk %d StreamID = %q, want %q", i, chunk.Ctrl.StreamID, responseID)
		}
	}
}

func TestResponseStreamForwardsPullObservationWithUpstreamID(t *testing.T) {
	var observed *genx.MessageChunk
	source := NewOutput(OutputConfig{Observe: func(chunk *genx.MessageChunk) { observed = chunk }})
	_ = source.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "provider"}})
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	stream.DeferOutputObservation()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if observed != nil {
		t.Fatalf("observed before acknowledgement: %#v", observed)
	}
	stream.ObserveOutput(chunk.Clone())
	if observed == nil || observed.Ctrl == nil || observed.Ctrl.StreamID != "provider" {
		t.Fatalf("forwarded observation = %#v", observed)
	}
}

func TestResponseStreamAbandonsReadAheadWithoutObservation(t *testing.T) {
	var observed []*genx.MessageChunk
	source := NewOutput(OutputConfig{Observe: func(chunk *genx.MessageChunk) {
		observed = append(observed, chunk)
	}})
	_ = source.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("one"), Ctrl: &genx.StreamCtrl{StreamID: "provider"}})
	_ = source.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("two"), Ctrl: &genx.StreamCtrl{StreamID: "provider"}})
	stream, _ := NewResponseStream(source)
	stream.DeferOutputObservation()
	first, _ := stream.Next()
	second, _ := stream.Next()
	stream.ObserveOutput(first)
	ids := stream.AbandonAllOutputObservations()
	if len(ids) != 1 || ids[0] != second.Ctrl.StreamID {
		t.Fatalf("abandoned response IDs = %#v, want %q", ids, second.Ctrl.StreamID)
	}
	source.WaitForObservers()
	if len(observed) != 1 || observed[0].Part != genx.Text("one") {
		t.Fatalf("observed = %#v, want only delivered first chunk", observed)
	}
}

func TestResponseStreamAbandonAllReleasesUnmappedSourceObservation(t *testing.T) {
	source := NewOutput(OutputConfig{InitialCapacity: 1, Observe: func(*genx.MessageChunk) {}})
	source.DeferOutputObservation()
	if err := source.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("bypass")}); err != nil {
		t.Fatal(err)
	}
	stream, err := NewResponseStream(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Next(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		source.WaitForObservers()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("source observation was not deferred")
	case <-time.After(20 * time.Millisecond):
	}
	if ids := stream.AbandonAllOutputObservations(); len(ids) != 0 {
		t.Fatalf("unmapped response IDs = %v", ids)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("bulk abandonment did not release the unmapped source observation")
	}
}
