package agentkit

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestResponseStreamAssignsFreshIDsPerProviderResponse(t *testing.T) {
	source := NewOutput(OutputConfig{})
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: genx.Text("transcript"), Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleModel, Part: genx.Text("next"), Ctrl: &genx.StreamCtrl{StreamID: "turn-2"}},
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
	chunks := make([]*genx.MessageChunk, 0, 4)
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
	if chunks[0].Ctrl.StreamID != "turn-1" {
		t.Fatalf("user StreamID = %q, want turn-1", chunks[0].Ctrl.StreamID)
	}
	firstResponseID := chunks[1].Ctrl.StreamID
	if firstResponseID == "" || firstResponseID == "turn-1" {
		t.Fatalf("first response StreamID = %q", firstResponseID)
	}
	if chunks[2].Ctrl.StreamID != firstResponseID {
		t.Fatalf("audio StreamID = %q, want shared %q", chunks[2].Ctrl.StreamID, firstResponseID)
	}
	if chunks[3].Ctrl.StreamID == "" || chunks[3].Ctrl.StreamID == "turn-2" || chunks[3].Ctrl.StreamID == firstResponseID {
		t.Fatalf("second response StreamID = %q", chunks[3].Ctrl.StreamID)
	}
}

func TestResponseStreamPreservesInterruptedResponseID(t *testing.T) {
	source := NewOutput(OutputConfig{})
	_ = source.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("prefix"), Ctrl: &genx.StreamCtrl{StreamID: "turn"}})
	_ = source.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true, Error: "interrupted"}})
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	prefix, _ := stream.Next()
	eos, _ := stream.Next()
	if prefix.Ctrl.StreamID != eos.Ctrl.StreamID || eos.Ctrl.Error != "interrupted" {
		t.Fatalf("prefix/EOS controls = %#v / %#v", prefix.Ctrl, eos.Ctrl)
	}
}

func TestResponseStreamRotatesWhenProviderReusesCompletedRoute(t *testing.T) {
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
		t.Fatalf("reused provider response kept StreamID %q", second.Ctrl.StreamID)
	}
}

func TestResponseStreamKeepsLateMIMEBOSWithCompletedTextRoute(t *testing.T) {
	source := NewOutput(OutputConfig{})
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "reused"}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reused", EndOfStream: true}},
		{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "reused", BeginOfStream: true}},
	} {
		_ = source.Push(chunk)
	}
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	first, _ := stream.Next()
	_, _ = stream.Next()
	bos, _ := stream.Next()
	if bos.Ctrl.StreamID != first.Ctrl.StreamID {
		t.Fatalf("late MIME BOS ID = %q, want shared %q", bos.Ctrl.StreamID, first.Ctrl.StreamID)
	}
}

func TestResponseStreamForwardsPullVisibleObservation(t *testing.T) {
	var observed *genx.MessageChunk
	source := NewOutput(OutputConfig{Observe: func(chunk *genx.MessageChunk) { observed = chunk }})
	_ = source.Push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("answer"),
		Ctrl: &genx.StreamCtrl{StreamID: "provider-response"},
	})
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	stream.DeferOutputObservation()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if observed != nil {
		t.Fatalf("source observed chunk before final acknowledgement: %#v", observed)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	stream.ObserveOutput(chunk.Clone())
	if observed == nil || observed.Ctrl == nil || observed.Ctrl.StreamID != "provider-response" {
		t.Fatalf("forwarded observation = %#v", observed)
	}
	if len(stream.pendingObservations) != 0 {
		t.Fatalf("pending observations after acknowledgement = %d", len(stream.pendingObservations))
	}
}

func TestResponseStreamRotatesCompletedResponseForDifferentMIME(t *testing.T) {
	source := NewOutput(OutputConfig{})
	for _, chunk := range []*genx.MessageChunk{
		{
			Role: genx.RoleModel,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: "reused", EndOfStream: true},
		},
		{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}},
			Ctrl: &genx.StreamCtrl{StreamID: "reused"},
		},
	} {
		_ = source.Push(chunk)
	}
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	textEOS, _ := stream.Next()
	audio, _ := stream.Next()
	if audio.Ctrl.StreamID == textEOS.Ctrl.StreamID {
		t.Fatalf("completed text and new audio reused StreamID %q", audio.Ctrl.StreamID)
	}
}

func TestResponseStreamKeepsSameMIMEWithinActiveMultimodalResponse(t *testing.T) {
	source := NewOutput(OutputConfig{})
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "response", BeginOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("model text"), Ctrl: &genx.StreamCtrl{StreamID: "response"}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "response", EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("tts transcript"), Ctrl: &genx.StreamCtrl{StreamID: "response"}},
	} {
		_ = source.Push(chunk)
	}
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	first, _ := stream.Next()
	_, _ = stream.Next()
	_, _ = stream.Next()
	transcript, _ := stream.Next()
	if transcript.Ctrl.StreamID != first.Ctrl.StreamID {
		t.Fatalf("transcript StreamID = %q, want shared %q", transcript.Ctrl.StreamID, first.Ctrl.StreamID)
	}
}

func TestResponseStreamBoundsCompletedResponseMappings(t *testing.T) {
	source := NewOutput(OutputConfig{})
	for index := range maxRetainedCompletedResponses + 20 {
		streamID := fmt.Sprintf("response-%d", index)
		_ = source.Push(&genx.MessageChunk{
			Role: genx.RoleModel,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true},
		})
	}
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	for {
		if _, err := stream.Next(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}
	if len(stream.responses) > maxRetainedCompletedResponses {
		t.Fatalf("retained completed responses = %d, max = %d", len(stream.responses), maxRetainedCompletedResponses)
	}
}

func TestResponseStreamReleasesControlTerminalMapping(t *testing.T) {
	source := NewOutput(OutputConfig{})
	_ = source.Push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Ctrl: &genx.StreamCtrl{StreamID: "terminal", EndOfStream: true},
	})
	_ = source.Close()
	stream, _ := NewResponseStream(source)
	if _, err := stream.Next(); err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if len(stream.responses) != 0 {
		t.Fatalf("response mappings after terminal EOS = %d", len(stream.responses))
	}
}
