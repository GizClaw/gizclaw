package doubaoasr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

type recoveryProvider struct {
	events chan asrResult
	sent   chan fakeDoubaoASRSend
	closed chan struct{}
	once   sync.Once
	send   func(context.Context, []byte, bool) error
}

func newRecoveryProvider() *recoveryProvider {
	return &recoveryProvider{events: make(chan asrResult, 16), sent: make(chan fakeDoubaoASRSend, 32), closed: make(chan struct{})}
}

func (p *recoveryProvider) SendAudio(ctx context.Context, data []byte, last bool) error {
	if p.send != nil {
		if err := p.send(ctx, data, last); err != nil {
			return err
		}
	}
	select {
	case p.sent <- fakeDoubaoASRSend{data: slices.Clone(data), isLast: last}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *recoveryProvider) Recv() iter.Seq2[*doubaospeech.ASRV2Result, error] {
	return func(yield func(*doubaospeech.ASRV2Result, error) bool) {
		for {
			select {
			case event, ok := <-p.events:
				if !ok {
					return
				}
				if !yield(event.value, event.err) || event.err != nil {
					return
				}
			case <-p.closed:
				return
			}
		}
	}
}

func (p *recoveryProvider) Close() error                 { p.once.Do(func() { close(p.closed) }); return nil }
func (p *recoveryProvider) AudioDuration() time.Duration { return 100 * time.Millisecond }

func recoveryFailure() error {
	return &doubaospeech.Error{Code: asrServerError, Message: "OperatorWrapper Process failed: big asr recv err. rpc timeout: CallWithTimeout: timeout in business code, timeout_config=3s", ReqID: "asr-original", ConnectID: "asr-original"}
}

func recoveryResult(text string, start, end int) *doubaospeech.ASRV2Result {
	return &doubaospeech.ASRV2Result{Text: text, Utterances: []doubaospeech.ASRV2Utterance{{Text: text, StartTime: start, EndTime: end, Definite: true}}, ReqID: "asr-replacement", ConnectID: "asr-replacement"}
}

func awaitRecovery[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("recovery made no progress")
		var zero T
		return zero
	}
}

func pushRecoveryAudio(t *testing.T, input *bufferStream, stream string, data []byte, bos, eos bool) {
	t.Helper()
	if err := input.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: data}, Ctrl: &genx.StreamCtrl{StreamID: stream, BeginOfStream: bos, EndOfStream: eos}}); err != nil {
		t.Fatal(err)
	}
}

func readRecoveryTranscript(t *testing.T, output genx.Stream) ([]*genx.MessageChunk, string) {
	t.Helper()
	var chunks []*genx.MessageChunk
	var text strings.Builder
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		chunks = append(chunks, chunk)
		if chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" {
			if part, ok := chunk.Part.(genx.Text); ok && !chunk.Ctrl.TextInterim {
				text.WriteString(string(part))
			}
			if chunk.IsEndOfStream() {
				return chunks, text.String()
			}
		}
	}
}

func TestRecoveryContinuesEighthUtteranceWithoutReplayingHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	first, second := newRecoveryProvider(), newRecoveryProvider()
	providers := []*recoveryProvider{first, second}
	var opens atomic.Int32
	transformer := newTransformer(Config{EmitInterim: true, RealtimePacing: new(false)})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		n := int(opens.Add(1))
		if n > len(providers) {
			return nil, fmt.Errorf("unexpected open %d", n)
		}
		return providers[n-1], nil
	}
	input := newBufferStream(16)
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for i := 1; i <= 7; i++ {
		pushRecoveryAudio(t, input, "guess", bytes.Repeat([]byte{byte(i)}, 3200), i == 1, false)
		awaitRecovery(t, first.sent)
		first.events <- asrResult{value: recoveryResult(fmt.Sprint(i), (i-1)*100, i*100)}
		_, text := readRecoveryTranscript(t, output)
		if text != fmt.Sprint(i) {
			t.Fatalf("text=%q", text)
		}
	}
	current := bytes.Repeat([]byte{8}, 3200)
	pushRecoveryAudio(t, input, "guess", current, false, false)
	awaitRecovery(t, first.sent)
	first.events <- asrResult{err: recoveryFailure()}
	replayed := awaitRecovery(t, second.sent)
	if !bytes.Equal(replayed.data, current) || replayed.isLast {
		t.Fatalf("replayed prior committed audio: %d bytes", len(replayed.data))
	}
	second.events <- asrResult{value: recoveryResult("eighth", 0, 100)}
	chunks, text := readRecoveryTranscript(t, output)
	if text != "eighth" {
		t.Fatalf("transcript=%q", text)
	}
	audioBytes := 0
	for _, chunk := range chunks {
		if chunk.Ctrl.StreamID != "guess:asr:8" {
			t.Fatalf("wrong recovery route: %#v", chunk.Ctrl)
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			audioBytes += len(blob.Data)
		}
	}
	if audioBytes != len(current) {
		t.Fatalf("history audio=%d, want %d", audioBytes, len(current))
	}
	pushRecoveryAudio(t, input, "guess", bytes.Repeat([]byte{9}, 3200), false, false)
	awaitRecovery(t, second.sent)
	second.events <- asrResult{value: recoveryResult("ninth", 100, 200)}
	chunks, text = readRecoveryTranscript(t, output)
	if text != "ninth" || chunks[len(chunks)-1].Ctrl.StreamID != "guess:asr:9" || opens.Load() != 2 {
		t.Fatalf("next turn=%q, opens=%d", text, opens.Load())
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	awaitRecovery(t, first.closed)
	awaitRecovery(t, second.closed)
}

func TestRecoveryFailsTwiceAndNeverOpensThirdAttempt(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, second := newRecoveryProvider(), newRecoveryProvider()
	var opens atomic.Int32
	transformer := newTransformer(Config{RealtimePacing: new(false)})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		if opens.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}
	input := newBufferStream(4)
	output, _ := transformer.Transform(ctx, input)
	defer output.Close()
	pushRecoveryAudio(t, input, "turn", make([]byte, 3200), true, false)
	awaitRecovery(t, first.sent)
	first.events <- asrResult{err: recoveryFailure()}
	awaitRecovery(t, second.sent)
	final := recoveryFailure()
	second.events <- asrResult{err: final}
	for {
		_, err := output.Next()
		if err != nil {
			if !errors.Is(err, final) {
				t.Fatalf("final error=%v", err)
			}
			break
		}
	}
	if opens.Load() != 2 {
		t.Fatalf("attempts=%d", opens.Load())
	}
}

func TestRecoveryFinalTextAfterPreviouslyCommittedUtterance(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, second := newRecoveryProvider(), newRecoveryProvider()
	var opens atomic.Int32
	transformer := newTransformer(Config{EmitInterim: true, RealtimePacing: new(false)})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		if opens.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}
	input := newBufferStream(4)
	output, _ := transformer.Transform(ctx, input)
	defer output.Close()
	pushRecoveryAudio(t, input, "turn", make([]byte, 3200), true, false)
	awaitRecovery(t, first.sent)
	first.events <- asrResult{value: recoveryResult("first", 0, 100)}
	readRecoveryTranscript(t, output)
	pushRecoveryAudio(t, input, "turn", make([]byte, 3200), false, false)
	awaitRecovery(t, first.sent)
	first.events <- asrResult{err: recoveryFailure()}
	awaitRecovery(t, second.sent)
	second.events <- asrResult{value: &doubaospeech.ASRV2Result{Text: "replacement final", IsFinal: true}}
	chunks, text := readRecoveryTranscript(t, output)
	if text != "replacement final" || chunks[len(chunks)-1].Ctrl.StreamID != "turn:asr:2" {
		t.Fatalf("replacement final suppressed by earlier transcript: %q %#v", text, chunks)
	}
}

func TestRecoveryDeadlineUnblocksReplayWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, second := newRecoveryProvider(), newRecoveryProvider()
	second.send = func(ctx context.Context, _ []byte, _ bool) error { <-ctx.Done(); return ctx.Err() }
	var opens atomic.Int32
	transformer := newTransformer(Config{EmitInterim: true, RealtimePacing: new(false)})
	transformer.recoveryTimeout = 20 * time.Millisecond
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		if opens.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}
	input := newBufferStream(4)
	output, _ := transformer.Transform(ctx, input)
	defer output.Close()
	pushRecoveryAudio(t, input, "turn", make([]byte, 3200), true, false)
	awaitRecovery(t, first.sent)
	first.events <- asrResult{err: recoveryFailure()}
	for {
		_, err := output.Next()
		if err != nil {
			if !errors.Is(err, errASRRecoveryTimeout) {
				t.Fatalf("replay deadline: %v", err)
			}
			break
		}
	}
	awaitRecovery(t, second.closed)
	if opens.Load() != 2 {
		t.Fatalf("attempts=%d", opens.Load())
	}
}

func TestRecoveryReplacementBOSUnblocksReplayWrite(t *testing.T) {
	for _, failAt := range []string{"receive", "send"} {
		t.Run(failAt, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			first, second, next := newRecoveryProvider(), newRecoveryProvider(), newRecoveryProvider()
			blocked := make(chan struct{})
			second.send = func(ctx context.Context, _ []byte, _ bool) error { close(blocked); <-ctx.Done(); return ctx.Err() }
			if failAt == "send" {
				first.send = func(context.Context, []byte, bool) error { return recoveryFailure() }
			}
			var opens atomic.Int32
			providers := []*recoveryProvider{first, second, next}
			transformer := newTransformer(Config{EmitInterim: true, RealtimePacing: new(false)})
			transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
				n := int(opens.Add(1))
				if n > len(providers) {
					return nil, fmt.Errorf("unexpected session %d", n)
				}
				return providers[n-1], nil
			}
			input := newBufferStream(8)
			output, _ := transformer.Transform(ctx, input)
			defer output.Close()
			pushRecoveryAudio(t, input, "old", make([]byte, 3200), true, false)
			if failAt == "receive" {
				awaitRecovery(t, first.sent)
				first.events <- asrResult{err: recoveryFailure()}
			}
			awaitRecovery(t, blocked)
			// Queue old suffix data ahead of the replacement BOS. It must be
			// discarded after preemption instead of opening another old session.
			pushRecoveryAudio(t, input, "old", make([]byte, 3200), false, false)
			pushRecoveryAudio(t, input, "new", make([]byte, 3200), true, false)
			awaitRecovery(t, second.closed)
			awaitRecovery(t, next.sent)
			next.events <- asrResult{value: recoveryResult("replacement", 0, 100)}
			chunks, text := readRecoveryTranscript(t, output)
			if text != "replacement" || chunks[len(chunks)-1].Ctrl.StreamID != "new" || opens.Load() != 3 {
				t.Fatalf("new generation=%q, opens=%d", text, opens.Load())
			}
		})
	}
}

func TestRecoveryDeadlineStillBoundsUploadAfterDefiniteOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, second := newRecoveryProvider(), newRecoveryProvider()
	second.send = func(ctx context.Context, _ []byte, _ bool) error {
		second.events <- asrResult{value: recoveryResult("already committed", 0, 100)}
		<-ctx.Done()
		return ctx.Err()
	}
	var opens atomic.Int32
	transformer := newTransformer(Config{EmitInterim: true, RealtimePacing: new(false)})
	transformer.recoveryTimeout = 40 * time.Millisecond
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		if opens.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}
	input := newBufferStream(4)
	output, _ := transformer.Transform(ctx, input)
	defer output.Close()
	pushRecoveryAudio(t, input, "turn", make([]byte, 3200), true, false)
	awaitRecovery(t, first.sent)
	first.events <- asrResult{err: recoveryFailure()}
	_, text := readRecoveryTranscript(t, output)
	if text != "already committed" {
		t.Fatal(text)
	}
	for {
		_, err := output.Next()
		if err != nil {
			if !errors.Is(err, errASRRecoveryTimeout) {
				t.Fatalf("unbounded upload: %v", err)
			}
			break
		}
	}
	if opens.Load() != 2 {
		t.Fatalf("retried after partial output: %d", opens.Load())
	}
	awaitRecovery(t, second.closed)
}

func TestTransformerBoundsInputReadAheadDuringRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, second := newRecoveryProvider(), newRecoveryProvider()
	blocked := make(chan struct{})
	second.send = func(ctx context.Context, _ []byte, _ bool) error { close(blocked); <-ctx.Done(); return ctx.Err() }
	var opens atomic.Int32
	transformer := newTransformer(Config{EmitInterim: true, RealtimePacing: new(false)})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		if opens.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}
	input := newBufferStream(4)
	output, _ := transformer.Transform(ctx, input)
	defer output.Close()
	pushRecoveryAudio(t, input, "turn", make([]byte, 3200), true, false)
	awaitRecovery(t, first.sent)
	first.events <- asrResult{err: recoveryFailure()}
	awaitRecovery(t, blocked)
	pushRecoveryAudio(t, input, "turn", make([]byte, asrRecoveryByteLimit+1), false, false)
	for {
		_, err := output.Next()
		if err != nil {
			if !errors.Is(err, errASRInputQueueLimit) {
				t.Fatalf("read-ahead overflow=%v", err)
			}
			break
		}
	}
	awaitRecovery(t, second.closed)
}

func TestRecoverySuccessReleasesDeadlineForNextUtterance(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, second := newRecoveryProvider(), newRecoveryProvider()
	var opens atomic.Int32
	route := newDoubaoASRRouteState(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn"}})
	session, err := newRecoveringSession(ctx, doubaoASRSessionConfig{format: "pcm", sampleRate: 16000, channels: 1, bits: 16}, route, true, 100*time.Millisecond, func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		if opens.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	read := make(chan *doubaospeech.ASRV2Result, 2)
	go func() {
		for result := range session.Recv() {
			read <- result
		}
	}()
	if err := session.SendAudio(ctx, make([]byte, 3200), false); err != nil {
		t.Fatal(err)
	}
	first.events <- asrResult{err: recoveryFailure()}
	awaitRecovery(t, second.sent)
	second.events <- asrResult{value: recoveryResult("recovered", 0, 100)}
	awaitRecovery(t, read)
	select {
	case <-second.closed:
		t.Fatal("recovery deadline closed the healthy replacement")
	case <-time.After(150 * time.Millisecond):
	}
	if err := session.SendAudio(ctx, make([]byte, 3200), false); err != nil {
		t.Fatal(err)
	}
	second.events <- asrResult{value: recoveryResult("next", 100, 200)}
	if result := awaitRecovery(t, read); result.Text != "next" {
		t.Fatalf("next utterance=%#v", result)
	}
}

func TestRecoveryPushToTalkReplaysOnceAndCommitsOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, second := newRecoveryProvider(), newRecoveryProvider()
	var opens atomic.Int32
	first.send = func(_ context.Context, _ []byte, last bool) error {
		if last {
			first.events <- asrResult{err: recoveryFailure()}
		}
		return nil
	}
	second.send = func(_ context.Context, _ []byte, last bool) error {
		if last {
			r := recoveryResult("answer", 0, 100)
			r.IsFinal = true
			second.events <- asrResult{value: r}
			close(second.events)
		}
		return nil
	}
	transformer := newTransformer(Config{RealtimePacing: new(false)})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		if opens.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}
	input := newBufferStream(4)
	output, _ := transformer.Transform(ctx, input)
	defer output.Close()
	audio := bytes.Repeat([]byte{9}, 3200)
	pushRecoveryAudio(t, input, "ptt", audio, true, false)
	pushRecoveryAudio(t, input, "ptt", nil, false, true)
	_ = input.Close()
	chunks := collectTransformerChunks(t, output)
	var texts []string
	bos, eos, audioBytes := 0, 0, 0
	for _, chunk := range chunks {
		if chunk.Ctrl.Label == "transcript" {
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				texts = append(texts, string(text))
			}
			if chunk.IsBeginOfStream() {
				bos++
			}
			if chunk.IsEndOfStream() {
				eos++
			}
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			audioBytes += len(blob.Data)
		}
	}
	if !slices.Equal(texts, []string{"answer"}) || bos != 1 || eos != 1 || audioBytes != 3200 || opens.Load() != 2 {
		t.Fatalf("texts=%v boundaries=%d/%d audio=%d opens=%d", texts, bos, eos, audioBytes, opens.Load())
	}
	if got := awaitRecovery(t, second.sent); !bytes.Equal(got.data, audio) || got.isLast {
		t.Fatalf("replay=%#v", got)
	}
	if got := awaitRecovery(t, second.sent); len(got.data) != 0 || !got.isLast {
		t.Fatalf("final marker=%#v", got)
	}
}

func TestRecoveryCancellationAndLateGeneration(t *testing.T) {
	for _, action := range []string{"cancel", "close", "close_error", "interrupt"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
			defer cancel()
			first, late, next := newRecoveryProvider(), newRecoveryProvider(), newRecoveryProvider()
			var opens atomic.Int32
			opening, retired := make(chan struct{}), make(chan struct{})
			transformer := newTransformer(Config{EmitInterim: true, RealtimePacing: new(false)})
			transformer.newSession = func(ctx context.Context, _ doubaoASRSessionConfig) (doubaoASRSession, error) {
				switch opens.Add(1) {
				case 1:
					return first, nil
				case 2:
					close(opening)
					<-ctx.Done()
					close(retired)
					return late, nil
				default:
					return next, nil
				}
			}
			input := newBufferStream(4)
			output, _ := transformer.Transform(ctx, input)
			defer output.Close()
			pushRecoveryAudio(t, input, "old", make([]byte, 3200), true, false)
			awaitRecovery(t, first.sent)
			first.events <- asrResult{err: recoveryFailure()}
			awaitRecovery(t, opening)
			switch action {
			case "cancel":
				cancel()
			case "close":
				_ = output.Close()
			case "close_error":
				_ = output.CloseWithError(errors.New("disconnected"))
			case "interrupt":
				pushRecoveryAudio(t, input, "new", make([]byte, 3200), true, false)
			}
			awaitRecovery(t, retired)
			awaitRecovery(t, late.closed)
			if action == "interrupt" {
				awaitRecovery(t, next.sent)
				next.events <- asrResult{value: recoveryResult("new generation", 0, 100)}
				_, text := readRecoveryTranscript(t, output)
				if text != "new generation" {
					t.Fatalf("replacement=%q", text)
				}
			} else if opens.Load() != 2 {
				t.Fatalf("opened after cancel: %d", opens.Load())
			}
		})
	}
}

func TestRecoverySkipsUnsafeOrNormalTerminals(t *testing.T) {
	for _, scenario := range []string{"committed_ptt", "missing_timestamp", "audio_limit", "packet_wait", "auth", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			provider := newRecoveryProvider()
			var opens atomic.Int32
			transformer := newTransformer(Config{EmitInterim: scenario != "committed_ptt", RealtimePacing: new(false)})
			transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
				opens.Add(1)
				return provider, nil
			}
			input := newBufferStream(4)
			output, _ := transformer.Transform(ctx, input)
			defer output.Close()
			pushRecoveryAudio(t, input, "turn", make([]byte, 3200), true, false)
			awaitRecovery(t, provider.sent)
			switch scenario {
			case "committed_ptt":
				provider.events <- asrResult{value: recoveryResult("already visible", 0, 100)}
				for {
					c, err := output.Next()
					if err != nil {
						t.Fatal(err)
					}
					if text, ok := c.Part.(genx.Text); ok && text != "" {
						break
					}
				}
			case "missing_timestamp":
				provider.events <- asrResult{value: recoveryResult("already committed", 0, 0)}
				readRecoveryTranscript(t, output)
			case "audio_limit":
				for range 151 {
					pushRecoveryAudio(t, input, "turn", make([]byte, 3200), false, false)
					awaitRecovery(t, provider.sent)
				}
			}
			switch scenario {
			case "packet_wait":
				provider.events <- asrResult{err: &doubaospeech.Error{Code: doubaoASRPacketWaitTimeout}}
			case "auth":
				provider.events <- asrResult{err: &doubaospeech.Error{Code: doubaospeech.CodeAuthError}}
			case "empty":
				close(provider.events)
			default:
				provider.events <- asrResult{err: recoveryFailure()}
			}
			if scenario == "packet_wait" || scenario == "empty" {
				pushRecoveryAudio(t, input, "turn", nil, false, true)
				_ = input.Close()
			}
			for {
				_, err := output.Next()
				if err != nil {
					break
				}
			}
			if opens.Load() != 1 {
				t.Fatalf("unsafe/normal terminal retried: %d", opens.Load())
			}
		})
	}
}

func TestRecoverySendFailureAndBoundedTimeout(t *testing.T) {
	for _, scenario := range []string{"send_failure", "silent_retry"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			first, second := newRecoveryProvider(), newRecoveryProvider()
			var opens atomic.Int32
			if scenario == "send_failure" {
				first.send = func(context.Context, []byte, bool) error { return recoveryFailure() }
			}
			transformer := newTransformer(Config{EmitInterim: true, RealtimePacing: new(false)})
			transformer.recoveryTimeout = 50 * time.Millisecond
			transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
				if opens.Add(1) == 1 {
					return first, nil
				}
				return second, nil
			}
			input := newBufferStream(4)
			output, _ := transformer.Transform(ctx, input)
			defer output.Close()
			pushRecoveryAudio(t, input, "turn", make([]byte, 3200), true, false)
			if scenario != "send_failure" {
				awaitRecovery(t, first.sent)
				first.events <- asrResult{err: recoveryFailure()}
			}
			awaitRecovery(t, second.sent)
			if scenario == "send_failure" {
				second.events <- asrResult{value: recoveryResult("recovered", 0, 100)}
				_, text := readRecoveryTranscript(t, output)
				if text != "recovered" {
					t.Fatal(text)
				}
			} else {
				for {
					_, err := output.Next()
					if err != nil {
						if !errors.Is(err, errASRRecoveryTimeout) {
							t.Fatalf("timeout=%v", err)
						}
						break
					}
				}
				awaitRecovery(t, second.closed)
			}
		})
	}
}

type recoveryLogHandler struct{ records chan slog.Record }

func (*recoveryLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}
func (h *recoveryLogHandler) Handle(ctx context.Context, r slog.Record) error {
	if gizlog.SessionID(ctx) != "session" {
		return nil
	}
	r = r.Clone()
	r.AddAttrs(slog.String("request_id", gizlog.RequestID(ctx)), slog.String("session_id", gizlog.SessionID(ctx)))
	h.records <- r
	return nil
}
func (h *recoveryLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recoveryLogHandler) WithGroup(string) slog.Handler      { return h }

func TestRecoveryLogsOriginalStartAndSuccessAtWarn(t *testing.T) {
	handler := &recoveryLogHandler{records: make(chan slog.Record, 16)}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(previous)
	ctx := gizlog.WithSessionID(gizlog.WithRequestID(t.Context(), "gizclaw-request"), "session")
	first, second := newRecoveryProvider(), newRecoveryProvider()
	var opens atomic.Int32
	route := newDoubaoASRRouteState(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn"}})
	session, err := newRecoveringSession(ctx, doubaoASRSessionConfig{format: "pcm", sampleRate: 16000, channels: 1, bits: 16}, route, true, time.Second, func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		if opens.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	read := make(chan *doubaospeech.ASRV2Result, 1)
	go func() {
		for result := range session.Recv() {
			read <- result
		}
	}()
	if err := session.SendAudio(ctx, make([]byte, 3200), false); err != nil {
		t.Fatal(err)
	}
	first.events <- asrResult{err: recoveryFailure()}
	awaitRecovery(t, second.sent)
	second.events <- asrResult{value: recoveryResult("private transcript", 0, 100)}
	awaitRecovery(t, read)
	var id string
	for i, event := range []string{"attempt failed", "retry start", "retry success"} {
		record := awaitRecovery(t, handler.records)
		attrs := map[string]any{}
		record.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.Any(); return true })
		if record.Level != slog.LevelWarn || record.Message != "doubao asr: "+event || attrs["request_id"] != "gizclaw-request" || attrs["session_id"] != "session" {
			t.Fatalf("log=%#v %v", record, attrs)
		}
		retry := int64(1)
		if i == 0 {
			retry = 0
		}
		if attrs["retry"] != retry {
			t.Fatalf("retry=%v", attrs)
		}
		if i == 0 {
			id = attrs["asr_request_id"].(string)
		}
		if len(id) != 32 || attrs["asr_request_id"] != id || attrs["segment_index"] != int64(1) {
			t.Fatalf("correlation=%v", attrs)
		}
		if i < 2 && (attrs["error_code"] != int64(asrServerError) || attrs["upstream_reqid"] != "asr-original" || attrs["upstream_connect_id"] != "asr-original" || attrs["upstream_trace_id"] != "") {
			t.Fatalf("provider metadata=%v", attrs)
		}
		if strings.Contains(fmt.Sprint(attrs), "private transcript") {
			t.Fatal("transcript leaked in retry log")
		}
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if got := session.AudioDuration(); got != 200*time.Millisecond {
		t.Fatalf("billed retry usage=%s", got)
	}
}
