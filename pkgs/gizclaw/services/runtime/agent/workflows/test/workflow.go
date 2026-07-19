package testworkflow

import (
	"context"
	"errors"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agent"
)

const Type = "test"

type Workflow struct {
	Prefix string
}

var _ agent.Factory = (*Workflow)(nil)

func (w *Workflow) NewAgent(context.Context, agent.Spec) (genx.Transformer, error) {
	prefix := w.Prefix
	if prefix == "" {
		prefix = "test: "
	}
	return transformer{prefix: prefix}, nil
}

type transformer struct {
	prefix string
}

func (t transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if input == nil {
		return nil, errors.New("test workflow: input stream is required")
	}
	return &stream{ctx: ctx, input: input, prefix: t.prefix}, nil
}

type stream struct {
	ctx    context.Context
	input  genx.Stream
	prefix string
}

func (s *stream) Next() (*genx.MessageChunk, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	chunk, err := s.input.Next()
	if err != nil {
		return nil, err
	}
	if chunk == nil {
		return nil, nil
	}
	out := chunk.Clone()
	if text, ok := out.Part.(genx.Text); ok {
		out.Role = genx.RoleModel
		out.Part = genx.Text(s.prefix + string(text))
	}
	return out, nil
}

func (s *stream) Close() error {
	return s.input.Close()
}

func (s *stream) CloseWithError(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
		return s.Close()
	}
	return s.input.CloseWithError(err)
}
