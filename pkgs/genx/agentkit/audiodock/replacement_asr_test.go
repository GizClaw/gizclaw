package audiodock

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestDockASRReplacementClosesOldAudioBeforeNewReply(t *testing.T) {
	testDockReplacementTerminatesReply(t, false)
}

func TestDockASRReplacesInitiativeAndTerminatesReply(t *testing.T) {
	testDockReplacementTerminatesReply(t, true)
}

func testDockReplacementTerminatesReply(t *testing.T, initiative bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	input := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
	asrOutput := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 8})
	defer asrOutput.Close()
	agent := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 8})
		go func() {
			defer output.Close()
			if initiative {
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleModel, Name: "answer", Part: genx.Text("opening reply"),
					Ctrl: &genx.StreamCtrl{StreamID: "opening", Label: "assistant", BeginOfStream: true, EndOfStream: true},
				})
			}
			var text string
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				if chunk == nil {
					continue
				}
				if part, ok := chunk.Part.(genx.Text); ok {
					text += string(part)
				}
				if !chunk.IsEndOfStream() {
					continue
				}
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleModel, Name: "answer", Part: genx.Text(text),
					Ctrl: &genx.StreamCtrl{StreamID: "reply-" + chunk.Ctrl.StreamID, Label: "assistant", BeginOfStream: true, EndOfStream: true},
				})
				text = ""
			}
		}()
		return output, nil
	})
	var calls atomic.Int32
	dock, err := New(Config{
		Agent: agent,
		ASR: transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
			return asrOutput, nil
		}),
		ResolveVoice: fixedVoice("voice"),
		TTS: muxFunc(func(ctx context.Context, _ string, _ genx.Stream) (genx.Stream, error) {
			call := calls.Add(1)
			output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
			go func() {
				defer output.Close()
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{byte(call)}},
					Ctrl: &genx.StreamCtrl{StreamID: "tts", BeginOfStream: true},
				})
				if call == 1 {
					<-ctx.Done()
					return
				}
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm"},
					Ctrl: &genx.StreamCtrl{StreamID: "tts", EndOfStream: true},
				})
			}()
			return output, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	pushTranscript := func(id, text string) {
		t.Helper()
		for _, chunk := range []*genx.MessageChunk{
			{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "transcript", BeginOfStream: true}},
			{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "transcript", EndOfStream: true}},
		} {
			if err := asrOutput.Push(chunk); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !initiative {
		pushTranscript("old", "old reply")
	}
	oldID := ""
	for oldID == "" {
		chunk, err := output.Next()
		if err != nil {
			t.Fatal(err)
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			oldID = chunk.Ctrl.StreamID
		}
	}
	// A continuous audio input produces a new ASR utterance without another
	// caller audio BOS. Its text BOS must still interrupt the old TTS route.
	pushTranscript("new", "new reply")
	oldAudioEnded := false
	newID := ""
	newTextEnded, newAudioEnded := false, false
	for !newTextEnded || !newAudioEnded {
		chunk, err := output.Next()
		if err != nil {
			t.Fatal(err)
		}
		if chunk.Ctrl.StreamID == oldID && chunk.IsEndOfStream() {
			if _, ok := chunk.Part.(*genx.Blob); ok {
				oldAudioEnded = true
			}
		}
		if text, ok := chunk.Part.(genx.Text); ok && chunk.Role == genx.RoleModel && strings.Contains(string(text), "new reply") {
			if !oldAudioEnded {
				t.Fatal("new reply started before the old audio terminal")
			}
			newID = chunk.Ctrl.StreamID
		}
		if newID != "" && chunk.Ctrl.StreamID == newID && chunk.IsEndOfStream() {
			if chunk.Ctrl.Error != "" {
				t.Fatalf("replacement reply failed: %s", chunk.Ctrl.Error)
			}
			switch chunk.Part.(type) {
			case genx.Text:
				newTextEnded = true
			case *genx.Blob:
				newAudioEnded = true
			}
		}
	}
}
