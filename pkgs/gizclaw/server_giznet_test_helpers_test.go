package gizclaw

import (
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type testGiznetConn struct {
	publicKey giznet.PublicKey
	peerInfo  *giznet.PeerInfo
}

func (c *testGiznetConn) Dial(uint64) (net.Conn, error) { return nil, nil }
func (c *testGiznetConn) ListenService(uint64) giznet.ServiceListener {
	return nil
}
func (c *testGiznetConn) CloseService(uint64) error       { return nil }
func (c *testGiznetConn) Read([]byte) (byte, int, error)  { return 0, 0, nil }
func (c *testGiznetConn) Write(byte, []byte) (int, error) { return 0, nil }
func (c *testGiznetConn) PublicKey() giznet.PublicKey     { return c.publicKey }
func (c *testGiznetConn) PeerInfo() *giznet.PeerInfo      { return c.peerInfo }
func (c *testGiznetConn) Close() error                    { return nil }

type testGiznetListener struct {
	closed chan struct{}
}

func newTestGiznetListener() *testGiznetListener {
	return &testGiznetListener{closed: make(chan struct{})}
}

func (l *testGiznetListener) Accept() (giznet.Conn, error) {
	<-l.closed
	return nil, giznet.ErrClosed
}

func (l *testGiznetListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}
