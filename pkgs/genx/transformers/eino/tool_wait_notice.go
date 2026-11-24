package eino

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

func (notice ToolWaitNotice) valid() bool {
	alias := strings.TrimSpace(notice.ToolAlias)
	if alias == "" || alias != notice.ToolAlias || utf8.RuneCountInString(alias) > 256 ||
		notice.Delay < 100*time.Millisecond || notice.Delay > 10*time.Second ||
		len(notice.Messages) < 1 || len(notice.Messages) > 8 {
		return false
	}
	for _, message := range notice.Messages {
		last, _ := utf8.DecodeLastRuneInString(message)
		if message != strings.TrimSpace(message) || utf8.RuneCountInString(message) > 80 ||
			!strings.ContainsRune("。！？!?", last) || strings.ContainsAny(message, "\r\n") {
			return false
		}
	}
	return true
}

func scheduleToolWaitNotice(ctx context.Context, delay time.Duration, publish func() error) func() (bool, error) {
	var finished atomic.Bool
	done := make(chan struct{})
	var sent bool
	var err error
	timer := time.AfterFunc(delay, func() {
		defer close(done)
		if finished.Load() || ctx.Err() != nil {
			return
		}
		sent = true
		err = publish()
	})
	return func() (bool, error) {
		finished.Store(true)
		if timer.Stop() {
			return false, nil
		}
		<-done
		return sent, err
	}
}

func (chatModel *streamingChatModel) publishWaitNotice(state *runState, textField string, notice *ToolWaitNotice) error {
	text := notice.Messages[rand.IntN(len(notice.Messages))] + " "
	for _, output := range chatModel.published {
		if output.Field == textField && output.MIMEType == "text/plain" {
			if err := state.emit(output, text); err != nil {
				return err
			}
		}
	}
	return nil
}
