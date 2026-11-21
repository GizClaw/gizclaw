package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	maxModelDiscards    = 16
	modelDiscardTimeout = 30 * time.Second
)

// Keep request-scoped identity, usage and tracing values. The workflow
// generation owns cancellation because a completed turn can release its context
// while the provider is still sending its final usage frame.
func (transformer *Transformer) scheduleModelDiscard(turn context.Context, request func(context.Context) error) {
	if turn.Err() != nil {
		return
	}
	select {
	case transformer.discardSlots <- struct{}{}:
	default:
		slog.WarnContext(turn, "eino: discarded model request at generation concurrency limit")
		return
	}
	started := transformer.runAsync(func(owner context.Context) {
		defer func() { <-transformer.discardSlots }()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(turn), modelDiscardTimeout)
		defer cancel()
		stop := context.AfterFunc(owner, cancel)
		defer stop()
		if owner.Err() != nil {
			return
		}
		if err := request(ctx); err != nil && owner.Err() == nil {
			// Provider errors may include a response body. Use a content-free
			// diagnostic here and leave request timing to the provider adapter.
			slog.WarnContext(ctx, "eino: background model response ended with an error", "timeout", errors.Is(ctx.Err(), context.DeadlineExceeded))
		}
	})
	if !started {
		<-transformer.discardSlots
	}
}

func drainModelResponse(ctx context.Context, component model.BaseChatModel, messages []*schema.Message, options ...model.Option) error {
	stream, err := component.Stream(ctx, messages, options...)
	if err != nil {
		return err
	}
	if stream == nil {
		return fmt.Errorf("model returned a nil stream")
	}
	defer stream.Close()
	for ctx.Err() == nil {
		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}
