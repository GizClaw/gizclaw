package gizedge

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const publicTCPMuxClassifyTimeout = 2 * time.Second

type publicTCPMux struct {
	parent net.Listener
	http   *publicTCPMuxListener
	ice    *publicTCPMuxListener
	done   chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

func newPublicTCPMux(parent net.Listener) *publicTCPMux {
	m := &publicTCPMux{
		parent: parent,
		done:   make(chan struct{}),
	}
	m.http = newPublicTCPMuxListener(parent.Addr())
	m.ice = newPublicTCPMuxListener(parent.Addr())
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.acceptLoop()
	}()
	return m
}

func (m *publicTCPMux) HTTPListener() net.Listener { return m.http }

func (m *publicTCPMux) ICETCPListener() net.Listener { return m.ice }

func (m *publicTCPMux) Close() error {
	if m == nil {
		return nil
	}
	var err error
	m.once.Do(func() {
		close(m.done)
		err = errors.Join(err, m.parent.Close())
		err = errors.Join(err, m.http.Close())
		err = errors.Join(err, m.ice.Close())
		m.wg.Wait()
	})
	return err
}

func (m *publicTCPMux) acceptLoop() {
	for {
		conn, err := m.parent.Accept()
		if err != nil {
			_ = m.http.Close()
			_ = m.ice.Close()
			return
		}
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.routeConn(conn)
		}()
	}
}

func (m *publicTCPMux) routeConn(conn net.Conn) {
	route, prefix, err := classifyPublicTCPConn(conn, time.Now().Add(publicTCPMuxClassifyTimeout))
	if err != nil {
		_ = conn.Close()
		return
	}
	wrapped := &prefixConn{Conn: conn, prefix: bytes.NewReader(prefix)}
	var delivered bool
	switch route {
	case publicTCPRouteHTTP:
		delivered = m.http.deliver(wrapped)
	case publicTCPRouteICE:
		delivered = m.ice.deliver(wrapped)
	}
	if !delivered {
		_ = wrapped.Close()
	}
}

type publicTCPRoute int

const (
	publicTCPRouteICE publicTCPRoute = iota
	publicTCPRouteHTTP
)

var publicTCPHTTPMethods = []string{
	"GET ",
	"POST ",
	"PUT ",
	"PATCH ",
	"DELETE ",
	"HEAD ",
	"OPTIONS ",
	"TRACE ",
	"CONNECT ",
}

const http2CleartextPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

func classifyPublicTCPConn(conn net.Conn, deadline time.Time) (publicTCPRoute, []byte, error) {
	var prefix []byte
	buf := make([]byte, 1)
	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return publicTCPRouteICE, nil, err
		}
		n, err := conn.Read(buf)
		if err != nil {
			return publicTCPRouteICE, nil, err
		}
		if n > 0 {
			prefix = append(prefix, buf[0])
		}

		state := classifyPublicTCPPrefix(prefix)
		switch state {
		case publicTCPPrefixHTTP:
			_ = conn.SetReadDeadline(time.Time{})
			return publicTCPRouteHTTP, prefix, nil
		case publicTCPPrefixICE:
			_ = conn.SetReadDeadline(time.Time{})
			return publicTCPRouteICE, prefix, nil
		case publicTCPPrefixNeedMore:
		}
	}
}

type publicTCPPrefixState int

const (
	publicTCPPrefixNeedMore publicTCPPrefixState = iota
	publicTCPPrefixHTTP
	publicTCPPrefixICE
)

func classifyPublicTCPPrefix(prefix []byte) publicTCPPrefixState {
	if len(prefix) == 0 {
		return publicTCPPrefixNeedMore
	}
	text := string(prefix)
	if strings.HasPrefix(http2CleartextPreface, text) {
		if text == http2CleartextPreface {
			return publicTCPPrefixHTTP
		}
		return publicTCPPrefixNeedMore
	}
	for _, method := range publicTCPHTTPMethods {
		if strings.HasPrefix(method, text) {
			if text == method {
				return publicTCPPrefixHTTP
			}
			return publicTCPPrefixNeedMore
		}
	}
	return publicTCPPrefixICE
}

type publicTCPMuxListener struct {
	addr   net.Addr
	conns  chan net.Conn
	done   chan struct{}
	mu     sync.Mutex
	closed bool
	once   sync.Once
}

func newPublicTCPMuxListener(addr net.Addr) *publicTCPMuxListener {
	return &publicTCPMuxListener{
		addr:  addr,
		conns: make(chan net.Conn, 32),
		done:  make(chan struct{}),
	}
}

func (l *publicTCPMuxListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	default:
	}
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *publicTCPMuxListener) Close() error {
	l.once.Do(func() {
		l.mu.Lock()
		l.closed = true
		close(l.done)
		for {
			select {
			case conn := <-l.conns:
				_ = conn.Close()
			default:
				l.mu.Unlock()
				return
			}
		}
	})
	return nil
}

func (l *publicTCPMuxListener) Addr() net.Addr {
	if l == nil {
		return nil
	}
	return l.addr
}

func (l *publicTCPMuxListener) deliver(conn net.Conn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	select {
	case l.conns <- conn:
		return true
	default:
		return false
	}
}

type prefixConn struct {
	net.Conn
	prefix *bytes.Reader
}

func (c *prefixConn) Read(p []byte) (int, error) {
	if c.prefix != nil && c.prefix.Len() > 0 {
		n, err := c.prefix.Read(p)
		if err != nil && !errors.Is(err, io.EOF) {
			return n, err
		}
		return n, nil
	}
	return c.Conn.Read(p)
}
