package gizhttp

import (
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// InterruptibleRequestBody enables full duplex HTTP responses and returns a
// request-owned body whose Close interrupts a blocked server Read. HTTP/1
// connection reuse is disabled; closing before EOF expires the read deadline.
// Call Close before writing an early response. Writers without ResponseController
// support rely on the supplied body's own Close to interrupt Read.
func InterruptibleRequestBody(w http.ResponseWriter, body io.ReadCloser) io.ReadCloser {
	if body == nil {
		return nil
	}
	controller := http.NewResponseController(w)
	_ = controller.EnableFullDuplex()
	// Set headers before any reader goroutine or proxy can write the response.
	w.Header().Set("Connection", "close")
	return &interruptibleRequestBody{body: body, interrupt: func() {
		_ = controller.SetReadDeadline(time.Now())
	}}
}

type interruptibleRequestBody struct {
	body      io.ReadCloser
	interrupt func()
	eof       atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

func (b *interruptibleRequestBody) Read(data []byte) (int, error) {
	n, err := b.body.Read(data)
	if err == io.EOF {
		b.eof.Store(true)
	}
	return n, err
}

func (b *interruptibleRequestBody) Close() error {
	b.closeOnce.Do(func() {
		if !b.eof.Load() {
			b.interrupt()
		}
		b.closeErr = b.body.Close()
	})
	return b.closeErr
}
