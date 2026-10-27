package agenthost

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxast "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func captureTestChunk(epoch *genx.ResponseEpoch, text string) *genx.MessageChunk {
	return &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "response", Label: "assistant", ResponseEpoch: epoch}}
}

func captureTestInput(id, mode string, end bool) *genx.MessageChunk {
	return &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: id, InputMode: mode, BeginOfStream: !end, EndOfStream: end}}
}

func startCaptureTest(t *testing.T, mode apitypes.WorkspaceInputMode) (*captureOutput, *InputStream, *InputStream) {
	t.Helper()
	input, source := NewInputStream(32), NewInputStream(32)
	gate := newCaptureOutput()
	ctx := context.WithValue(t.Context(), captureOutputKey{}, gate)
	agent := captureOutputAgent{Agent: historyTestAgent{output: source}, mode: mode}
	output, err := agent.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	return gate, input, source
}

func admitCapture(t *testing.T, gate *captureOutput, input *InputStream, chunk *genx.MessageChunk) {
	t.Helper()
	service := &Service{runtime: &runtime{capture: gate}}
	if _, pushed, err := service.PushInput(t.Context(), input, chunk); err != nil || !pushed {
		t.Fatal(err)
	}
}

func TestCaptureOutputReleasesBeforeProviderReadsEOS(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModePushToTalk)
		admitCapture(t, gate, input, captureTestInput("capture", "", false))
		bos, _ := input.Next()
		chunk := captureTestChunk(bos.Ctrl.ResponseEpoch, "already translated")
		_ = source.Push(t.Context(), chunk)
		available := make(chan *genx.MessageChunk, 1)
		go func() { chunk, _ := gate.Next(); available <- chunk }()
		synctest.Wait()
		select {
		case <-available:
			t.Fatal("PTT output escaped before release")
		default:
		}
		admitCapture(t, gate, input, captureTestInput("capture", "", true))
		// No provider reads input again. Publication must not depend on its
		// upload backlog, Finish, or SessionFinished.
		if got := <-available; got != chunk {
			t.Fatalf("released %v, want cached %v", got, chunk)
		}
	})
}

func TestCaptureOutputUsesDeclaredModeAndWorkspaceDefault(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fallback apitypes.WorkspaceInputMode
		declared string
		hold     bool
	}{
		{"legacy PTT", apitypes.WorkspaceInputModePushToTalk, "", true},
		{"legacy realtime", apitypes.WorkspaceInputModeRealtime, "", false},
		{"PTT in realtime workspace", apitypes.WorkspaceInputModeRealtime, "push-to-talk", true},
		{"explicit realtime", apitypes.WorkspaceInputModePushToTalk, "realtime", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				gate, input, source := startCaptureTest(t, tc.fallback)
				admitCapture(t, gate, input, captureTestInput("capture", tc.declared, false))
				bos, _ := input.Next()
				chunk := captureTestChunk(bos.Ctrl.ResponseEpoch, "translated")
				_ = source.Push(t.Context(), chunk)
				available := make(chan *genx.MessageChunk, 1)
				go func() { chunk, _ := gate.Next(); available <- chunk }()
				synctest.Wait()
				if tc.hold {
					select {
					case <-available:
						t.Fatal("PTT output escaped")
					default:
					}
					admitCapture(t, gate, input, captureTestInput("capture", "", true))
				}
				if got := <-available; got != chunk {
					t.Fatalf("output=%v, want %v", got, chunk)
				}
			})
		})
	}
}

func TestCaptureOutputDropsReusedInputIDEpoch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModePushToTalk)
		admitCapture(t, gate, input, captureTestInput("reused", "", false))
		old, _ := input.Next()
		_ = source.Push(t.Context(), captureTestChunk(old.Ctrl.ResponseEpoch, "old cached"))
		synctest.Wait()
		admitCapture(t, gate, input, captureTestInput("reused", "", false))
		current, _ := input.Next()
		_ = source.Push(t.Context(), captureTestChunk(old.Ctrl.ResponseEpoch, "old late"))
		want := captureTestChunk(current.Ctrl.ResponseEpoch, "new")
		_ = source.Push(t.Context(), want)
		admitCapture(t, gate, input, captureTestInput("reused", "", true))
		got, err := gate.Next()
		if err != nil || got != want {
			t.Fatalf("replacement output=%v, %v; want new epoch", got, err)
		}
	})
}

func TestCaptureOutputFailureDiscardsSpeculation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModePushToTalk)
		admitCapture(t, gate, input, captureTestInput("capture", "", false))
		bos, _ := input.Next()
		_ = source.Push(t.Context(), captureTestChunk(bos.Ctrl.ResponseEpoch, "discard"))
		synctest.Wait()
		failure := captureTestChunk(bos.Ctrl.ResponseEpoch, "failed payload must not escape PTT")
		failure.Ctrl.EndOfStream = true
		failure.Ctrl.Error = "provider failed"
		_ = source.Push(t.Context(), failure)
		if got, err := gate.Next(); err != nil || got.Ctrl.Error != failure.Ctrl.Error || got.Part != genx.Text("") {
			t.Fatalf("failure=%v, %v; want only terminal", got, err)
		}
		admitCapture(t, gate, input, captureTestInput("capture", "", true))
		_ = source.Close()
		if chunk, err := gate.Next(); chunk != nil || !errors.Is(err, io.EOF) {
			t.Fatalf("speculative output survived failure: %v, %v", chunk, err)
		}
	})
}

func TestCaptureOutputInputErrorDiscardsPreparedAudio(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModePushToTalk)
		admitCapture(t, gate, input, captureTestInput("capture", "", false))
		bos, _ := input.Next()
		_ = source.Push(t.Context(), captureTestChunk(bos.Ctrl.ResponseEpoch, "discard"))
		synctest.Wait()
		terminal := captureTestInput("capture", "", true)
		terminal.Ctrl.Error, terminal.Ctrl.ErrorCode = "input failed", "INPUT_FAILED"
		admitCapture(t, gate, input, terminal)
		got, err := gate.Next()
		if err != nil || got.Ctrl.ErrorCode != "INPUT_FAILED" || !got.IsEndOfStream() || got.Part != genx.Text("") {
			t.Fatalf("input error output=%v, %v", got, err)
		}
		_ = source.Close()
		if chunk, err := gate.Next(); chunk != nil || !IsStreamDone(err) {
			t.Fatalf("prepared output survived input error: %v, %v", chunk, err)
		}
	})
}

func TestCaptureOutputInterruptsPrefetchedUnacknowledgedEOS(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModePushToTalk)
		gate.DeferOutputObservation()
		admitCapture(t, gate, input, captureTestInput("capture", "", false))
		bos, _ := input.Next()
		admitCapture(t, gate, input, captureTestInput("capture", "", true))
		for index := range 2 {
			chunk := captureTestChunk(bos.Ctrl.ResponseEpoch, "")
			chunk.Ctrl.BeginOfStream, chunk.Ctrl.EndOfStream = index == 0, index == 1
			_ = source.Push(t.Context(), chunk)
			if _, err := gate.Next(); err != nil {
				t.Fatal(err)
			}
		}
		// A downstream queue has read the normal EOS, but its audio has not
		// drained. Replacement must still close that prefetched route.
		admitCapture(t, gate, input, captureTestInput("replacement", "", false))
		got, err := gate.Next()
		if err != nil || !got.IsEndOfStream() || got.Ctrl.ErrorCode != "STREAM_INTERRUPTED" || got.Ctrl.ResponseEpoch != bos.Ctrl.ResponseEpoch {
			t.Fatalf("prefetched interruption=%v, %v", got, err)
		}
	})
}

func TestCaptureOutputPreservesCommittedUserHistoryOnReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModePushToTalk)
		admitCapture(t, gate, input, captureTestInput("capture", "", false))
		bos, _ := input.Next()
		admitCapture(t, gate, input, captureTestInput("capture", "", true))
		transcript := captureTestChunk(bos.Ctrl.ResponseEpoch, "committed transcript")
		transcript.Role, transcript.Ctrl.Label = genx.RoleUser, "transcript"
		_ = source.Push(t.Context(), transcript)
		_ = source.Push(t.Context(), captureTestChunk(bos.Ctrl.ResponseEpoch, "interrupt assistant"))
		synctest.Wait()
		admitCapture(t, gate, input, captureTestInput("replacement", "", false))
		if got, err := gate.Next(); err != nil || got != transcript {
			t.Fatalf("committed user history was dropped: %v, %v", got, err)
		}
	})
}

func TestCaptureOutputClosesPublishedTranscriptOnReplacement(t *testing.T) {
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	recorder := newHistoryRecorder(history, "gear", nil)
	gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModeRealtime)
	admitCapture(t, gate, input, captureTestInput("capture", "realtime", false))
	first, _ := input.Next()
	transcript := captureTestChunk(first.Ctrl.ResponseEpoch, "committed prefix")
	transcript.Role, transcript.Ctrl.Label, transcript.Ctrl.BeginOfStream = genx.RoleUser, "transcript", true
	_ = source.Push(t.Context(), transcript)
	got, err := gate.Next()
	if err != nil {
		t.Fatal(err)
	}
	recorder.recordDelivered(t.Context(), got)
	admitCapture(t, gate, input, captureTestInput("replacement", "realtime", false))
	current, _ := input.Next()
	late := transcript.Clone()
	late.Part, late.Ctrl.BeginOfStream, late.Ctrl.EndOfStream = genx.Text(""), false, true
	_ = source.Push(t.Context(), late)
	marker := captureTestChunk(current.Ctrl.ResponseEpoch, "replacement")
	marker.Ctrl.BeginOfStream, marker.Ctrl.EndOfStream = true, true
	_ = source.Push(t.Context(), marker)
	terminal, err := gate.Next()
	if err != nil || terminal.Role != genx.RoleUser || !terminal.IsEndOfStream() || terminal.Ctrl.ErrorCode != "STREAM_INTERRUPTED" || terminal.Ctrl.ResponseEpoch != first.Ctrl.ResponseEpoch {
		t.Fatalf("transcript replacement terminal=%v, %v", terminal, err)
	}
	recorder.recordDelivered(t.Context(), terminal)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := recorder.waitForPriorText(ctx, "replacement"); err != nil {
		t.Fatalf("replacement left the transcript History barrier open: %v", err)
	}
	page, err := history.ListEntries(t.Context(), "", "", 10)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Text != "committed prefix" || !page.Entries[0].Interrupted {
		t.Fatalf("interrupted transcript History=%+v, %v", page.Entries, err)
	}
	if got, err := gate.Next(); err != nil || got != marker {
		t.Fatalf("late transcript terminal escaped: %v, %v", got, err)
	}
}

func TestCaptureOutputPreservesInterruptAcrossConsecutiveBOS(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModeRealtime)
		admitCapture(t, gate, input, captureTestInput("first", "realtime", false))
		first, _ := input.Next()
		published := captureTestChunk(first.Ctrl.ResponseEpoch, "first")
		published.Ctrl.BeginOfStream = true
		_ = source.Push(t.Context(), published)
		if _, err := gate.Next(); err != nil {
			t.Fatal(err)
		}
		// A normal PTT turn opens control and audio before its output reader
		// can dequeue the prior turn's interruption.
		admitCapture(t, gate, input, &genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "replacement", BeginOfStream: true}})
		_, _ = input.Next()
		admitCapture(t, gate, input, captureTestInput("replacement", "realtime", false))
		current, _ := input.Next()
		marker := captureTestChunk(current.Ctrl.ResponseEpoch, "replacement")
		marker.Ctrl.BeginOfStream, marker.Ctrl.EndOfStream = true, true
		_ = source.Push(t.Context(), marker)
		synctest.Wait()
		terminal, err := gate.Next()
		if err != nil || !terminal.IsEndOfStream() || terminal.Ctrl.ErrorCode != "STREAM_INTERRUPTED" || terminal.Ctrl.ResponseEpoch != first.Ctrl.ResponseEpoch {
			t.Fatalf("queued interruption was lost: %v, %v", terminal, err)
		}
		if got, err := gate.Next(); err != nil || got != marker {
			t.Fatalf("replacement order=%v, %v", got, err)
		}
	})
}

func TestCaptureOutputAcknowledgesControlTerminalForAllMIMEs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModeRealtime)
		gate.DeferOutputObservation()
		admitCapture(t, gate, input, captureTestInput("first", "realtime", false))
		first, _ := input.Next()
		for _, part := range []genx.Part{genx.Text("text"), &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}} {
			chunk := captureTestChunk(first.Ctrl.ResponseEpoch, "")
			chunk.Part, chunk.Ctrl.BeginOfStream = part, true
			_ = source.Push(t.Context(), chunk)
			got, err := gate.Next()
			if err != nil {
				t.Fatal(err)
			}
			gate.ObserveOutput(got)
		}
		terminal := captureTestChunk(first.Ctrl.ResponseEpoch, "")
		terminal.Part, terminal.Ctrl.EndOfStream = nil, true
		_ = source.Push(t.Context(), terminal)
		got, err := gate.Next()
		if err != nil {
			t.Fatal(err)
		}
		gate.ObserveOutput(got)
		admitCapture(t, gate, input, captureTestInput("replacement", "realtime", false))
		current, _ := input.Next()
		marker := captureTestChunk(current.Ctrl.ResponseEpoch, "replacement")
		marker.Ctrl.BeginOfStream = true
		_ = source.Push(t.Context(), marker)
		if got, err := gate.Next(); err != nil || got != marker {
			t.Fatalf("acknowledged control terminal left a MIME route open: %v, %v", got, err)
		}
	})
}

func TestCaptureOutputRetainsInclusiveAudioLimit(t *testing.T) {
	turn := &captureTurn{}
	chunk := &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}
	for range 6000 {
		if err := turn.retain(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := turn.retain(chunk); !errors.Is(err, errCaptureOutputLimit) {
		t.Fatalf("overflow error=%v", err)
	}
	// Source/history audio is not part of speculative assistant accounting.
	chunk.Role = genx.RoleUser
	if err := turn.retain(chunk); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureOutputDefersHistoryAndDeliveryObservation(t *testing.T) {
	input, source := NewInputStream(8), NewInputStream(8)
	upstream := newRecordingObservationStream(source)
	upstream.abandoned = make(chan *genx.MessageChunk, 8)
	gate := newCaptureOutput()
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	agent := wrapHistoryAgent(captureOutputAgent{Agent: historyTestAgent{output: upstream}, mode: apitypes.WorkspaceInputModePushToTalk}, history)
	out, err := agent.Transform(context.WithValue(ctx, captureOutputKey{}, gate), input)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	admitCapture(t, gate, input, captureTestInput("capture", "", false))
	bos, _ := input.Next()
	for index, text := range []string{"", "translation", ""} {
		chunk := captureTestChunk(bos.Ctrl.ResponseEpoch, text)
		chunk.Ctrl.BeginOfStream, chunk.Ctrl.EndOfStream = index == 0, index == 2
		if err := source.Push(ctx, chunk); err != nil {
			t.Fatal(err)
		}
	}
	gate.mu.Lock()
	for len(gate.retained) < 3 && gate.err == nil {
		gate.cond.Wait()
	}
	gate.mu.Unlock()
	page, err := agent.ListHistory(ctx, apitypes.PeerRunHistoryListRequest{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("speculation was recorded: items=%v err=%v", page.Items, err)
	}
	select {
	case <-upstream.observed:
		t.Fatal("speculation was acknowledged as delivered")
	default:
	}
	admitCapture(t, gate, input, captureTestInput("capture", "", true))
	for range 3 {
		if _, err := out.Next(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-upstream.observed:
		case <-ctx.Done():
			t.Fatal("delivery observation was not forwarded")
		}
	}
	page, err = agent.ListHistory(ctx, apitypes.PeerRunHistoryListRequest{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Text != "translation" {
		t.Fatalf("released history=%v, %v", page.Items, err)
	}
}

type captureInputPusher func(context.Context, *genx.MessageChunk) error

func (p captureInputPusher) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	return p(ctx, chunk)
}

func TestCaptureInputAdmissionDoesNotBlockOutputProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate, input, source := startCaptureTest(t, apitypes.WorkspaceInputModePushToTalk)
		service := &Service{runtime: &runtime{capture: gate}}
		if _, _, err := service.PushInput(t.Context(), input, captureTestInput("capture", "", false)); err != nil {
			t.Fatal(err)
		}
		bos, _ := input.Next()
		entered, release := make(chan struct{}), make(chan struct{})
		pusher := captureInputPusher(func(context.Context, *genx.MessageChunk) error {
			close(entered)
			<-release
			return nil
		})
		done := make(chan error, 1)
		go func() {
			_, _, err := service.PushInput(t.Context(), pusher, captureTestInput("capture", "", true))
			done <- err
		}()
		<-entered
		want := captureTestChunk(bos.Ctrl.ResponseEpoch, "prepared during blocked admission")
		_ = source.Push(t.Context(), want)
		synctest.Wait()
		gate.mu.Lock()
		if len(gate.retained) != 1 || gate.turn.ended {
			t.Errorf("producer could not progress during input admission: retained=%d ended=%v", len(gate.retained), gate.turn.ended)
		}
		gate.mu.Unlock()
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if got, err := gate.Next(); err != nil || got != want {
			t.Fatalf("release output=%v, %v", got, err)
		}
	})
}

type captureASTMux struct {
	ttsStarted chan struct{}
	upload     chan struct{}
	uploadDone chan struct{}
}

func (m captureASTMux) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	out := NewInputStream(32)
	context.AfterFunc(ctx, func() { _ = out.CloseWithError(context.Cause(ctx)) })
	go func() {
		defer out.Close()
		first, err := input.Next()
		if err != nil {
			return
		}
		epoch := first.Ctrl.ResponseEpoch
		if strings.HasPrefix(pattern, "model/") {
			for _, text := range []string{"", "Hello.", ""} {
				chunk := captureTestChunk(epoch, text)
				chunk.Ctrl.BeginOfStream = text == "" && !first.IsEndOfStream()
				chunk.Ctrl.EndOfStream = first.IsEndOfStream()
				_ = out.Push(ctx, chunk)
				if text != "" {
					first = &genx.MessageChunk{Ctrl: &genx.StreamCtrl{EndOfStream: true}}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-m.upload:
			}
			for {
				chunk, err := input.Next()
				if err != nil {
					return
				}
				if chunk.IsEndOfStream() {
					close(m.uploadDone)
					return
				}
			}
		}
		close(m.ttsStarted)
		for _, flags := range []int{1, 0, 2} {
			if flags == 2 {
				for {
					chunk, err := input.Next()
					if err != nil || chunk.IsEndOfStream() {
						break
					}
				}
			}
			data := []byte(nil)
			if flags == 0 {
				data = []byte{0xf8, 0xff, 0xfe}
			}
			_ = out.Push(ctx, &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: data}, Ctrl: &genx.StreamCtrl{StreamID: "tts", BeginOfStream: flags == 1, EndOfStream: flags == 2, ResponseEpoch: epoch}})
		}
	}()
	return out, nil
}

func TestCaptureOutputPreparesExternalTTSWhileUploaderIsBlocked(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux := captureASTMux{ttsStarted: make(chan struct{}), upload: make(chan struct{}), uploadDone: make(chan struct{})}
		transformer, err := genxast.New(genxast.Config{Transformer: mux, Model: "ast", ExternalVoice: "voice", Params: map[string]any{"lang_pair": "zh/en"}, PrepareOutput: PrepareSpeechOutput})
		if err != nil {
			t.Fatal(err)
		}
		gate := newCaptureOutput()
		ctx := context.WithValue(t.Context(), captureOutputKey{}, gate)
		input := NewInputStream(32)
		agent := captureOutputAgent{Agent: NewTransformerAgent(transformer), mode: apitypes.WorkspaceInputModePushToTalk}
		out, err := agent.Transform(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		admitCapture(t, gate, input, captureTestInput("capture", "", false))
		<-mux.ttsStarted
		synctest.Wait()
		gate.mu.Lock()
		if len(gate.ready) != 0 || len(gate.retained) == 0 {
			t.Errorf("precomputed PTT output: ready=%d retained=%d", len(gate.ready), len(gate.retained))
		}
		gate.mu.Unlock()
		admitCapture(t, gate, input, captureTestInput("capture", "", true))
		for {
			chunk, err := out.Next()
			if err != nil {
				t.Fatal(err)
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				break
			}
		}
		select {
		case <-mux.uploadDone:
			t.Fatal("test failed to keep the provider uploader blocked")
		default:
		}
	})
}
