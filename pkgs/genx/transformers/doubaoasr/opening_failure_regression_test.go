package doubaoasr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
)

type openingFailureAgent struct{}

func (openingFailureAgent) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	return input, nil
}

func TestOpeningFailureReachesDockWithCallerInputOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	asr := newTransformer(Config{RealtimePacing: new(false)})
	var opens atomic.Int32
	asr.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		opens.Add(1)
		return nil, fmt.Errorf("websocket connect failed: %w", io.EOF)
	}
	dock, err := audiodock.New(audiodock.Config{ASR: asr, Agent: openingFailureAgent{}})
	if err != nil {
		t.Fatal(err)
	}
	input := newBufferStream(4)
	defer input.Close()
	output, err := dock.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	pushRecoveryAudio(t, input, "failed-open", nil, true, false)
	terminal := make(chan error, 1)
	go func() {
		for {
			_, err := output.Next()
			if err != nil {
				terminal <- err
				return
			}
		}
	}()
	select {
	case err := <-terminal:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("lost provider cause: %v", err)
		}
		code, _, _, ok := genx.PublicErrorDetails(err)
		if !ok || code != "ASR_STREAM_ERROR" {
			t.Fatalf("lost public ASR failure: %q %v", code, err)
		}
		if opens.Load() != 2 {
			t.Fatalf("opens=%d, want one retry", opens.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("provider startup failure was swallowed while caller input stayed open")
	}
}

func TestOpeningEOFRetryControls(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cause     error
		wantOpens int
		recover   bool
	}{
		{"transient", fmt.Errorf("connect: %w", io.EOF), 2, true},
		{"exhausted", fmt.Errorf("connect: %w", io.EOF), 2, false},
		{"permanent", errors.New("invalid provider configuration"), 1, false},
		{"canceled", context.Canceled, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var opens int
			provider := newRecoveryProvider()
			defer provider.Close()
			session, err := newRecoveringSession(t.Context(), doubaoASRSessionConfig{format: "pcm", sampleRate: 16000, channels: 1, bits: 16}, newDoubaoASRRouteState(nil), false, time.Second,
				func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
					opens++
					if tc.recover && opens == 2 {
						return provider, nil
					}
					return nil, tc.cause
				})
			if tc.recover {
				if err != nil {
					t.Fatal(err)
				}
				defer session.Close()
				if err := session.SendAudio(t.Context(), []byte{1, 2}, false); err != nil {
					t.Fatal(err)
				}
				sent := awaitRecovery(t, provider.sent)
				if len(sent.data) != 2 {
					t.Fatalf("audio not accepted by replacement: %#v", sent)
				}
			} else if !errors.Is(err, tc.cause) {
				t.Fatalf("error=%v, want cause %v", err, tc.cause)
			}
			if opens != tc.wantOpens {
				t.Fatalf("opens=%d, want %d", opens, tc.wantOpens)
			}
		})
	}
}

func TestOpeningEOFRetryCancellationAndBudget(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		t.Run(fmt.Sprint("parent_cancel=", cancelParent), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var opens int
			_, err := newRecoveringSession(ctx, doubaoASRSessionConfig{format: "pcm", sampleRate: 16000, channels: 1, bits: 16}, newDoubaoASRRouteState(nil), false, 20*time.Millisecond,
				func(attempt context.Context, _ doubaoASRSessionConfig) (doubaoASRSession, error) {
					opens++
					if opens == 1 {
						return nil, fmt.Errorf("connect: %w", io.EOF)
					}
					if cancelParent {
						cancel()
					}
					<-attempt.Done()
					return nil, context.Cause(attempt)
				})
			want := errASRRecoveryTimeout
			if cancelParent {
				want = context.Canceled
			}
			if !errors.Is(err, want) || opens != 2 {
				t.Fatalf("opens=%d error=%v, want %v", opens, err, want)
			}
		})
	}
}

func TestMidstreamEOFDoesNotRetry(t *testing.T) {
	provider := newRecoveryProvider()
	var opens atomic.Int32
	session, err := newRecoveringSession(t.Context(), doubaoASRSessionConfig{format: "pcm", sampleRate: 16000, channels: 1, bits: 16}, newDoubaoASRRouteState(nil), false, time.Second,
		func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
			opens.Add(1)
			return provider, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	provider.events <- asrResult{err: fmt.Errorf("receive: %w", io.EOF)}
	terminal := make(chan error, 1)
	go func() {
		for _, err := range session.Recv() {
			if err != nil {
				terminal <- err
				return
			}
		}
	}()
	if err := awaitRecovery(t, terminal); !errors.Is(err, io.EOF) {
		t.Fatalf("lost midstream cause: %v", err)
	}
	if opens.Load() != 1 {
		t.Fatalf("retried midstream EOF: %d opens", opens.Load())
	}
}
