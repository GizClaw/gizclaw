package transformer

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	speech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
)

func TestPTTWebSocketCaptureFailure(t *testing.T) {
	for _, controlEOS := range []bool{false, true} {
		t.Run(fmt.Sprintf("controlEOS=%t", controlEOS), func(t *testing.T) {
			p := newPTTWireProvider(t, "capture599")
			client := speech.NewClient("local-only", speech.WithAPIKey("local-only"), speech.WithWebSocketURL("ws"+strings.TrimPrefix(p.server.URL, "http")))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 32)
			defer input.Abort(context.Canceled)
			transcode := false
			tr, err := doubaorealtime.New(doubaorealtime.Config{
				Client: client, Model: "2.2.0.0", Mode: doubaorealtime.ModePushToTalk,
				InputFormat: "pcm", InputTranscode: &transcode, Format: "pcm",
			})
			if err != nil {
				t.Fatal(err)
			}
			output, err := tr.Transform(ctx, input.Stream())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			chunks := make(chan *genx.MessageChunk, 64)
			outputError := make(chan error, 1)
			go func() {
				defer close(chunks)
				for {
					chunk, err := output.Next()
					if err != nil {
						outputError <- err
						return
					}
					chunks <- chunk
				}
			}()
			push := func(chunk *genx.MessageChunk) {
				t.Helper()
				if err := input.Add(chunk); err != nil {
					t.Fatal(err)
				}
			}
			bos := func(id string) { push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}) }
			audio := func(id string) {
				push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 640)}, Ctrl: &genx.StreamCtrl{StreamID: id}})
			}
			eos := func(id string) {
				push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}})
				if controlEOS {
					push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}})
				}
			}
			observe := func(id string, turn int) {
				t.Helper()
				transcript, answer := "", ""
				textEOS, audioEOS := false, false
				for {
					select {
					case chunk, ok := <-chunks:
						if !ok {
							t.Fatalf("output terminated after capture failure: %v", <-outputError)
						}
						if chunk.Ctrl == nil {
							continue
						}
						owner := chunk.Ctrl.StreamID
						if chunk.Ctrl.SourceStreamID != "" {
							owner = chunk.Ctrl.SourceStreamID
						}
						if owner == "failed-capture" {
							if text, ok := chunk.Part.(genx.Text); ok && text != "" {
								t.Fatalf("failed capture published text %q", text)
							}
						}
						if owner != id {
							continue
						}
						if text, ok := chunk.Part.(genx.Text); ok {
							if chunk.Role == genx.RoleUser {
								transcript += string(text)
							}
							if chunk.Role == genx.RoleModel {
								answer += string(text)
							}
						}
						if chunk.Role == genx.RoleModel && chunk.Ctrl.EndOfStream {
							if chunk.Ctrl.Error != "" {
								t.Fatal(chunk.Ctrl.Error)
							}
							switch chunk.Part.(type) {
							case genx.Text:
								textEOS = true
							case *genx.Blob:
								audioEOS = true
							}
							if textEOS && audioEOS {
								if transcript != fmt.Sprintf("synthetic turn %d", turn) || answer != fmt.Sprintf("answer %d", turn) {
									t.Fatalf("wrong new-turn output: %q / %q", transcript, answer)
								}
								t.Logf("COMPLETE owner=%s transcript=%q answer=%q", id, transcript, answer)
								return
							}
						}
					case <-ctx.Done():
						t.Fatal("turn did not complete: ", ctx.Err())
					}
				}
			}
			bos("normal-1")
			audio("normal-1")
			audio("normal-1")
			eos("normal-1")
			observe("normal-1", 1)
			bos("failed-capture")
			audio("failed-capture")
			select {
			case <-p.replacementReady:
			case <-ctx.Done():
				t.Fatal("capture failure did not open replacement: ", ctx.Err())
			}
			// This suffix is provided only after the failure and replacement;
			// it must be consumed locally, through both possible EOS shapes.
			audio("failed-capture")
			eos("failed-capture")
			for turn := 2; turn <= 3; turn++ {
				id := fmt.Sprintf("normal-%d", turn)
				bos(id)
				audio(id)
				audio(id)
				eos(id)
				observe(id, turn)
			}
			p.mu.Lock()
			connections, bytes, ends := p.connections, p.audioBytes, p.turns
			p.mu.Unlock()
			t.Logf("capture recovery controlEOS=%t connections=%d uploaded_bytes=%d EndASR=%d", controlEOS, connections, bytes, ends)
			// 3 healthy turns + only the failed turn's already submitted frame.
			if connections != 2 || bytes != 4480 || ends != 3 {
				t.Fatalf("audio replay or failed suffix uploaded: connections=%d bytes=%d EndASR=%d", connections, bytes, ends)
			}
		})
	}
}
