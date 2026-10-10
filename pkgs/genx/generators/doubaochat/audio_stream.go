package doubaochat

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// audioStream has one reader and two producers. Neither producer waits for
// the other before starting I/O or publishing its own output.
type audioStream struct {
	ctx        context.Context
	cancel     context.CancelCauseFunc
	chunks     chan *genx.MessageChunk
	done       chan struct{}
	finished   bool
	endErr     error
	replyUsage genx.Usage
	asrUsage   genx.Usage
}

func newAudioStream(ctx context.Context, next genx.Generator, pattern string, reply, transcription genx.ModelContext) *audioStream {
	ctx, cancel := context.WithCancelCause(ctx)
	stream := &audioStream{ctx: ctx, cancel: cancel, chunks: make(chan *genx.MessageChunk, 8), done: make(chan struct{})}
	var workers sync.WaitGroup
	workers.Go(func() { stream.forwardReply(next, pattern, reply) })
	workers.Go(func() {
		text, usage, err := transcribeAudio(ctx, next, pattern, transcription)
		stream.asrUsage = usage
		if err != nil {
			cancel(err)
			return
		}
		stream.publish(genx.NewInputTranscriptChunk(text))
	})
	go func() {
		workers.Wait()
		close(stream.chunks)
		close(stream.done)
	}()
	return stream
}

func (s *audioStream) forwardReply(next genx.Generator, pattern string, request genx.ModelContext) {
	reply, err := next.GenerateStream(s.ctx, pattern, request)
	if err != nil {
		s.cancel(err)
		return
	}
	defer reply.Close()
	for {
		chunk, err := reply.Next()
		if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
			if state, ok := errors.AsType[*genx.State](err); ok {
				s.replyUsage = state.Usage()
			}
			return
		}
		if err != nil {
			s.cancel(err)
			return
		}
		if chunk == nil {
			continue
		}
		if err := genx.StreamError(chunk.Ctrl); err != nil {
			s.cancel(err)
			return
		}
		if _, transcript := genx.InputTranscript(chunk); transcript {
			continue
		}
		if !s.publish(chunk) {
			return
		}
	}
}

func (s *audioStream) publish(chunk *genx.MessageChunk) bool {
	select {
	case s.chunks <- chunk:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *audioStream) Next() (*genx.MessageChunk, error) {
	if s.finished {
		return nil, s.endErr
	}
	if s.ctx.Err() == nil {
		select {
		case chunk, ok := <-s.chunks:
			if ok {
				return chunk, nil
			}
		case <-s.ctx.Done():
		}
	}
	s.finished = true
	s.endErr = context.Cause(s.ctx)
	if s.endErr == nil {
		s.endErr = genx.Done(genx.Usage{
			PromptTokenCount:        s.replyUsage.PromptTokenCount + s.asrUsage.PromptTokenCount,
			CachedContentTokenCount: s.replyUsage.CachedContentTokenCount + s.asrUsage.CachedContentTokenCount,
			GeneratedTokenCount:     s.replyUsage.GeneratedTokenCount + s.asrUsage.GeneratedTokenCount,
		})
	}
	s.cancel(nil)
	return nil, s.endErr
}

func (s *audioStream) Close() error {
	s.cancel(nil)
	<-s.done
	return nil
}

func (s *audioStream) CloseWithError(err error) error {
	s.cancel(err)
	<-s.done
	return nil
}

var _ genx.Stream = (*audioStream)(nil)
