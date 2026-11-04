//go:build store_e2e

package store_e2e_test

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

// These TCP proxies retain real TiKV responses after the connections have
// served healthy reads. Requests, PD and Raft remain live. The driver therefore
// tests socket/context deadlines rather than replacing KV results with mocks.
type pausedTiKVResponseProxy struct {
	listener net.Listener
	paused   atomic.Bool
	held     atomic.Int64
	resume   chan struct{}
}

func newPausedTiKVResponseProxy(t *testing.T, target string) *pausedTiKVResponseProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &pausedTiKVResponseProxy{listener: listener, resume: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	accepted := make(chan struct{})
	var handlers sync.WaitGroup
	go func() {
		defer close(accepted)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			handlers.Go(func() { p.serve(ctx, target, conn) })
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		<-accepted
		handlers.Wait()
	})
	return p
}

func (p *pausedTiKVResponseProxy) address() string { return p.listener.Addr().String() }

func (p *pausedTiKVResponseProxy) serve(ctx context.Context, target string, source net.Conn) {
	defer source.Close()
	stopSource := context.AfterFunc(ctx, func() { _ = source.Close() })
	defer stopSource()
	upstream, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", target)
	if err != nil {
		return
	}
	stopUpstream := context.AfterFunc(ctx, func() { _ = upstream.Close() })
	defer stopUpstream()
	requestsDone := make(chan struct{})
	go func() {
		defer close(requestsDone)
		_, _ = io.Copy(upstream, source)
		_ = upstream.Close()
	}()
	defer func() { _ = source.Close(); _ = upstream.Close(); <-requestsDone }()
	buffer := make([]byte, 64*1024)
	for {
		n, err := upstream.Read(buffer)
		if n > 0 {
			if p.paused.Load() {
				p.held.Add(1)
				select {
				case <-ctx.Done():
					return
				case <-p.resume:
				}
			}
			if _, err := source.Write(buffer[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

type redirectedTiKVClient struct {
	tikv.Client
	addresses map[string]string
	enabled   atomic.Bool
}

func (c *redirectedTiKVClient) address(address string) string {
	if !c.enabled.Load() {
		return address
	}
	if proxy, ok := c.addresses[address]; ok {
		return proxy
	}
	return address
}

func (c *redirectedTiKVClient) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	return c.Client.SendRequest(ctx, c.address(address), req, timeout)
}

func (c *redirectedTiKVClient) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	c.Client.SendRequestAsync(ctx, c.address(address), req, callback)
}
