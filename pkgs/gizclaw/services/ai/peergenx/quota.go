package peergenx

import (
	"context"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type quotaBuilder struct {
	base      Builder
	authorize func(context.Context) (context.Context, func(), error)
}

func (b quotaBuilder) BuildGenerator(ctx context.Context, config GeneratorConfig) (genx.Generator, error) {
	base, err := b.base.BuildGenerator(ctx, config)
	if err != nil {
		return nil, err
	}
	return quotaGenerator{Generator: base, authorize: b.authorize}, nil
}
func (b quotaBuilder) BuildTransformer(ctx context.Context, config TransformerConfig) (genx.Transformer, error) {
	base, err := b.base.BuildTransformer(ctx, config)
	if err != nil {
		return nil, err
	}
	return quotaTransformer{Transformer: base, authorize: b.authorize}, nil
}

type quotaGenerator struct {
	genx.Generator
	authorize func(context.Context) (context.Context, func(), error)
}

func (g quotaGenerator) GenerateStream(ctx context.Context, pattern string, input genx.ModelContext) (genx.Stream, error) {
	ctx, release, err := g.authorize(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := g.Generator.GenerateStream(ctx, pattern, input)
	if err != nil || stream == nil {
		release()
		return stream, err
	}
	return newQuotaStream(ctx, stream, release), nil
}
func (g quotaGenerator) Invoke(ctx context.Context, pattern string, input genx.ModelContext, tool *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	ctx, release, err := g.authorize(ctx)
	if err != nil {
		return genx.Usage{}, nil, err
	}
	defer release()
	return g.Generator.Invoke(ctx, pattern, input, tool)
}

type quotaTransformer struct {
	genx.Transformer
	authorize func(context.Context) (context.Context, func(), error)
}

func (t quotaTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	ctx, release, err := t.authorize(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := t.Transformer.Transform(ctx, input)
	if err != nil || stream == nil {
		release()
		return stream, err
	}
	return newQuotaStream(ctx, stream, release), nil
}

type quotaStream struct {
	genx.Stream
	ctx     context.Context
	release func()
	once    sync.Once
	stop    func() bool
	done    chan struct{}
}

func newQuotaStream(ctx context.Context, stream genx.Stream, release func()) *quotaStream {
	s := &quotaStream{Stream: stream, ctx: ctx, release: release, done: make(chan struct{})}
	s.stop = context.AfterFunc(ctx, func() { defer close(s.done); _ = stream.CloseWithError(context.Cause(ctx)) })
	return s
}
func (s *quotaStream) finish() {
	s.once.Do(func() {
		if !s.stop() {
			<-s.done
		}
		s.release()
	})
}
func (s *quotaStream) Next() (*genx.MessageChunk, error) {
	if s.ctx.Err() != nil {
		s.finish()
		return nil, context.Cause(s.ctx)
	}
	chunk, err := s.Stream.Next()
	if s.ctx.Err() != nil {
		err = context.Cause(s.ctx)
		chunk = nil
	}
	if err != nil {
		s.finish()
	}
	return chunk, err
}
func (s *quotaStream) Close() error { s.finish(); return s.Stream.Close() }
func (s *quotaStream) CloseWithError(err error) error {
	s.finish()
	return s.Stream.CloseWithError(err)
}
