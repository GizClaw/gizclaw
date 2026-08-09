package doubaorealtime

import (
	"bytes"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestSharedAssistantLifecycleFacade(t *testing.T) {
	state := NewSharedAssistantLifecycle()
	if got := state.CurrentEpoch(); got != 1 {
		t.Fatalf("CurrentEpoch() = %d, want 1", got)
	}
	if !state.AcceptsOutput() || !state.CanPush(1) {
		t.Fatal("new lifecycle rejected its initial epoch")
	}

	epoch := state.MarkStarted(" response ")
	pushed := 0
	accepted, err := state.PushIfCurrent(epoch, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "response", Label: doubaoRealtimeAssistantLabel, BeginOfStream: true},
	}, func() error {
		pushed++
		return nil
	})
	if err != nil || !accepted || pushed != 1 {
		t.Fatalf("PushIfCurrent() = (%t, %v), pushes = %d", accepted, err, pushed)
	}
	wantErr := errors.New("push failed")
	accepted, err = state.PushIfCurrent(epoch, &genx.MessageChunk{}, func() error { return wantErr })
	if !accepted || !errors.Is(err, wantErr) {
		t.Fatalf("PushIfCurrent(error) = (%t, %v), want accepted error", accepted, err)
	}

	interruption := state.InterruptRoutes("fallback", false)
	if !interruption.Interrupted || interruption.StreamID != "response" ||
		!interruption.TextOpen || !interruption.AudioOpen ||
		!interruption.TextStarted || interruption.AudioStarted {
		t.Fatalf("InterruptRoutes() = %#v", interruption)
	}
	if state.AcceptsOutput() || state.CanPush(epoch) {
		t.Fatal("interrupted lifecycle still accepted stale output")
	}
	if accepted, err := state.PushIfCurrent(epoch, &genx.MessageChunk{}, func() error {
		pushed++
		return nil
	}); accepted || err != nil || pushed != 1 {
		t.Fatalf("stale PushIfCurrent() = (%t, %v), pushes = %d", accepted, err, pushed)
	}

	state.SetAccept(true)
	next := state.NextEpoch()
	if got := state.MarkStarted("next"); got != next {
		t.Fatalf("MarkStarted() epoch = %d, want %d", got, next)
	}
	state.MarkRouteStarted(next, true)
	state.MarkRouteStarted(next, false)
	state.MarkRouteDone("wrong", true)
	ObserveSharedAssistantOutput(state, doubaoRealtimeAssistantLabel, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "next", Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
	})
	if got := state.InterruptRoutes("", false); !got.Interrupted || got.StreamID != "next" ||
		got.TextOpen || !got.AudioOpen || !got.TextStarted || !got.AudioStarted {
		t.Fatalf("partially completed InterruptRoutes() = %#v", got)
	}

	state.SetAccept(true)
	state.MarkStarted("complete")
	state.MarkRouteStarted(state.CurrentEpoch(), true)
	state.MarkRouteStarted(state.CurrentEpoch(), false)
	state.MarkRouteDone("complete", true)
	ObserveSharedAssistantOutput(state, doubaoRealtimeAssistantLabel, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "complete", Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
	})
	if got := state.InterruptRoutes("complete", false); got.Interrupted {
		t.Fatalf("completed lifecycle interrupted = %#v", got)
	}

	if streamID, interrupted := state.Interrupt(" forced ", true); !interrupted || streamID != "complete" {
		t.Fatalf("Interrupt(force) = (%q, %t)", streamID, interrupted)
	}
	if accepted, err := state.PushIfCurrent(state.CurrentEpoch(), nil, func() error { return nil }); accepted || err != nil {
		t.Fatalf("PushIfCurrent(nil) = (%t, %v)", accepted, err)
	}
	ObserveSharedAssistantOutput(nil, doubaoRealtimeAssistantLabel, nil)
}

func TestSharedStreamIDsFacade(t *testing.T) {
	ids := NewSharedStreamIDs()
	if got := ids.Input(); got != "audio:rt:1" {
		t.Fatalf("default Input() = %q", got)
	}
	ids.BeginInput(" turn ")
	if got := ids.Input(); got != "turn:rt:1" {
		t.Fatalf("Input() = %q", got)
	}
	explicit := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "explicit"}}
	if got := ids.ServiceInput(explicit); got != "explicit" {
		t.Fatalf("ServiceInput() = %q", got)
	}
	if got := ids.EndInputSegment(); got != "turn:rt:1" {
		t.Fatalf("EndInputSegment() = %q", got)
	}
	if got := ids.Response(); got != "turn:rt:1" {
		t.Fatalf("Response() = %q", got)
	}
	if got := ids.Input(); got != "turn:rt:2" {
		t.Fatalf("next Input() = %q", got)
	}
	if got := SharedChunkInputStreamID(nil, " fallback "); got != " fallback " {
		t.Fatalf("SharedChunkInputStreamID() = %q", got)
	}
}

func TestSharedAudioFacade(t *testing.T) {
	input := NewSharedAudioInput("", 0, 0, false)
	if got := input.Format(); got != "pcm" {
		t.Fatalf("Format() = %q", got)
	}
	if got, err := input.Prepare(nil); err != nil || got != nil {
		t.Fatalf("Prepare(nil) = (%v, %v)", got, err)
	}
	want := []byte{1, 0, 2, 0}
	if got, err := input.Prepare(&genx.Blob{MIMEType: "audio/pcm", Data: want}); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("Prepare(PCM) = (%v, %v)", got, err)
	}
	if got, err := input.PrepareFrames(&genx.Blob{MIMEType: "audio/pcm", Data: want}); err != nil || len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("PrepareFrames(PCM) = (%v, %v)", got, err)
	}
	input.Close()
	input.Close()

	inputs := NewSharedAudioInputs("pcm", 16000, 1, false)
	first := inputs.Stream("")
	if first != inputs.Stream("default") {
		t.Fatal("Stream() did not reuse the canonical default route")
	}
	fromBlob, err := inputs.StreamForBlob("", &genx.Blob{MIMEType: " Audio/PCM "})
	if err != nil || fromBlob != first {
		t.Fatalf("StreamForBlob() = (%p, %v), want %p", fromBlob, err, first)
	}
	if _, err := inputs.StreamForBlob("", &genx.Blob{MIMEType: "audio/mpeg"}); err == nil {
		t.Fatal("StreamForBlob() accepted a MIME change")
	}
	inputs.CloseStream("")
	if next := inputs.Stream("default"); next == first {
		t.Fatal("CloseStream() retained the closed input")
	}
	inputs.Close()
	inputs.Close()

	if SharedAudioInputEOS(nil) {
		t.Fatal("SharedAudioInputEOS(nil) = true")
	}
	if !SharedAudioInputEOS(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{EndOfStream: true}}) {
		t.Fatal("SharedAudioInputEOS(route EOS) = false")
	}
	if got := SharedAudioFormat(" "); got != "pcm" {
		t.Fatalf("SharedAudioFormat() = %q", got)
	}
	if got := SharedAudioSampleRate(0); got != 16000 {
		t.Fatalf("SharedAudioSampleRate() = %d", got)
	}
	if got := SharedPCM16LE([]int16{0x0102, -1}); !bytes.Equal(got, []byte{0x02, 0x01, 0xff, 0xff}) {
		t.Fatalf("SharedPCM16LE() = %v", got)
	}
}
