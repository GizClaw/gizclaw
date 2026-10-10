package peergenx

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type usageBuilder struct {
	base   Builder
	record genx.UsageRecorder
}

func (b usageBuilder) BuildGenerator(ctx context.Context, cfg GeneratorConfig) (genx.Generator, error) {
	impl, err := b.base.BuildGenerator(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return usageGenerator{Generator: impl, record: b.record}, nil
}
func (b usageBuilder) BuildTransformer(ctx context.Context, cfg TransformerConfig) (genx.Transformer, error) {
	impl, err := b.base.BuildTransformer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return usageTransformer{Transformer: impl, record: b.record}, nil
}

type usageParentKey struct{}

// providerUsageContext preserves a caller's recorder while replacing nested
// product metering scopes, so a nested provider call is attributed once.
func providerUsageContext(ctx context.Context, record genx.UsageRecorder) context.Context {
	parent := ctx
	if original, ok := ctx.Value(usageParentKey{}).(context.Context); ok {
		parent = original
	}
	child := genx.WithUsageRecorder(ctx, func(item genx.UsageRecord) { genx.RecordUsage(parent, item); record(item) })
	return context.WithValue(child, usageParentKey{}, parent)
}

type usageGenerator struct {
	genx.Generator
	record genx.UsageRecorder
}

func (g usageGenerator) GenerateStream(ctx context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
	return g.Generator.GenerateStream(providerUsageContext(ctx, g.record), pattern, mctx)
}
func (g usageGenerator) Invoke(ctx context.Context, pattern string, mctx genx.ModelContext, tool *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return g.Generator.Invoke(providerUsageContext(ctx, g.record), pattern, mctx, tool)
}

func (g usageGenerator) TranscribeInput(ctx context.Context, pattern string, input genx.ModelContext) (string, genx.Usage, error) {
	return transcribeInput(providerUsageContext(ctx, g.record), g.Generator, pattern, input)
}

type usageTransformer struct {
	genx.Transformer
	record genx.UsageRecorder
}

func (t usageTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return t.Transformer.Transform(providerUsageContext(ctx, t.record), input)
}
