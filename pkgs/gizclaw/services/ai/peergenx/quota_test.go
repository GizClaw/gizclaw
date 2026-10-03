package peergenx

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
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
