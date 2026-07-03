package gizhttp

import (
	"errors"
	"fmt"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type listener struct {
	conn    giznet.Conn
	service uint64
	inner   giznet.ServiceListener
}

type listenerAddr struct {
	peerPK  string
	service uint64
}

func NewListener(conn giznet.Conn, service uint64) net.Listener {
	return &listener{
		conn:    conn,
		service: service,
		inner:   conn.ListenService(service),
	}
}

func (l *listener) Accept() (net.Conn, error) {
	if l == nil || l.inner == nil {
		return nil, net.ErrClosed
	}
	stream, err := l.inner.Accept()
	if err != nil {
		return nil, err
	}
	return wrapStream(stream), nil
}

func (l *listener) Close() error {
	if l == nil || l.inner == nil {
		return net.ErrClosed
	}
	return l.inner.Close()
}

func (l *listener) Addr() net.Addr {
	peerPK := ""
	if l != nil && l.conn != nil {
		peerPK = l.conn.PublicKey().String()
	}
	return listenerAddr{peerPK: peerPK, service: l.service}
}

func (a listenerAddr) Network() string {
	return "gizhttp"
}

func (a listenerAddr) String() string {
	return fmt.Sprintf("%s/service/%d", a.peerPK, a.service)
}

func IsClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, giznet.ErrClosed) ||
		errors.Is(err, giznet.ErrConnClosed) ||
		errors.Is(err, giznet.ErrServiceMuxClosed)
}
