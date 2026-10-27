package agenthost

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestPrepareSpeechOutputGroupsPTTBeforeSynthesis(t *testing.T) {
	for _, mode := range []apitypes.WorkspaceInputMode{apitypes.WorkspaceInputModePushToTalk, apitypes.WorkspaceInputModeRealtime} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				gate, input, _ := startCaptureTest(t, mode)
				admitCapture(t, gate, input, captureTestInput("capture", "", false))
				bos, _ := input.Next()
				source := NewInputStream(32)
				for index, text := range []string{"Hello", "world."} {
					for step := range 4 {
						chunk := captureTestChunk(bos.Ctrl.ResponseEpoch, "")
						chunk.Ctrl.StreamID = []string{"source-one", "source-two"}[index]
						chunk.Ctrl.BeginOfStream, chunk.Ctrl.MessageEnd, chunk.Ctrl.EndOfStream = step == 0, step == 2, step == 3
						if step == 1 {
							chunk.Part = genx.Text(text)
						}
						_ = source.Push(t.Context(), chunk)
					}
				}
				_ = source.Push(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "source-two", Label: "assistant", ResponseEpoch: bos.Ctrl.ResponseEpoch, ResponseEpochEnd: true}})
				_ = source.Close()
				prepared, err := PrepareSpeechOutput(context.WithValue(t.Context(), activeCaptureOutputKey{}, gate), source)
				if err != nil {
					t.Fatal(err)
				}
				defer prepared.Close()
				ids := map[string]bool{}
				var text strings.Builder
				var begins, ends, sentences, completed int
				for {
					chunk, err := prepared.Next()
					if IsStreamDone(err) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					ids[chunk.Ctrl.StreamID] = true
					if chunk.IsBeginOfStream() {
						begins++
					}
					if chunk.IsEndOfStream() {
						ends++
					}
					if chunk.Ctrl.MessageEnd {
						sentences++
					}
					if chunk.Ctrl.ResponseEpochEnd {
						completed++
					}
					if part, ok := chunk.Part.(genx.Text); ok {
						text.WriteString(string(part))
					}
				}
				if mode == apitypes.WorkspaceInputModePushToTalk {
					if len(ids) != 1 || begins != 1 || ends != 1 || sentences != 2 || completed != 1 || text.String() != "Hello world." {
						t.Fatalf("PTT ids=%v BOS=%d EOS=%d sentences=%d completed=%d text=%q", ids, begins, ends, sentences, completed, text.String())
					}
				} else if len(ids) != 2 || begins != 2 || ends != 2 {
					t.Fatalf("realtime provider routes changed: ids=%v BOS=%d EOS=%d", ids, begins, ends)
				}
			})
		})
	}
}

func TestCaptureOutputHidesInternalSentenceBoundariesAndAcknowledgesOriginal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input, source := NewInputStream(8), NewInputStream(8)
		upstream := newRecordingObservationStream(source)
		gate := newCaptureOutput()
		agent := captureOutputAgent{Agent: historyTestAgent{output: upstream}, mode: apitypes.WorkspaceInputModePushToTalk}
		out, err := agent.Transform(context.WithValue(t.Context(), captureOutputKey{}, gate), input)
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		admitCapture(t, gate, input, captureTestInput("capture", "", false))
		bos, _ := input.Next()
		chunk := captureTestChunk(bos.Ctrl.ResponseEpoch, "sentence")
		chunk.Ctrl.MessageID, chunk.Ctrl.MessageEnd = "internal-message", true
		_ = source.Push(t.Context(), chunk)
		synctest.Wait()
		admitCapture(t, gate, input, captureTestInput("capture", "", true))
		got, err := out.Next()
		if err != nil || got.Ctrl.MessageID != "" || got.Ctrl.MessageEnd || got.Part != chunk.Part {
			t.Fatalf("visible output=%v, %v", got, err)
		}
		if original := <-upstream.observed; original != chunk {
			t.Fatal("rewriting lost delivery acknowledgement")
		}
	})
}

func TestPrepareSpeechOutputRealtimeEndsWithoutNextInputPacket(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, _ := startCaptureTest(t, apitypes.WorkspaceInputModeRealtime)
		admitCapture(t, gate, input, captureTestInput("capture", "", false))
		bos, _ := input.Next()
		source := NewInputStream(8)
		prepared, err := PrepareSpeechOutput(context.WithValue(t.Context(), activeCaptureOutputKey{}, gate), source)
		if err != nil {
			t.Fatal(err)
		}
		defer prepared.Close()
		for index, text := range []string{"", "last sentence", ""} {
			chunk := captureTestChunk(bos.Ctrl.ResponseEpoch, text)
			chunk.Ctrl.BeginOfStream, chunk.Ctrl.MessageEnd = index == 0, index == 2
			_ = source.Push(t.Context(), chunk)
		}
		for range 2 {
			if _, err := prepared.Next(); err != nil {
				t.Fatal(err)
			}
		}
		terminal := make(chan *genx.MessageChunk, 1)
		go func() { chunk, _ := prepared.Next(); terminal <- chunk }()
		synctest.Wait()
		select {
		case chunk := <-terminal:
			if chunk == nil || !chunk.IsEndOfStream() || chunk.Ctrl.MessageEnd {
				t.Fatalf("subtitle final did not close realtime text: %v", chunk)
			}
		default:
			t.Fatal("realtime text waited for a new packet or session EOS")
		}
	})
}

func TestPrepareSpeechOutputKeepsTranscriptOpenAcrossCaptureControlEOS(t *testing.T) {
	for _, mode := range []apitypes.WorkspaceInputMode{apitypes.WorkspaceInputModePushToTalk, apitypes.WorkspaceInputModeRealtime} {
		for _, role := range []genx.Role{"", genx.RoleUser} {
			t.Run(string(mode)+"/role="+string(role), func(t *testing.T) {
				epoch := genx.NewResponseEpoch("input")
				gate := newCaptureOutput()
				gate.turn = &captureTurn{epoch: epoch, audio: true, ptt: mode == apitypes.WorkspaceInputModePushToTalk}
				source := historyStreamFromChunks(
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("before "), Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript", BeginOfStream: true, ResponseEpoch: epoch}},
					&genx.MessageChunk{Role: role, Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "user", EndOfStream: true, ResponseEpoch: epoch}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("after"), Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript", ResponseEpoch: epoch}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript", EndOfStream: true, ResponseEpoch: epoch}},
					&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("translation"), Ctrl: &genx.StreamCtrl{StreamID: "model", Label: "assistant", BeginOfStream: true, EndOfStream: true, ResponseEpoch: epoch}},
					&genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "model", Label: "assistant", ResponseEpoch: epoch, ResponseEpochEnd: true}},
				)
				prepared, err := PrepareSpeechOutput(context.WithValue(t.Context(), activeCaptureOutputKey{}, gate), source)
				if err != nil {
					t.Fatal(err)
				}
				defer prepared.Close()
				history := newTestWorkspaceHistory(t, newTestObjectStore(t))
				recorder := newHistoryRecorder(history, "gear", nil)
				for {
					chunk, err := prepared.Next()
					if IsStreamDone(err) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					recorder.recordDelivered(t.Context(), chunk)
				}
				page, err := history.List(t.Context(), apitypes.PeerRunHistoryListRequest{Order: new(apitypes.PeerRunHistoryListRequestOrderAsc)})
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Items) != 2 {
					t.Fatalf("History entries=%d, want one user and one assistant", len(page.Items))
				}
				if page.Items[0].Text != "before after" {
					t.Fatalf("transcript=%q", page.Items[0].Text)
				}
			})
		}
	}
}
