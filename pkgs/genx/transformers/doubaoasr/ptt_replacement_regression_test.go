package doubaoasr

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPushToTalkReplacementClosesPreviousHistoryAndTranscript(t *testing.T) {
	for _, withPartial := range []bool{true, false} {
		name := "without-transcript"
		if withPartial {
			name = "with-definite-transcript"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			first, second := newFakeDoubaoASRSession(), newFakeDoubaoASRSession()
			firstSent := make(chan struct{})
			var announced atomic.Bool
			first.sendAudio = func(_ context.Context, data []byte, last bool) error {
				if len(data) > 0 && announced.CompareAndSwap(false, true) {
					if withPartial {
						first.result <- &doubaospeech.ASRV2Result{Text: "first partial", Utterances: []doubaospeech.ASRV2Utterance{{Text: "first partial", EndTime: 100, Definite: true}}}
					}
					close(firstSent)
				}
				return nil
			}
			transformer := newTransformer(Config{Format: "pcm", RealtimePacing: new(false)})
			opener := newFakeDoubaoASROpener(first, second)
			transformer.newSession = opener.open
			input := newBufferStream(8)
			output, err := transformer.Transform(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			push := func(id string, bos, eos bool, data []byte) {
				t.Helper()
				if err := input.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: data}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: bos, EndOfStream: eos}}); err != nil {
					t.Fatal(err)
				}
			}
			push("first", true, false, bytes.Repeat([]byte{1, 0}, 1600))
			select {
			case <-firstSent:
			case <-ctx.Done():
				t.Fatal("first ASR session did not accept audio")
			}
			var chunks []*genx.MessageChunk
			if withPartial {
				for {
					chunk, err := output.Next()
					if err != nil {
						t.Fatal(err)
					}
					chunks = append(chunks, chunk)
					if chunk != nil && chunk.Part == genx.Text("first partial") {
						break
					}
				}
			}
			push("second", true, false, bytes.Repeat([]byte{2, 0}, 1600))
			push("second", false, true, nil)
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			for {
				chunk, err := output.Next()
				if err != nil {
					if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
						t.Fatal(err)
					}
					break
				}
				chunks = append(chunks, chunk)
			}
			firstAudioEOS, firstTextEOS, secondTextEOS := 0, 0, 0
			firstTextEnd, firstAudioEnd, secondStart := -1, -1, -1
			var secondText strings.Builder
			for index, chunk := range chunks {
				if chunk == nil || chunk.Ctrl == nil {
					continue
				}
				if chunk.Ctrl.StreamID == "second" {
					if value, ok := chunk.Part.(genx.Text); ok {
						secondText.WriteString(string(value))
					}
				}
				if chunk.Ctrl.StreamID == "second" && secondStart < 0 {
					secondStart = index
				}
				if !chunk.IsEndOfStream() {
					continue
				}
				if chunk.Ctrl.StreamID == "first" {
					if chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
						firstAudioEOS++
						firstAudioEnd = index
					}
					if chunk.Ctrl.Label == "transcript" {
						firstTextEOS++
						firstTextEnd = index
						if chunk.Ctrl.Error != "interrupted" {
							t.Fatalf("first transcript terminal error=%q", chunk.Ctrl.Error)
						}
					}
				}
				if chunk.Ctrl.StreamID == "second" && chunk.Ctrl.Label == "transcript" {
					secondTextEOS++
					if chunk.Ctrl.Error != "" {
						t.Fatalf("replacement transcript failed: %q", chunk.Ctrl.Error)
					}
				}
			}
			if firstAudioEOS != 1 || firstTextEOS != 1 || secondTextEOS != 1 || firstTextEnd < 0 || firstAudioEnd < 0 || secondStart < 0 || firstTextEnd >= secondStart || firstAudioEnd >= secondStart || secondText.String() != "recognized text" {
				t.Fatalf("incomplete replacement: old audio EOS=%d old text EOS=%d new text EOS=%d old end=%d new start=%d", firstAudioEOS, firstTextEOS, secondTextEOS, firstTextEnd, secondStart)
			}
			if opener.openCount() != 2 {
				t.Fatalf("provider sessions=%d", opener.openCount())
			}
		})
	}
}
