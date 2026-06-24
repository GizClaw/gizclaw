package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

var (
	ErrNilService      = errors.New("agent: nil service")
	ErrMissingHost     = errors.New("agent: host is required")
	ErrMissingPattern  = errors.New("agent: pattern source is required")
	ErrMissingSource   = errors.New("agent: stream source is required")
	ErrMissingConsumer = errors.New("agent: stream consumer is required")
)

type PatternSource interface {
	AgentPattern(context.Context) (string, error)
}

type PatternSourceFunc func(context.Context) (string, error)

func (f PatternSourceFunc) AgentPattern(ctx context.Context) (string, error) {
	return f(ctx)
}

type StreamSource interface {
	OpenAgentInput(context.Context) (genx.Stream, error)
}

type StreamSourceFunc func(context.Context) (genx.Stream, error)

func (f StreamSourceFunc) OpenAgentInput(ctx context.Context) (genx.Stream, error) {
	return f(ctx)
}

type StreamConsumer interface {
	ConsumeAgentOutput(context.Context, genx.Stream) error
}

type StreamConsumerFunc func(context.Context, genx.Stream) error

func (f StreamConsumerFunc) ConsumeAgentOutput(ctx context.Context, stream genx.Stream) error {
	return f(ctx, stream)
}

// Service manages the per-connection agent runtime. Reload tears down the
// current pipeline and builds a fresh one from the current workspace pattern.
type Service struct {
	Host     *Host
	Pattern  PatternSource
	Source   StreamSource
	Consumer StreamConsumer
	Logger   *slog.Logger

	mu      sync.Mutex
	runtime *runtime
}

func (s *Service) Reload(ctx context.Context) error {
	if s == nil {
		return ErrNilService
	}
	if err := s.validate(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	pattern, err := s.Pattern.AgentPattern(ctx)
	if err != nil {
		return fmt.Errorf("agent: resolve pattern: %w", err)
	}
	input, err := s.Source.OpenAgentInput(ctx)
	if err != nil {
		return fmt.Errorf("agent: open input stream: %w", err)
	}
	if input == nil {
		return fmt.Errorf("agent: input stream is required")
	}

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	output, err := s.Host.Transform(runCtx, pattern, input)
	if err != nil {
		cancel()
		_ = input.CloseWithError(err)
		return err
	}
	next := &runtime{
		cancel: cancel,
		input:  input,
		output: output,
		done:   make(chan struct{}),
	}

	previous := s.swap(next)
	go s.consume(runCtx, next)
	if err := previous.stop(ctx); err != nil {
		return fmt.Errorf("agent: stop previous runtime: %w", err)
	}
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	current := s.runtime
	s.runtime = nil
	s.mu.Unlock()
	return current.stop(ctx)
}

func (s *Service) validate() error {
	switch {
	case s.Host == nil:
		return ErrMissingHost
	case s.Pattern == nil:
		return ErrMissingPattern
	case s.Source == nil:
		return ErrMissingSource
	case s.Consumer == nil:
		return ErrMissingConsumer
	default:
		return nil
	}
}

func (s *Service) swap(next *runtime) *runtime {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.runtime
	s.runtime = next
	return previous
}

func (s *Service) consume(ctx context.Context, rt *runtime) {
	defer close(rt.done)
	err := s.Consumer.ConsumeAgentOutput(ctx, rt.output)
	if err != nil && ctx.Err() == nil {
		s.logger().Error("agent: output consumer failed", "error", err)
	}
}

func (s *Service) logger() *slog.Logger {
	if s != nil && s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

type runtime struct {
	cancel context.CancelFunc
	input  genx.Stream
	output genx.Stream
	done   chan struct{}
}

func (r *runtime) stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.cancel()
	err := errors.Join(r.output.Close(), r.input.Close())
	select {
	case <-r.done:
		return err
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}
