package streamkit

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestResponseTracksMIMERoutesAndInterruptEOS(t *testing.T) {
	response := NewResponse(ResponseConfig{})
	if response.StreamID() == "" {
		t.Fatal("StreamID() is empty")
	}
	if !response.Declare("application/json") || response.Declare("application/json") {
		t.Fatal("Declare() did not reject a duplicate route")
	}
	text := &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: response.StreamID()}}
	audio := &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus; rate=24000", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: response.StreamID()}}
	if !response.Accept(text) || !response.Accept(audio) {
		t.Fatal("response rejected initial routes")
	}
	textEOS := &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: response.StreamID(), EndOfStream: true}}
	if !response.Accept(textEOS) {
		t.Fatal("response rejected text EOS")
	}

	interrupted := response.End("interrupted")
	if len(interrupted) != 2 {
		t.Fatalf("End() chunks = %d, want open audio and JSON routes", len(interrupted))
	}
	chunk := interrupted[0]
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "application/json" {
		t.Fatalf("End() part = %#v", chunk.Part)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID != response.StreamID() || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "interrupted" {
		t.Fatalf("End() ctrl = %#v", chunk.Ctrl)
	}
	if response.Accept(audio) {
		t.Fatal("response accepted late audio")
	}
}

func TestResponseCompleteRequiresEveryDeclaredMIMERoute(t *testing.T) {
	response := NewResponse(ResponseConfig{StreamID: "response"})
	textBOS := &genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{BeginOfStream: true}}
	audioBOS := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{BeginOfStream: true}}
	textEOS := &genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{EndOfStream: true}}
	audioEOS := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{EndOfStream: true}}
	for _, chunk := range []*genx.MessageChunk{textBOS, audioBOS, textEOS} {
		if !response.Accept(chunk) {
			t.Fatalf("Accept(%#v) = false", chunk)
		}
	}
	if response.Complete() {
		t.Fatal("response completed while its audio route remained open")
	}
	if !response.Accept(audioEOS) {
		t.Fatal("Accept(audio EOS) = false")
	}
	if !response.Complete() {
		t.Fatal("response did not complete after every declared route ended")
	}
}

func TestResponseUsesFreshIDsAndRejectsCrossResponseChunks(t *testing.T) {
	first := NewResponse(ResponseConfig{})
	second := NewResponse(ResponseConfig{})
	if first.StreamID() == second.StreamID() {
		t.Fatalf("fresh responses shared StreamID %q", first.StreamID())
	}
	if second.Accept(&genx.MessageChunk{Part: genx.Text("late"), Ctrl: &genx.StreamCtrl{StreamID: first.StreamID()}}) {
		t.Fatal("second response accepted first response chunk")
	}
}

func TestResponseControlEOSClosesAllRoutes(t *testing.T) {
	response := NewResponse(ResponseConfig{StreamID: "response"})
	response.Declare("text/plain")
	response.Declare("audio/pcm")
	if !response.Accept(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "response", EndOfStream: true}}) {
		t.Fatal("response rejected control EOS")
	}
	if got := response.End("interrupted"); len(got) != 0 {
		t.Fatalf("End() after control EOS returned %d chunks", len(got))
	}
}

func TestResponseWithoutRoutesEndsWithControlEOS(t *testing.T) {
	response := NewResponse(ResponseConfig{StreamID: "response"})
	chunks := response.End("provider failed")
	if len(chunks) != 1 || chunks[0].Part != nil {
		t.Fatalf("End() chunks = %#v, want one control EOS", chunks)
	}
	if ctrl := chunks[0].Ctrl; ctrl == nil || ctrl.StreamID != "response" || !ctrl.EndOfStream || ctrl.Error != "provider failed" {
		t.Fatalf("End() ctrl = %#v", chunks[0].Ctrl)
	}
}

func TestResponseEpochEndsOnlyAfterEveryDeclaredRoute(t *testing.T) {
	epoch := genx.NewResponseEpoch("input-owner")
	response := NewResponse(ResponseConfig{StreamID: "response", ResponseEpoch: epoch})
	response.Declare("text/plain")
	response.Declare("audio/opus")
	textEOS := response.applyMetadata(&genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{EndOfStream: true}})
	if !response.Accept(textEOS) {
		t.Fatal("Accept(text EOS) = false")
	}
	response.markEpochEnd(textEOS)
	if textEOS.Ctrl.ResponseEpoch != epoch || textEOS.Ctrl.ResponseEpochEnd {
		t.Fatalf("text EOS provenance = %#v", textEOS.Ctrl)
	}
	audioEOS := response.applyMetadata(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{EndOfStream: true}})
	if !response.Accept(audioEOS) {
		t.Fatal("Accept(audio EOS) = false")
	}
	response.markEpochEnd(audioEOS)
	if audioEOS.Ctrl.ResponseEpoch != epoch || !audioEOS.Ctrl.ResponseEpochEnd {
		t.Fatalf("audio EOS provenance = %#v", audioEOS.Ctrl)
	}
}

func TestResponseEpochOverridesSourceMetadataAndMarksSynthesizedTerminal(t *testing.T) {
	epoch := genx.NewResponseEpoch("input-owner")
	foreign := genx.NewResponseEpoch("foreign-input")
	response := NewResponse(ResponseConfig{StreamID: "response", ResponseEpoch: epoch})
	chunk := response.applyMetadata(&genx.MessageChunk{Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{ResponseEpoch: foreign}})
	if chunk.Ctrl.ResponseEpoch != epoch || chunk.Ctrl.ResponseEpochEnd {
		t.Fatalf("applied provenance = %#v", chunk.Ctrl)
	}
	terminals := response.End("interrupted")
	if len(terminals) != 1 || terminals[0].Ctrl.ResponseEpoch != epoch || !terminals[0].Ctrl.ResponseEpochEnd {
		t.Fatalf("synthesized terminal = %#v", terminals)
	}
}
