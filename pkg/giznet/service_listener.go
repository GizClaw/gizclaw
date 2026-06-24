package giznet

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/core"
)

type ServiceListener struct {
	conn    *Conn
	service uint64
	closed  atomic.Bool
	once    sync.Once
}

type serviceListenerAddr struct {
	peerPK  string
	service uint64
}

func (l *ServiceListener) Accept() (net.Conn, error) {
	if l == nil || l.conn == nil {
		return nil, ErrNilConn
	}
	if l.closed.Load() {
		return nil, net.ErrClosed
	}
	if l.conn.peer != nil && l.conn.peer.State() == core.PeerStateOffline {
		return nil, net.ErrClosed
	}
	smux, err := l.conn.serviceMux()
	if err != nil {
		if errors.Is(err, ErrConnClosed) || errors.Is(err, ErrClosed) || errors.Is(err, ErrPeerNotFound) || errors.Is(err, core.ErrServiceMuxClosed) {
			return nil, net.ErrClosed
		}
		return nil, err
	}
	stream, err := smux.AcceptStream(l.service)
	if err != nil {
		if l.closed.Load() || errors.Is(err, ErrAcceptQueueClosed) || errors.Is(err, core.ErrServiceMuxClosed) {
			return nil, net.ErrClosed
		}
		return nil, err
	}
	return stream, nil
}

func (l *ServiceListener) Close() error {
	if l == nil || l.conn == nil {
		return ErrNilConn
	}
	l.once.Do(func() {
		l.closed.Store(true)
		smux, err := l.conn.serviceMux()
		if err != nil {
			return
		}
		_ = smux.StopAcceptingService(l.service)
	})
	return nil
}

func (l *ServiceListener) Addr() net.Addr {
	peerPK := ""
	if l != nil && l.conn != nil {
		peerPK = l.conn.PublicKey().String()
	}
	return serviceListenerAddr{peerPK: peerPK, service: l.Service()}
}

func (l *ServiceListener) Service() uint64 {
	if l == nil {
		return 0
	}
	return l.service
}

func (a serviceListenerAddr) Network() string {
	return "kcp-service"
}

func (a serviceListenerAddr) String() string {
	return fmt.Sprintf("%s/service/%d", a.peerPK, a.service)
}
