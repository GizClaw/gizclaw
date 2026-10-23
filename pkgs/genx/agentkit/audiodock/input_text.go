package audiodock

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// inputText is owned by one input route. Once done closes, text and err are
// immutable. Waiters never hold the router lock or gate raw audio delivery.
type inputText struct {
	done chan struct{}
	text strings.Builder
	err  error
}

func (text *inputText) wait(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", context.Cause(ctx)
	case <-text.done:
		return text.text.String(), text.err
	}
}

// inputTexts retains only the current PTT input. An interrupted route cannot
// satisfy its replacement, and late ASR from the previous input is ignored.
type inputTexts struct {
	mu      sync.Mutex
	id      string
	current *inputText
	closed  bool
}

func (texts *inputTexts) start(id string) *inputText {
	texts.mu.Lock()
	defer texts.mu.Unlock()
	texts.finishLocked(errors.New("interrupted"))
	text := &inputText{done: make(chan struct{})}
	texts.id, texts.current = id, text
	if texts.closed {
		texts.finishLocked(io.ErrUnexpectedEOF)
	}
	return text
}

func (texts *inputTexts) append(chunk *genx.MessageChunk) {
	if chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
		return
	}
	texts.mu.Lock()
	defer texts.mu.Unlock()
	if texts.current == nil || dockStreamID(chunk) != texts.id {
		return
	}
	if chunk.Ctrl == nil || !chunk.Ctrl.TextInterim {
		if text, ok := chunk.Part.(genx.Text); ok {
			texts.current.text.WriteString(string(text))
		}
	}
	if chunk.IsEndOfStream() {
		texts.finishLocked(genx.StreamError(chunk.Ctrl))
	}
}

func (texts *inputTexts) close(err error) {
	texts.mu.Lock()
	defer texts.mu.Unlock()
	texts.closed = true
	texts.finishLocked(err)
}

func (texts *inputTexts) finishLocked(err error) {
	if texts.current != nil {
		texts.current.err = err
		close(texts.current.done)
		texts.current = nil
	}
}
