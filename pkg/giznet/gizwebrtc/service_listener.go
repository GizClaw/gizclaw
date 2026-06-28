package gizwebrtc

import (
	"net"
	"sync"
)

type ServiceListener struct {
	conn    *Conn
	service uint64
	ch      chan net.Conn
	closeCh chan struct{}
	once    sync.Once
}

func newServiceListener(conn *Conn, service uint64) *ServiceListener {
	return &ServiceListener{
		conn:    conn,
		service: service,
		ch:      make(chan net.Conn, serviceQueueSize),
		closeCh: make(chan struct{}),
	}
}

func (l *ServiceListener) Accept() (net.Conn, error) {
	if l == nil {
		return nil, ErrNilConn
	}
	select {
	case c, ok := <-l.ch:
		if !ok {
			return nil, ErrServiceClosed
		}
		return c, nil
	case <-l.closeCh:
		return nil, ErrServiceClosed
	case <-l.conn.closeCh:
		return nil, ErrConnClosed
	}
}

func (l *ServiceListener) enqueue(c net.Conn) error {
	select {
	case l.ch <- c:
		return nil
	case <-l.closeCh:
		_ = c.Close()
		return ErrServiceClosed
	case <-l.conn.closeCh:
		_ = c.Close()
		return ErrConnClosed
	}
}

func (l *ServiceListener) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		close(l.closeCh)
	})
	return nil
}

func (l *ServiceListener) Addr() net.Addr {
	if l == nil || l.conn == nil {
		return nil
	}
	return l.conn.localAddr
}
