package dashscoperealtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/GizClaw/dashscope-realtime-go"
)

// Only initial setup is replayable: no input has been consumed and no output
// stream exists yet. Established sessions never enter this retry loop.
func (t *Transformer) connect(ctx context.Context, config *dashscope.SessionConfig) (dashScopeRealtimeSession, error) {
	wait := t.retryWait
	if wait == nil {
		wait = waitDashScopeConnect
	}
	delay := 100 * time.Millisecond
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		session, err := t.connectOnce(ctx, config)
		if err == nil {
			if err := ctx.Err(); err != nil {
				_ = session.Close()
				return nil, err
			}
			return session, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt == 5 || !dashScopeConnectRecoverable(err) {
			return nil, err
		}
		if err := wait(ctx, delay); err != nil {
			return nil, err
		}
		delay = min(delay*2, 5*time.Second)
	}
}

func (t *Transformer) connectOnce(ctx context.Context, config *dashscope.SessionConfig) (dashScopeRealtimeSession, error) {
	session, err := t.realtime.Connect(ctx, &dashscope.RealtimeConfig{Model: t.model})
	if err != nil {
		if session != nil {
			_ = session.Close()
		}
		return nil, fmt.Errorf("dashscope connect: %w", err)
	}
	stopCancel := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stopCancel()
	ready := false
	defer func() {
		if !ready {
			_ = session.Close()
		}
	}()
	for event, err := range session.Events() {
		if err != nil {
			return nil, fmt.Errorf("dashscope wait session: %w", err)
		}
		if event != nil && event.Type == dashscope.EventTypeError && event.Error != nil {
			return nil, &dashscope.Error{Code: event.Error.Code, Message: event.Error.Message}
		}
		if event != nil && event.Type == dashscope.EventTypeSessionCreated {
			if err := session.UpdateSession(config); err != nil {
				return nil, fmt.Errorf("dashscope update session: %w", err)
			}
			ready = true
			return session, nil
		}
	}
	return nil, fmt.Errorf("dashscope: session.created not received: %w", io.EOF)
}

func waitDashScopeConnect(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func dashScopeConnectRecoverable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if apiErr, ok := errors.AsType[*dashscope.Error](err); ok {
		if apiErr.IsAuth() || apiErr.Code == dashscope.ErrCodeInvalidParameter || apiErr.Code == dashscope.ErrCodeModelNotFound {
			return false
		}
		if apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 && apiErr.HTTPStatus != 429 {
			return false
		}
		return apiErr.HTTPStatus == 503 || apiErr.HTTPStatus == 429 || apiErr.Code == dashscope.ErrCodeServiceBusy
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	// Some websocket transports flatten the underlying network error.
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "broken pipe") || strings.Contains(message, "connection reset by peer")
}
