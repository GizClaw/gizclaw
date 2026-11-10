package doubaoasr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestInterruptedAudioEOSKeepsASROuterStreamAlive(t *testing.T) {
	for _, emitInterim := range []bool{false, true} {
		t.Run(fmt.Sprint(emitInterim), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			first, second := newFakeDoubaoASRSession(), newFakeDoubaoASRSession()
			first.recvDone = make(chan struct{})
			first.sendAudio = func(_ context.Context, data []byte, last bool) error {
				first.sends = append(first.sends, fakeDoubaoASRSend{data: slices.Clone(data), isLast: last})
				result := &doubaospeech.ASRV2Result{Text: "first partial"}
				if !emitInterim {
					result.Utterances = []doubaospeech.ASRV2Utterance{{Text: "first partial", EndTime: 100, Definite: true}}
				}
				first.result <- result
				return nil
			}
			first.close = func() error {
				select {
				case first.result <- &doubaospeech.ASRV2Result{Text: "stale first result", IsFinal: true}:
				default:
				}
				return nil
			}
			transformer := newTransformer(Config{Format: "pcm", EmitInterim: emitInterim, RealtimePacing: new(false)})
			opener := newFakeDoubaoASROpener(first, second)
			transformer.newSession = func(ctx context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
				if opener.openCount() == 1 {
					select {
					case <-first.recvDone:
					default:
						return nil, errors.New("replacement opened before old receiver joined")
					}
				}
				return opener.open(ctx, cfg)
			}
			input := newBufferStream(8)
			output, err := transformer.Transform(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			const mimeType = "audio/pcm;rate=24000;channels=1"
			push := func(chunk *genx.MessageChunk) {
				t.Helper()
				if err := input.Push(chunk); err != nil {
					t.Fatal(err)
				}
			}
			// Conversion leaves FIR lookahead and an incomplete provider packet.
			push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: mimeType, Data: bytes.Repeat([]byte{1, 0}, 4800), OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}, Ctrl: &genx.StreamCtrl{StreamID: "first", BeginOfStream: true}})
			var chunks []*genx.MessageChunk
			for {
				chunk, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				chunks = append(chunks, chunk)
				if chunk.Part == genx.Text("first partial") {
					break
				}
			}
			cause := errors.New("interrupted")
			terminal := &genx.MessageChunk{Part: &genx.Blob{MIMEType: mimeType, Data: make([]byte, 640)}, Ctrl: &genx.StreamCtrl{StreamID: "first", EndOfStream: true}}
			genx.SetStreamError(terminal.Ctrl, cause)
			push(terminal)
			push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "first", EndOfStream: true}})
			push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "first", EndOfStream: true}})
			push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: mimeType, Data: bytes.Repeat([]byte{2, 0}, 4800)}, Ctrl: &genx.StreamCtrl{StreamID: "second", BeginOfStream: true}})
			push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: mimeType}, Ctrl: &genx.StreamCtrl{StreamID: "second", EndOfStream: true}})
			_ = input.Close()
			chunks = append(chunks, collectTransformerChunks(t, output)...)
			if opener.openCount() != 2 || len(first.sends) != 1 || first.sends[0].isLast || len(first.sends[0].data) != 3200 {
				t.Fatalf("sessions=%d; failed session sends=%#v", opener.openCount(), first.sends)
			}
			oldTextEOS, oldHistoryEOS, secondTextEOS := 0, 0, 0
			secondStarted := false
			var secondText string
			for _, chunk := range chunks {
				if chunk.Ctrl == nil {
					continue
				}
				if text, ok := chunk.Part.(genx.Text); ok && text == "stale first result" {
					t.Fatal("retired provider result escaped into output")
				}
				if chunk.Ctrl.StreamID == "second" {
					secondStarted = true
					if text, ok := chunk.Part.(genx.Text); ok {
						secondText += string(text)
					}
					if chunk.IsEndOfStream() && chunk.Ctrl.Label == "transcript" {
						secondTextEOS++
						if genx.StreamError(chunk.Ctrl) != nil {
							t.Fatal("replacement transcript failed")
						}
					}
				}
				if chunk.Ctrl.StreamID != "first" || !chunk.IsEndOfStream() || chunk.Ctrl.Label == "" {
					continue
				}
				if secondStarted || !errors.Is(genx.StreamError(chunk.Ctrl), cause) {
					t.Fatalf("old terminal crossed replacement or lost typed cause: %#v", chunk.Ctrl)
				}
				switch chunk.Ctrl.Label {
				case "transcript":
					oldTextEOS++
				case genx.HistoryUserAudioLabel:
					oldHistoryEOS++
					if chunk.Part.(*genx.Blob).MIMEType != "audio/opus" {
						t.Fatal("failed history route lost original MIME")
					}
				}
			}
			if oldTextEOS != 1 || !emitInterim && oldHistoryEOS != 1 || secondTextEOS != 1 || secondText != "recognized text" {
				t.Fatalf("old text EOS=%d history EOS=%d; replacement EOS=%d text=%q", oldTextEOS, oldHistoryEOS, secondTextEOS, secondText)
			}
		})
	}
}
