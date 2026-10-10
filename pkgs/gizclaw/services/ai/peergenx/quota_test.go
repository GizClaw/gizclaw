package peergenx

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerquota"
)

type quotaTestTransformer struct{ called *atomic.Int32 }

func (t quotaTestTransformer) Transform(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
	t.called.Add(1)
	return &quotaTestStream{ctx: ctx}, nil
}

type quotaTestStream struct{ ctx context.Context }

func (s *quotaTestStream) Next() (*genx.MessageChunk, error) { <-s.ctx.Done(); return nil, s.ctx.Err() }
func (*quotaTestStream) Close() error                        { return nil }
func (*quotaTestStream) CloseWithError(error) error          { return nil }

func TestQuotaStopsBeforeProviderAndCancelsItsActiveStream(t *testing.T) {
	var called atomic.Int32
	denied := quotaTransformer{Transformer: quotaTestTransformer{&called}, authorize: func(context.Context) (context.Context, func(), error) { return nil, nil, ErrDenied }}
	if _, err := denied.Transform(t.Context(), nil); !errors.Is(err, ErrDenied) || called.Load() != 0 {
		t.Fatalf("denial error=%v calls=%d", err, called.Load())
	}
	call, cancel := context.WithCancel(t.Context())
	var released atomic.Int32
	accepted := quotaTransformer{Transformer: quotaTestTransformer{&called}, authorize: func(context.Context) (context.Context, func(), error) {
		return call, func() { released.Add(1); cancel() }, nil
	}}
	stream, err := accepted.Transform(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := stream.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("terminal=%v", err)
	}
	if err := stream.Close(); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if called.Load() != 1 || released.Load() != 1 {
		t.Fatalf("calls=%d releases=%d", called.Load(), released.Load())
	}
}

type detachedQuotaStream struct{ terminal chan error }

func (s *detachedQuotaStream) Next() (*genx.MessageChunk, error) { return nil, <-s.terminal }
func (s *detachedQuotaStream) Close() error                      { return s.CloseWithError(io.EOF) }
func (s *detachedQuotaStream) CloseWithError(err error) error {
	select {
	case s.terminal <- err:
	default:
	}
	return nil
}
func TestQuotaClosesOutputThatDoesNotObserveProviderContext(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	revoked := errors.New("quota revoked")
	var releases atomic.Int32
	stream := newQuotaStream(ctx, &detachedQuotaStream{terminal: make(chan error, 1)}, func() { releases.Add(1) })
	result := make(chan error, 1)
	go func() { _, err := stream.Next(); result <- err }()
	cancel(revoked)
	select {
	case err := <-result:
		if !errors.Is(err, revoked) {
			t.Fatalf("terminal=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("quota did not unblock output")
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if releases.Load() != 1 {
		t.Fatalf("releases=%d", releases.Load())
	}
}

type openingQuotaProvider struct {
	cancel context.CancelCauseFunc
	cause  error
	late   *openingQuotaStream
}

func (p openingQuotaProvider) GenerateStream(ctx context.Context, _ string, _ genx.ModelContext) (genx.Stream, error) {
	p.cancel(p.cause)
	<-ctx.Done()
	if p.late != nil {
		return p.late, nil
	}
	return nil, ctx.Err()
}
func (p openingQuotaProvider) Transform(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
	return p.GenerateStream(ctx, "", nil)
}
func (p openingQuotaProvider) Invoke(ctx context.Context, _ string, _ genx.ModelContext, _ *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	p.cancel(p.cause)
	<-ctx.Done()
	return genx.Usage{PromptTokenCount: 5}, &genx.FuncCall{Name: "late"}, ctx.Err()
}

type openingQuotaStream struct {
	cause  error
	closes int
}

type observedQuotaFixture struct {
	openingQuotaStream
	deferred  bool
	observed  *genx.MessageChunk
	abandoned *genx.MessageChunk
}

func (s *observedQuotaFixture) DeferOutputObservation()                { s.deferred = true }
func (s *observedQuotaFixture) ObserveOutput(chunk *genx.MessageChunk) { s.observed = chunk }
func (s *observedQuotaFixture) AbandonOutputObservation(chunk *genx.MessageChunk) {
	s.abandoned = chunk
}

func TestQuotaPreservesProviderDeliveryObservation(t *testing.T) {
	provider := &observedQuotaFixture{}
	stream := newQuotaStream(t.Context(), provider, func() {})
	defer stream.Close()
	observer, ok := stream.(quotaOutputObserver)
	if !ok {
		t.Fatal("quota hid provider delivery observation")
	}
	chunk := &genx.MessageChunk{Part: genx.Text("spoken")}
	observer.DeferOutputObservation()
	observer.ObserveOutput(chunk)
	observer.AbandonOutputObservation(chunk)
	if !provider.deferred || provider.observed != chunk || provider.abandoned != chunk {
		t.Fatalf("quota did not forward exact delivery identities: %+v", provider)
	}
	plain := newQuotaStream(t.Context(), &openingQuotaStream{}, func() {})
	defer plain.Close()
	if _, ok := plain.(quotaOutputObserver); ok {
		t.Fatal("quota advertised observation for an unsupported provider")
	}
}

func (*openingQuotaStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (s *openingQuotaStream) Close() error                    { s.closes++; return nil }
func (s *openingQuotaStream) CloseWithError(cause error) error {
	s.cause = cause
	s.closes++
	return nil
}

func TestQuotaPreservesCauseDuringProviderOpeningAndInvoke(t *testing.T) {
	for _, cause := range []error{peerquota.ErrDenied, peerquota.ErrUnavailable, context.Canceled} {
		for _, operation := range []string{"generate", "transform", "invoke", "late generate", "late transform"} {
			t.Run(operation+cause.Error(), func(t *testing.T) {
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(context.Canceled)
				releases := 0
				authorize := func(context.Context) (context.Context, func(), error) {
					return ctx, func() { releases++; cancel(context.Canceled) }, nil
				}
				provider := openingQuotaProvider{cancel: cancel, cause: cause}
				if operation == "late generate" || operation == "late transform" {
					provider.late = &openingQuotaStream{}
				}
				var output genx.Stream
				var err error
				switch operation {
				case "generate", "late generate":
					output, err = (quotaGenerator{Generator: provider, authorize: authorize}).GenerateStream(t.Context(), "", nil)
				case "transform", "late transform":
					output, err = (quotaTransformer{Transformer: provider, authorize: authorize}).Transform(t.Context(), nil)
				case "invoke":
					var usage genx.Usage
					var call *genx.FuncCall
					usage, call, err = (quotaGenerator{Generator: provider, authorize: authorize}).Invoke(t.Context(), "", nil, nil)
					if usage.PromptTokenCount != 5 || call != nil {
						t.Fatalf("usage=%+v call=%+v", usage, call)
					}
				}
				if !errors.Is(err, cause) || output != nil || releases != 1 {
					t.Fatalf("error=%v output=%T releases=%d", err, output, releases)
				}
				if provider.late != nil && (provider.late.closes != 1 || !errors.Is(provider.late.cause, cause)) {
					t.Fatalf("late stream=%+v", provider.late)
				}
			})
		}
	}
}

func TestQuotaStartupDoesNotReplaceUnrelatedProviderFailureDuringRelease(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	providerErr := errors.New("provider failure")
	_, err := finishQuotaStartup(ctx, nil, providerErr, func() { cancel(context.Canceled) })
	if !errors.Is(err, providerErr) {
		t.Fatalf("provider error was replaced: %v", err)
	}
}
