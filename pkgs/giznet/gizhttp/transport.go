package gizhttp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type transport struct {
	conn    giznet.Conn
	service uint64
}

func NewRoundTripper(conn giznet.Conn, service uint64) *transport {
	return &transport{conn: conn, service: service}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil || t.conn == nil {
		return nil, fmt.Errorf("gizhttp: nil conn")
	}
	stream, err := dialService(req.Context(), t.conn, t.service)
	if err != nil {
		return nil, fmt.Errorf("gizhttp: dial service %d: %w", t.service, err)
	}

	stopCancel := watchRequestContext(req, stream)
	writeReq, bodyCtrl := cloneRequestForWrite(req)
	go func() {
		_ = writeReq.Write(stream)
	}()

	resp, err := http.ReadResponse(bufio.NewReader(stream), req)
	if err != nil {
		stopCancel()
		bodyCtrl.abort()
		_ = stream.Close()
		return nil, fmt.Errorf("gizhttp: read response: %w", err)
	}
	resp.Body = &readCloser{
		ReadCloser: resp.Body,
		closeFn: func() error {
			stopCancel()
			bodyCtrl.abort()
			return stream.Close()
		},
	}
	return resp, nil
}

func dialService(ctx context.Context, conn giznet.Conn, service uint64) (net.Conn, error) {
	if dialer, ok := conn.(giznet.ContextDialer); ok {
		return dialer.DialContext(ctx, service)
	}
	return conn.Dial(service)
}

type requestBodyController struct {
	body io.Closer
	once sync.Once
}

func cloneRequestForWrite(req *http.Request) (*http.Request, *requestBodyController) {
	clone := req.Clone(req.Context())
	ctrl := &requestBodyController{}
	if req.Body != nil {
		clone.Body = req.Body
		ctrl.body = req.Body
	}
	return clone, ctrl
}

func (c *requestBodyController) abort() {
	c.once.Do(func() {
		if c.body != nil {
			_ = c.body.Close()
		}
	})
}

func watchRequestContext(req *http.Request, stream net.Conn) func() {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-req.Context().Done():
			_ = stream.SetDeadline(time.Now())
			_ = stream.Close()
		case <-done:
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
		})
	}
}

type readCloser struct {
	io.ReadCloser
	closeFn func() error
}

func (r *readCloser) Close() error {
	err1 := r.ReadCloser.Close()
	err2 := r.closeFn()
	if err1 != nil {
		return err1
	}
	return err2
}
