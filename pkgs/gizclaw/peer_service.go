package gizclaw

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
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
	APIKeys                *apikey.Server
	WebRTCSignalingHandler func() http.Handler
	PeerAvailability       func(context.Context, giznet.PublicKey) error

	// DeviceReads builds the owner-scoped device projection for one API key
	// owner; Contacts and DeviceControl serve the /gizclaw/v1 device and
	// contact extension with the same owner binding.
	DeviceReads   func(giznet.PublicKey) peerresource.DeviceReads
	Contacts      *contact.Server
	DeviceControl *deviceController
}

// PeerService serves one peer connection.
type PeerService struct {
	admin              *adminService
	public             *peerHTTP
	manager            *Manager
	apiKeys            *apikey.Server
	openAIOnce         sync.Once
	openAIProtocol     http.Handler
	openAIProtocolErr  error
	openAIResponseOnce sync.Once
	openAIResponses    *openaiapi.ResponseRuntime
}

var _ peerhttp.StrictServerInterface = (*peerHTTP)(nil)

func (s *PeerService) ServeConn(conn giznet.Conn) error {
	return s.serveConn(conn, nil)
}

func (s *PeerService) serveConn(conn giznet.Conn, isRetiring func() bool) error {
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
	oldConn, err := s.activateConn(context.Background(), conn)
	if err != nil {
		return err
	}
	publicKey := conn.PublicKey()
	defer s.manager.SetPeerDown(publicKey, conn)
	if oldConn != nil {
		_ = oldConn.Close()
	}
	return s.serveActiveConn(conn, isRetiring)
}

func (s *PeerService) serveActiveConn(conn giznet.Conn, isRetiring func() bool) error {
	errCh := make(chan error, 3)
	go func() { errCh <- s.serveAdminWithRetiring(conn, isRetiring) }()
	go func() { errCh <- s.servePublicWithRetiring(conn, isRetiring) }()
	go func() { errCh <- s.serveEdgePublicWithRetiring(conn, isRetiring) }()

	var errs []error
	for i := range 3 {
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

func (s *PeerService) activateConn(ctx context.Context, conn giznet.Conn) (giznet.Conn, error) {
	if s == nil || s.manager == nil {
		return nil, errors.New("gizclaw: nil manager")
	}
	return s.manager.activatePeer(ctx, conn)
}

func isPeerServiceClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, giznet.ErrClosed) ||
		errors.Is(err, giznet.ErrConnClosed) ||
		errors.Is(err, giznet.ErrServiceMuxClosed)
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
