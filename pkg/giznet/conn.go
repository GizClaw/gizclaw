package giznet

import (
	"net"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/core"
)

type Conn struct {
	pk       PublicKey
	peer     *core.Peer
	smux     *core.ServiceMux
	listener *Listener
	closed   atomic.Bool
}

func (c *Conn) Dial(service uint64) (net.Conn, error) {
	smux, err := c.dialServiceMux()
	if err != nil {
		return nil, err
	}
	return smux.OpenStream(service)
}

func (c *Conn) ListenService(service uint64) *ServiceListener {
	return &ServiceListener{
		conn:    c,
		service: service,
	}
}

func (c *Conn) CloseService(service uint64) error {
	smux, err := c.serviceMux()
	if err != nil {
		return err
	}
	return smux.CloseService(service)
}

func (c *Conn) Read(buf []byte) (byte, int, error) {
	if err := c.validate(); err != nil {
		return 0, 0, err
	}
	smux, err := c.serviceMux()
	if err != nil {
		return 0, 0, err
	}
	return smux.Read(buf)
}

func (c *Conn) Write(protocol byte, payload []byte) (int, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	smux, err := c.serviceMux()
	if err != nil {
		return 0, err
	}
	return smux.Write(protocol, payload)
}

// Close marks this handle as closed, releases the peer from the listener's
// established set, and tears down the local service mux. The underlying UDP peer
// and Noise session are retained so future service traffic can establish a new
// Conn.
func (c *Conn) Close() error {
	if c == nil || c.peer == nil || c.listener == nil {
		return ErrNilConn
	}
	return c.listener.releaseConn(c, func() error {
		if !c.closed.CompareAndSwap(false, true) {
			return ErrConnClosed
		}
		c.peer.CloseServiceMux(c.smux)
		return nil
	})
}

func (c *Conn) PublicKey() PublicKey {
	if c == nil {
		return PublicKey{}
	}
	return c.pk
}

func (c *Conn) PeerInfo() *PeerInfo {
	if c == nil || c.peer == nil {
		return nil
	}
	return c.peer.PeerInfo()
}

func (c *Conn) validate() error {
	if c == nil || c.peer == nil {
		return ErrNilConn
	}
	if c.closed.Load() {
		return ErrConnClosed
	}
	return nil
}

func (c *Conn) serviceMux() (*core.ServiceMux, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if c.peer.IsClosed() {
		return nil, ErrUDPClosed
	}
	if c.smux != nil && !c.smux.IsClosed() {
		return c.smux, nil
	}
	if c.smux != nil && c.smux.IsClosed() && c.peer.State() != core.PeerStateEstablished {
		return nil, core.ErrServiceMuxClosed
	}
	smux, err := c.peer.ServiceMux()
	if err != nil {
		return nil, err
	}
	c.smux = smux
	return c.smux, nil
}

func (c *Conn) dialServiceMux() (*core.ServiceMux, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if c.peer.IsClosed() {
		return nil, ErrUDPClosed
	}
	if c.smux != nil && !c.smux.IsClosed() {
		return c.smux, nil
	}
	if c.smux != nil && c.smux.IsClosed() && c.peer.State() != core.PeerStateEstablished {
		if err := c.peer.Connect(); err != nil {
			return nil, err
		}
	}
	smux, err := c.peer.ServiceMux()
	if err != nil {
		return nil, err
	}
	c.smux = smux
	return c.smux, nil
}
