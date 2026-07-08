package peer

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	ErrPeerNotFound      = errors.New("peer: peer not found")
	ErrPeerAlreadyExists = errors.New("peer: peer already exists")
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type PeerManager interface {
	PeerRuntime(context.Context, giznet.PublicKey) apitypes.Runtime
	RefreshPeer(context.Context, giznet.PublicKey) (adminhttp.RefreshResult, bool, error)
}

type Server struct {
	Store           kv.Store
	BuildCommit     string
	Endpoint        string
	ServerPublicKey giznet.PublicKey
	SignalingPath   string
	ICETCP          bool
	PeerManager     PeerManager

	mu sync.Mutex
}

type PeerAdminService interface {
	ListPeers(context.Context, adminhttp.ListPeersRequestObject) (adminhttp.ListPeersResponseObject, error)
	FindPubKeyByIMEI(context.Context, adminhttp.FindPubKeyByIMEIRequestObject) (adminhttp.FindPubKeyByIMEIResponseObject, error)
	FindPubKeyBySN(context.Context, adminhttp.FindPubKeyBySNRequestObject) (adminhttp.FindPubKeyBySNResponseObject, error)
	DeletePeer(context.Context, adminhttp.DeletePeerRequestObject) (adminhttp.DeletePeerResponseObject, error)
	GetPeer(context.Context, adminhttp.GetPeerRequestObject) (adminhttp.GetPeerResponseObject, error)
	GetPeerConfig(context.Context, adminhttp.GetPeerConfigRequestObject) (adminhttp.GetPeerConfigResponseObject, error)
	PutPeerConfig(context.Context, adminhttp.PutPeerConfigRequestObject) (adminhttp.PutPeerConfigResponseObject, error)
	GetPeerInfo(context.Context, adminhttp.GetPeerInfoRequestObject) (adminhttp.GetPeerInfoResponseObject, error)
	PutPeerInfo(context.Context, adminhttp.PutPeerInfoRequestObject) (adminhttp.PutPeerInfoResponseObject, error)
	GetPeerRuntime(context.Context, adminhttp.GetPeerRuntimeRequestObject) (adminhttp.GetPeerRuntimeResponseObject, error)
	ApprovePeer(context.Context, adminhttp.ApprovePeerRequestObject) (adminhttp.ApprovePeerResponseObject, error)
	BlockPeer(context.Context, adminhttp.BlockPeerRequestObject) (adminhttp.BlockPeerResponseObject, error)
	RefreshPeer(context.Context, adminhttp.RefreshPeerRequestObject) (adminhttp.RefreshPeerResponseObject, error)
}

type PeerHTTPService interface {
	GetServerInfo(context.Context, peerhttp.GetServerInfoRequestObject) (peerhttp.GetServerInfoResponseObject, error)
}

var _ PeerAdminService = (*Server)(nil)
var _ PeerHTTPService = (*Server)(nil)

// ListPeers implements `adminhttp.StrictServerInterface.ListPeers`.
func (s *Server) ListPeers(ctx context.Context, request adminhttp.ListPeersRequestObject) (adminhttp.ListPeersResponseObject, error) {
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := s.listPage(ctx, cursor, limit)
	if err != nil {
		return adminhttp.ListPeers500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListPeers200JSONResponse(toAdminRegistrationList(items, hasNext, nextCursor)), nil
}

// FindPubKeyByIMEI implements `adminhttp.StrictServerInterface.FindPubKeyByIMEI`.
func (s *Server) FindPubKeyByIMEI(ctx context.Context, request adminhttp.FindPubKeyByIMEIRequestObject) (adminhttp.FindPubKeyByIMEIResponseObject, error) {
	tac, err := pathUnescape(request.Tac)
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	serial, err := pathUnescape(request.Serial)
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	publicKey, err := s.resolveByIMEI(ctx, tac, serial)
	if err != nil {
		return adminhttp.FindPubKeyByIMEI404JSONResponse(apitypes.NewErrorResponse("PEER_IMEI_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.FindPubKeyByIMEI200JSONResponse(adminhttp.PublicKeyResponse{PublicKey: publicKey.String()}), nil
}

// FindPubKeyBySN implements `adminhttp.StrictServerInterface.FindPubKeyBySN`.
func (s *Server) FindPubKeyBySN(ctx context.Context, request adminhttp.FindPubKeyBySNRequestObject) (adminhttp.FindPubKeyBySNResponseObject, error) {
	sn, err := pathUnescape(request.Sn)
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	publicKey, err := s.resolveBySN(ctx, sn)
	if err != nil {
		return adminhttp.FindPubKeyBySN404JSONResponse(apitypes.NewErrorResponse("PEER_SN_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.FindPubKeyBySN200JSONResponse(adminhttp.PublicKeyResponse{PublicKey: publicKey.String()}), nil
}

// DeletePeer implements `adminhttp.StrictServerInterface.DeletePeer`.
func (s *Server) DeletePeer(ctx context.Context, request adminhttp.DeletePeerRequestObject) (adminhttp.DeletePeerResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.delete(ctx, publicKey)
	if err != nil {
		return adminhttp.DeletePeer404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.DeletePeer200JSONResponse(toAdminRegistration(peer)), nil
}

// GetPeer implements `adminhttp.StrictServerInterface.GetPeer`.
func (s *Server) GetPeer(ctx context.Context, request adminhttp.GetPeerRequestObject) (adminhttp.GetPeerResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return adminhttp.GetPeer404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.GetPeer200JSONResponse(toAdminRegistration(peer)), nil
}

// GetPeerConfig implements `adminhttp.StrictServerInterface.GetPeerConfig`.
func (s *Server) GetPeerConfig(ctx context.Context, request adminhttp.GetPeerConfigRequestObject) (adminhttp.GetPeerConfigResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return adminhttp.GetPeerConfig404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.GetPeerConfig200JSONResponse(peer.Configuration), nil
}

// PutPeerConfig implements `adminhttp.StrictServerInterface.PutPeerConfig`.
func (s *Server) PutPeerConfig(ctx context.Context, request adminhttp.PutPeerConfigRequestObject) (adminhttp.PutPeerConfigResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutPeerConfig400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", "request body required")), nil
	}
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return adminhttp.PutPeerConfig400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", err.Error())), nil
	}
	peer, err := s.putConfig(ctx, publicKey, *request.Body)
	if err != nil {
		if errors.Is(err, ErrPeerNotFound) {
			return adminhttp.PutPeerConfig404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.PutPeerConfig400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", err.Error())), nil
	}
	return adminhttp.PutPeerConfig200JSONResponse(peer.Configuration), nil
}

// GetPeerInfo implements `adminhttp.StrictServerInterface.GetPeerInfo`.
func (s *Server) GetPeerInfo(ctx context.Context, request adminhttp.GetPeerInfoRequestObject) (adminhttp.GetPeerInfoResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return adminhttp.GetPeerInfo404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.GetPeerInfo200JSONResponse(peer.Device), nil
}

// PutPeerInfo implements `adminhttp.StrictServerInterface.PutPeerInfo`.
func (s *Server) PutPeerInfo(ctx context.Context, request adminhttp.PutPeerInfoRequestObject) (adminhttp.PutPeerInfoResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutPeerInfo400JSONResponse(apitypes.NewErrorResponse("INVALID_DEVICE_INFO", "request body required")), nil
	}
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	info, err := toAdminDeviceInfo(*request.Body)
	if err != nil {
		return adminhttp.PutPeerInfo400JSONResponse(apitypes.NewErrorResponse("INVALID_DEVICE_INFO", err.Error())), nil
	}
	peer, err := s.putInfo(ctx, publicKey, info)
	if err != nil {
		return adminhttp.PutPeerInfo404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	out, err := toAdminDeviceInfo(peer.Device)
	if err != nil {
		return adminhttp.PutPeerInfo400JSONResponse(apitypes.NewErrorResponse("INVALID_DEVICE_INFO", err.Error())), nil
	}
	return adminhttp.PutPeerInfo200JSONResponse(out), nil
}

// GetPeerRuntime implements `adminhttp.StrictServerInterface.GetPeerRuntime`.
func (s *Server) GetPeerRuntime(ctx context.Context, request adminhttp.GetPeerRuntimeRequestObject) (adminhttp.GetPeerRuntimeResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return adminhttp.GetPeerRuntime200JSONResponse(toAdminRuntime(s.peerRuntime(ctx, publicKey))), nil
}

// ApprovePeer implements `adminhttp.StrictServerInterface.ApprovePeer`.
func (s *Server) ApprovePeer(ctx context.Context, request adminhttp.ApprovePeerRequestObject) (adminhttp.ApprovePeerResponseObject, error) {
	if request.Body == nil {
		return adminhttp.ApprovePeer400JSONResponse(apitypes.NewErrorResponse("INVALID_ROLE", "request body required")), nil
	}
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return adminhttp.ApprovePeer400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", err.Error())), nil
	}
	peer, err := s.approve(ctx, publicKey, apitypes.PeerRole(request.Body.Role))
	if err != nil {
		return adminhttp.ApprovePeer400JSONResponse(apitypes.NewErrorResponse("INVALID_ROLE", err.Error())), nil
	}
	return adminhttp.ApprovePeer200JSONResponse(toAdminRegistration(peer)), nil
}

// BlockPeer implements `adminhttp.StrictServerInterface.BlockPeer`.
func (s *Server) BlockPeer(ctx context.Context, request adminhttp.BlockPeerRequestObject) (adminhttp.BlockPeerResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.block(ctx, publicKey)
	if err != nil {
		return adminhttp.BlockPeer404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.BlockPeer200JSONResponse(toAdminRegistration(peer)), nil
}

// RefreshPeer implements `adminhttp.StrictServerInterface.RefreshPeer`.
func (s *Server) RefreshPeer(ctx context.Context, request adminhttp.RefreshPeerRequestObject) (adminhttp.RefreshPeerResponseObject, error) {
	if s.PeerManager == nil {
		return adminhttp.RefreshPeer502JSONResponse(apitypes.NewErrorResponse("DEVICE_REFRESH_FAILED", "refresh provider not configured")), nil
	}
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	result, online, err := s.PeerManager.RefreshPeer(ctx, publicKey)
	if err != nil {
		switch {
		case errors.Is(err, ErrPeerNotFound):
			return adminhttp.RefreshPeer404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
		case !online:
			return adminhttp.RefreshPeer409JSONResponse(apitypes.NewErrorResponse("DEVICE_OFFLINE", err.Error())), nil
		default:
			return adminhttp.RefreshPeer502JSONResponse(apitypes.NewErrorResponse("DEVICE_REFRESH_FAILED", err.Error())), nil
		}
	}
	return adminhttp.RefreshPeer200JSONResponse(result), nil
}

func (s *Server) GetSelfInfo(ctx context.Context, publicKey giznet.PublicKey) (apitypes.DeviceInfo, error) {
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	info, err := toPeerDeviceInfo(peer.Device)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return info, nil
}

func (s *Server) PutSelfInfo(ctx context.Context, publicKey giznet.PublicKey, body apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	info, err := toAdminDeviceInfo(body)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	peer, err := s.putInfo(ctx, publicKey, info)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	out, err := toPeerDeviceInfo(peer.Device)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return out, nil
}

func (s *Server) GetSelfRuntime(ctx context.Context, publicKey giznet.PublicKey) apitypes.Runtime {
	return s.peerRuntime(ctx, publicKey)
}

// GetServerInfo implements `peerhttp.StrictServerInterface.GetServerInfo`.
func (s *Server) GetServerInfo(_ context.Context, _ peerhttp.GetServerInfoRequestObject) (peerhttp.GetServerInfoResponseObject, error) {
	signalingPath := s.SignalingPath
	if signalingPath == "" {
		signalingPath = "/webrtc/v1/offer"
	}
	return peerhttp.GetServerInfo200JSONResponse(apitypes.ServerInfo{
		BuildCommit: s.BuildCommit,
		Endpoint:    s.Endpoint,
		Ice: struct {
			Tcp bool `json:"tcp"`
			Udp bool `json:"udp"`
		}{
			Tcp: s.ICETCP,
			Udp: true,
		},
		Protocol:      "gizclaw-webrtc",
		PublicKey:     s.ServerPublicKey.String(),
		ServerTime:    time.Now().UnixMilli(),
		SignalingPath: signalingPath,
	}), nil
}

func pathUnescape(value string) (string, error) {
	return url.PathUnescape(value)
}

func parsePublicKeyParam(value string) (giznet.PublicKey, error) {
	text, err := pathUnescape(value)
	if err != nil {
		return giznet.PublicKey{}, err
	}
	return publicKeyFromText(text)
}

func normalizeListParams(cursor *string, limit *int32) (string, int) {
	nextCursor := ""
	if cursor != nil {
		nextCursor = string(*cursor)
	}
	nextLimit := defaultListLimit
	if limit != nil {
		nextLimit = int(*limit)
	}
	if nextLimit <= 0 {
		nextLimit = defaultListLimit
	}
	if nextLimit > maxListLimit {
		nextLimit = maxListLimit
	}
	return nextCursor, nextLimit
}
