// Package doubaochat adapts Volc Ark Doubao chat completions to GenX user
// audio.
//
// Doubao Seed chat models accept audio input but their API returns no
// transcript of it. Generator converts request audio to a format the API
// accepts, asks the model to begin its reply with <asr>verbatim
// transcript</asr>, and reports that transcript as a genx input transcript
// chunk ahead of the reply text.
package doubaochat

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// transcriptInstruction is the system prompt appended to a request whose
// latest user message carries audio. The reply comes first so the transcript
// does not delay the first reply text; transcriptFilter removes the <asr>
// segment wherever the model places it.
const transcriptInstruction = "The user's latest message is audio. Never start your response with a tag. " +
	"First write one complete sentence of your reply to the user. " +
	"Then write one separate line <asr>verbatim transcript of that audio</asr>, in the language or dialect actually spoken, " +
	"without translation or correction (<asr></asr> when there is no speech). " +
	"Then continue your reply on the next line if there is more to say. The transcript line is not part of your reply."

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

// GenerateStream converts request audio. When the latest user message is
// audio, it removes the model's <asr> segment from the reply and reports that
// transcript as one input transcript chunk as soon as the segment closes. A
// reply without the segment keeps its text and reports no transcript.
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
	stream, err := g.next.GenerateStream(ctx, pattern, withTranscriptInstruction(prepared))
	if err != nil {
		return nil, err
	}
	return &transcriptStream{ctx: ctx, next: stream}, nil
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

func withTranscriptInstruction(mctx genx.ModelContext) genx.ModelContext {
	builder := copyModelContext(mctx)
	builder.Prompts = append(builder.Prompts, &genx.Prompt{Name: "doubaochat_transcript", Text: transcriptInstruction})
	return builder.Build()
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

// transcriptStream reports the <asr> transcript as one input transcript chunk
// and passes the remaining model output through.
type transcriptStream struct {
	ctx      context.Context
	next     genx.Stream
	filter   transcriptFilter
	reported bool
	pending  []*genx.MessageChunk
	finished bool
	endErr   error
}

func (s *transcriptStream) Next() (*genx.MessageChunk, error) {
	for {
		if len(s.pending) != 0 {
			chunk := s.pending[0]
			s.pending = s.pending[1:]
			return chunk, nil
		}
		if s.finished {
			return nil, s.endErr
		}
		chunk, err := s.next.Next()
		if err != nil {
			s.finished, s.endErr = true, err
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				s.flush()
			}
			continue
		}
		if chunk == nil {
			continue
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok || chunk.Role != genx.RoleModel {
			s.pending = append(s.pending, chunk)
			continue
		}
		before, transcript, closed, after := s.filter.consume(string(text))
		if before != "" {
			s.pending = append(s.pending, textChunk(chunk, before))
		}
		if closed {
			s.reported = true
			s.pending = append(s.pending, genx.NewInputTranscriptChunk(transcript))
		}
		if after != "" || chunk.IsEndOfStream() && before == "" {
			s.pending = append(s.pending, textChunk(chunk, after))
		}
	}
}

func (s *transcriptStream) flush() {
	held, transcript, closed := s.filter.finish()
	if held != "" {
		s.pending = append(s.pending, &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(held)})
	}
	if closed && !s.reported {
		s.reported = true
		s.pending = append(s.pending, genx.NewInputTranscriptChunk(transcript))
	}
	if !s.reported {
		// The reply stays usable; the turn only loses its user text.
		slog.WarnContext(s.ctx, "doubaochat: model reply carried no <asr> transcript")
	}
}

func textChunk(source *genx.MessageChunk, text string) *genx.MessageChunk {
	chunk := source.Clone()
	chunk.Part = genx.Text(text)
	return chunk
}

func (s *transcriptStream) Close() error {
	return s.next.Close()
}

func (s *transcriptStream) CloseWithError(err error) error {
	return s.next.CloseWithError(err)
}

var _ genx.Stream = (*transcriptStream)(nil)
