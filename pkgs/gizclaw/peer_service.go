package gizclaw

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	// ServicePeerRPC is the reliable peer RPC service stream.
	ServicePeerRPC uint64 = 0x00
	// ServicePeerHTTP is the reliable peer HTTP service stream.
	ServicePeerHTTP uint64 = 0x01
	// ServicePeerOpenAI is the reliable peer OpenAI-compatible HTTP service stream.
	ServicePeerOpenAI uint64 = 0x02
	// ServiceEdgeHTTP is the reliable edge-node HTTP forwarding service stream.
	ServiceEdgeHTTP uint64 = 0x30
	// ServiceEdgeRPC is the reliable edge-node control RPC service stream.
	ServiceEdgeRPC uint64 = 0x31
	// ServiceAdminHTTP is the reliable admin HTTP service stream.
	ServiceAdminHTTP uint64 = 0x10

	// EventStreamAgent is the reliable agent event stream.
	EventStreamAgent uint64 = 0x20
	// EventStreamTelemetry is the unreliable telemetry event packet.
	EventStreamTelemetry byte = 0x40

	// MediaStreamOpus is the WebRTC Opus media stream codec.
	MediaStreamOpus = "audio/opus"
)

type peerHTTP struct {
	peer.PeerHTTPService
	Self   peerHTTPSelfService
	Status peerHTTPStatusService
	publiclogin.PeerHTTP
	WebRTCSignalingHandler func() http.Handler
}

type peerHTTPSelfService interface {
	GetSelfRegistration(context.Context, giznet.PublicKey) (peerhttp.PeerSelf, error)
	GetSelfRuntime(context.Context, giznet.PublicKey) apitypes.Runtime
}

type peerHTTPStatusService interface {
	GetStatus(context.Context, giznet.PublicKey) (apitypes.PeerStatus, error)
	PutStatus(context.Context, giznet.PublicKey, apitypes.PeerStatus) (apitypes.PeerStatus, error)
}

// PeerService serves one peer connection.
type PeerService struct {
	admin    *adminService
	public   *peerHTTP
	manager  *Manager
	sessions *publiclogin.SessionManager
}

var _ peerhttp.StrictServerInterface = (*peerHTTP)(nil)

func (s *PeerService) ServeConn(conn giznet.Conn) error {
	if s == nil {
		return errors.New("gizclaw: nil peer service")
	}
	if conn == nil {
		return errors.New("gizclaw: nil conn")
	}
	defer func() {
		_ = conn.Close()
	}()
	if err := s.validateServices(); err != nil {
		return err
	}
	if err := s.ensureConnectedPeer(context.Background(), conn); err != nil {
		return err
	}
	publicKey := conn.PublicKey()
	oldConn := s.manager.SetPeerUp(publicKey, conn)
	defer s.manager.SetPeerDown(publicKey, conn)
	if oldConn != nil {
		_ = oldConn.Close()
	}

	errCh := make(chan error, 3)
	go func() { errCh <- s.serveAdmin(conn) }()
	go func() { errCh <- s.servePublic(conn) }()
	go func() { errCh <- s.serveEdgePublic(conn) }()

	var errs []error
	for i := 0; i < 3; i++ {
		err := <-errCh
		if i == 0 {
			_ = conn.Close()
		}
		if err != nil && !isPeerServiceClosed(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func isPeerServiceClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, giznet.ErrClosed) ||
		errors.Is(err, giznet.ErrConnClosed) ||
		errors.Is(err, giznet.ErrServiceMuxClosed)
}

func (s *PeerService) ensureConnectedPeer(ctx context.Context, conn giznet.Conn) error {
	if s == nil || s.manager == nil {
		return nil
	}
	_, err := s.manager.EnsurePeer(ctx, conn.PublicKey())
	return err
}

func (s *PeerService) validateServices() error {
	switch {
	case s.admin == nil:
		return errors.New("gizclaw: nil admin service")
	case s.manager == nil:
		return errors.New("gizclaw: nil manager")
	case s.public == nil:
		return errors.New("gizclaw: nil public service")
	default:
		return nil
	}
}
