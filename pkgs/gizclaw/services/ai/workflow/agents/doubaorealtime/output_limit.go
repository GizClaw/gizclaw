package doubaorealtime

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type outputLimitTransformer struct {
	Transformer genx.Transformer
	MaxRunes    int
}

func (t outputLimitTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t.Transformer == nil {
		return nil, fmt.Errorf("doubaorealtime: output limiter transformer is required")
	}
	stream, err := t.Transformer.Transform(ctx, input)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, fmt.Errorf("doubaorealtime: output limiter source stream is required")
	}
	if t.MaxRunes <= 0 {
		return stream, nil
	}
	return &outputLimitStream{
		source: stream,
		limit:  t.MaxRunes,
		counts: make(map[string]int),
	}, nil
}

type outputLimitStream struct {
	source genx.Stream
	limit  int
	counts map[string]int
}

func (s *outputLimitStream) Next() (*genx.MessageChunk, error) {
	for {
		chunk, err := s.source.Next()
		if err != nil || chunk == nil {
			return chunk, err
		}
		text, limited := limitedAssistantText(chunk)
		if !limited {
			return chunk, nil
		}
		streamID := chunk.Ctrl.StreamID
		if chunk.IsBeginOfStream() {
			s.counts[streamID] = 0
		}
		if text == "" {
			if chunk.IsEndOfStream() {
				delete(s.counts, streamID)
			}
			return chunk, nil
		}
		remaining := s.limit - s.counts[streamID]
		if remaining > 0 {
			runeCount := utf8.RuneCountInString(text)
			if runeCount <= remaining {
				s.counts[streamID] += runeCount
				if chunk.IsEndOfStream() {
					delete(s.counts, streamID)
				}
				return chunk, nil
			}
			cloned := chunk.Clone()
			cloned.Part = genx.Text(string([]rune(text)[:remaining]))
			s.counts[streamID] = s.limit
			if cloned.IsEndOfStream() {
				delete(s.counts, streamID)
			}
			return cloned, nil
		}
		if chunk.IsEndOfStream() {
			delete(s.counts, streamID)
			cloned := chunk.Clone()
			cloned.Part = genx.Text("")
			return cloned, nil
		}
		if chunk.IsBeginOfStream() || outputLimitControlChunk(chunk) {
			cloned := chunk.Clone()
			cloned.Part = genx.Text("")
			return cloned, nil
		}
	}
}

func (s *outputLimitStream) Close() error {
	return s.source.Close()
}

func (s *outputLimitStream) CloseWithError(err error) error {
	return s.source.CloseWithError(err)
}

func limitedAssistantText(chunk *genx.MessageChunk) (string, bool) {
	if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.Label != "assistant" {
		return "", false
	}
	text, ok := chunk.Part.(genx.Text)
	return string(text), ok
}

func outputLimitControlChunk(chunk *genx.MessageChunk) bool {
	ctrl := chunk.Ctrl
	return ctrl.Error != "" || ctrl.ErrorCode != "" || ctrl.ErrorRetryable
}
