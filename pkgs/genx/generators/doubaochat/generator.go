// Package doubaochat adapts Volc Ark Doubao chat completions to GenX user
// audio. Audio turns run two independent requests in parallel: the caller's
// reply and an adapter-owned transcription of the current audio. One stream
// carries the unchanged reply and a separate GenX input-transcript chunk.
package doubaochat

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

var _ genx.Generator = (*Generator)(nil)

// Generator wraps the OpenAI-compatible Generator of one Doubao chat model.
type Generator struct {
	next genx.Generator
}

// New wraps next, which must send requests to a Doubao chat completions model
// that accepts audio input.
func New(next genx.Generator) *Generator {
	return &Generator{next: next}
}

// GenerateStream converts request audio. An audio turn sends the original
// prompts and history to the reply request, while a parallel request receives
// only the current audio and the adapter's transcription instruction. Reply
// text is never parsed for ASR tags. Both requests share cancellation and the
// returned stream reports transcription separately from the visible reply.
func (g *Generator) GenerateStream(ctx context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
	if g == nil || g.next == nil {
		return nil, errors.New("doubaochat: Generator is nil")
	}
	prepared, audioTurn, err := prepareModelContext(mctx)
	if err != nil {
		return nil, err
	}
	if !audioTurn {
		return g.next.GenerateStream(ctx, pattern, prepared)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	transcription, err := audioTranscriptContext(prepared)
	if err != nil {
		return nil, err
	}
	return newAudioStream(ctx, g.next, pattern, prepared, transcription), nil
}

// Invoke converts request audio and delegates the function call unchanged.
func (g *Generator) Invoke(ctx context.Context, pattern string, mctx genx.ModelContext, fn *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	if g == nil || g.next == nil {
		return genx.Usage{}, nil, errors.New("doubaochat: Generator is nil")
	}
	prepared, _, err := prepareModelContext(mctx)
	if err != nil {
		return genx.Usage{}, nil, err
	}
	return g.next.Invoke(ctx, pattern, prepared, fn)
}

func copyModelContext(mctx genx.ModelContext) *genx.ModelContextBuilder {
	builder := &genx.ModelContextBuilder{Params: mctx.Params()}
	for prompt := range mctx.Prompts() {
		builder.Prompts = append(builder.Prompts, prompt)
	}
	for message := range mctx.Messages() {
		builder.Messages = append(builder.Messages, message)
	}
	for cot := range mctx.CoTs() {
		builder.CoTs = append(builder.CoTs, cot)
	}
	for tool := range mctx.Tools() {
		builder.Tools = append(builder.Tools, tool)
	}
	return builder
}
