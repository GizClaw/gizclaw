package agentkit

import (
	"errors"
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
