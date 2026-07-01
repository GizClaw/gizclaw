package peer

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/serverpublic"
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
	RefreshPeer(context.Context, giznet.PublicKey) (adminservice.RefreshResult, bool, error)
}

type Server struct {
	Store           kv.Store
	BuildCommit     string
	Endpoint        string
	ServerPublicKey giznet.PublicKey
	SignalingPath   string
	PeerManager     PeerManager

	mu sync.Mutex
}

type PeerAdminService interface {
	ListPeers(context.Context, adminservice.ListPeersRequestObject) (adminservice.ListPeersResponseObject, error)
	FindPubKeyByIMEI(context.Context, adminservice.FindPubKeyByIMEIRequestObject) (adminservice.FindPubKeyByIMEIResponseObject, error)
	FindPubKeyBySN(context.Context, adminservice.FindPubKeyBySNRequestObject) (adminservice.FindPubKeyBySNResponseObject, error)
	DeletePeer(context.Context, adminservice.DeletePeerRequestObject) (adminservice.DeletePeerResponseObject, error)
	GetPeer(context.Context, adminservice.GetPeerRequestObject) (adminservice.GetPeerResponseObject, error)
	GetPeerConfig(context.Context, adminservice.GetPeerConfigRequestObject) (adminservice.GetPeerConfigResponseObject, error)
	PutPeerConfig(context.Context, adminservice.PutPeerConfigRequestObject) (adminservice.PutPeerConfigResponseObject, error)
	GetPeerInfo(context.Context, adminservice.GetPeerInfoRequestObject) (adminservice.GetPeerInfoResponseObject, error)
	PutPeerInfo(context.Context, adminservice.PutPeerInfoRequestObject) (adminservice.PutPeerInfoResponseObject, error)
	GetPeerRuntime(context.Context, adminservice.GetPeerRuntimeRequestObject) (adminservice.GetPeerRuntimeResponseObject, error)
	ApprovePeer(context.Context, adminservice.ApprovePeerRequestObject) (adminservice.ApprovePeerResponseObject, error)
	BlockPeer(context.Context, adminservice.BlockPeerRequestObject) (adminservice.BlockPeerResponseObject, error)
	RefreshPeer(context.Context, adminservice.RefreshPeerRequestObject) (adminservice.RefreshPeerResponseObject, error)
}

type ServerPublicService interface {
	GetServerInfo(context.Context, serverpublic.GetServerInfoRequestObject) (serverpublic.GetServerInfoResponseObject, error)
}

var _ PeerAdminService = (*Server)(nil)
var _ ServerPublicService = (*Server)(nil)

// ListPeers implements `adminservice.StrictServerInterface.ListPeers`.
func (s *Server) ListPeers(ctx context.Context, request adminservice.ListPeersRequestObject) (adminservice.ListPeersResponseObject, error) {
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := s.listPage(ctx, cursor, limit)
	if err != nil {
		return adminservice.ListPeers500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListPeers200JSONResponse(toAdminRegistrationList(items, hasNext, nextCursor)), nil
}

// FindPubKeyByIMEI implements `adminservice.StrictServerInterface.FindPubKeyByIMEI`.
func (s *Server) FindPubKeyByIMEI(ctx context.Context, request adminservice.FindPubKeyByIMEIRequestObject) (adminservice.FindPubKeyByIMEIResponseObject, error) {
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
		return adminservice.FindPubKeyByIMEI404JSONResponse(apitypes.NewErrorResponse("PEER_IMEI_NOT_FOUND", err.Error())), nil
	}
	return adminservice.FindPubKeyByIMEI200JSONResponse(adminservice.PublicKeyResponse{PublicKey: publicKey.String()}), nil
}

// FindPubKeyBySN implements `adminservice.StrictServerInterface.FindPubKeyBySN`.
func (s *Server) FindPubKeyBySN(ctx context.Context, request adminservice.FindPubKeyBySNRequestObject) (adminservice.FindPubKeyBySNResponseObject, error) {
	sn, err := pathUnescape(request.Sn)
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	publicKey, err := s.resolveBySN(ctx, sn)
	if err != nil {
		return adminservice.FindPubKeyBySN404JSONResponse(apitypes.NewErrorResponse("PEER_SN_NOT_FOUND", err.Error())), nil
	}
	return adminservice.FindPubKeyBySN200JSONResponse(adminservice.PublicKeyResponse{PublicKey: publicKey.String()}), nil
}

// DeletePeer implements `adminservice.StrictServerInterface.DeletePeer`.
func (s *Server) DeletePeer(ctx context.Context, request adminservice.DeletePeerRequestObject) (adminservice.DeletePeerResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.delete(ctx, publicKey)
	if err != nil {
		return adminservice.DeletePeer404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminservice.DeletePeer200JSONResponse(toAdminRegistration(peer)), nil
}

// GetPeer implements `adminservice.StrictServerInterface.GetPeer`.
func (s *Server) GetPeer(ctx context.Context, request adminservice.GetPeerRequestObject) (adminservice.GetPeerResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return adminservice.GetPeer404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminservice.GetPeer200JSONResponse(toAdminRegistration(peer)), nil
}

// GetPeerConfig implements `adminservice.StrictServerInterface.GetPeerConfig`.
func (s *Server) GetPeerConfig(ctx context.Context, request adminservice.GetPeerConfigRequestObject) (adminservice.GetPeerConfigResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return adminservice.GetPeerConfig404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminservice.GetPeerConfig200JSONResponse(peer.Configuration), nil
}

// PutPeerConfig implements `adminservice.StrictServerInterface.PutPeerConfig`.
func (s *Server) PutPeerConfig(ctx context.Context, request adminservice.PutPeerConfigRequestObject) (adminservice.PutPeerConfigResponseObject, error) {
	if request.Body == nil {
		return adminservice.PutPeerConfig400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", "request body required")), nil
	}
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return adminservice.PutPeerConfig400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", err.Error())), nil
	}
	peer, err := s.putConfig(ctx, publicKey, *request.Body)
	if err != nil {
		if errors.Is(err, ErrPeerNotFound) {
			return adminservice.PutPeerConfig404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
		}
		return adminservice.PutPeerConfig400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", err.Error())), nil
	}
	return adminservice.PutPeerConfig200JSONResponse(peer.Configuration), nil
}

// GetPeerInfo implements `adminservice.StrictServerInterface.GetPeerInfo`.
func (s *Server) GetPeerInfo(ctx context.Context, request adminservice.GetPeerInfoRequestObject) (adminservice.GetPeerInfoResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return adminservice.GetPeerInfo404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminservice.GetPeerInfo200JSONResponse(peer.Device), nil
}

// PutPeerInfo implements `adminservice.StrictServerInterface.PutPeerInfo`.
func (s *Server) PutPeerInfo(ctx context.Context, request adminservice.PutPeerInfoRequestObject) (adminservice.PutPeerInfoResponseObject, error) {
	if request.Body == nil {
		return adminservice.PutPeerInfo400JSONResponse(apitypes.NewErrorResponse("INVALID_DEVICE_INFO", "request body required")), nil
	}
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	info, err := toAdminDeviceInfo(*request.Body)
	if err != nil {
		return adminservice.PutPeerInfo400JSONResponse(apitypes.NewErrorResponse("INVALID_DEVICE_INFO", err.Error())), nil
	}
	peer, err := s.putInfo(ctx, publicKey, info)
	if err != nil {
		return adminservice.PutPeerInfo404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	out, err := toAdminDeviceInfo(peer.Device)
	if err != nil {
		return adminservice.PutPeerInfo400JSONResponse(apitypes.NewErrorResponse("INVALID_DEVICE_INFO", err.Error())), nil
	}
	return adminservice.PutPeerInfo200JSONResponse(out), nil
}

// GetPeerRuntime implements `adminservice.StrictServerInterface.GetPeerRuntime`.
func (s *Server) GetPeerRuntime(ctx context.Context, request adminservice.GetPeerRuntimeRequestObject) (adminservice.GetPeerRuntimeResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return adminservice.GetPeerRuntime200JSONResponse(toAdminRuntime(s.peerRuntime(ctx, publicKey))), nil
}

// ApprovePeer implements `adminservice.StrictServerInterface.ApprovePeer`.
func (s *Server) ApprovePeer(ctx context.Context, request adminservice.ApprovePeerRequestObject) (adminservice.ApprovePeerResponseObject, error) {
	if request.Body == nil {
		return adminservice.ApprovePeer400JSONResponse(apitypes.NewErrorResponse("INVALID_ROLE", "request body required")), nil
	}
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return adminservice.ApprovePeer400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", err.Error())), nil
	}
	peer, err := s.approve(ctx, publicKey, apitypes.PeerRole(request.Body.Role))
	if err != nil {
		return adminservice.ApprovePeer400JSONResponse(apitypes.NewErrorResponse("INVALID_ROLE", err.Error())), nil
	}
	return adminservice.ApprovePeer200JSONResponse(toAdminRegistration(peer)), nil
}

// BlockPeer implements `adminservice.StrictServerInterface.BlockPeer`.
func (s *Server) BlockPeer(ctx context.Context, request adminservice.BlockPeerRequestObject) (adminservice.BlockPeerResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	peer, err := s.block(ctx, publicKey)
	if err != nil {
		return adminservice.BlockPeer404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	return adminservice.BlockPeer200JSONResponse(toAdminRegistration(peer)), nil
}

// RefreshPeer implements `adminservice.StrictServerInterface.RefreshPeer`.
func (s *Server) RefreshPeer(ctx context.Context, request adminservice.RefreshPeerRequestObject) (adminservice.RefreshPeerResponseObject, error) {
	if s.PeerManager == nil {
		return adminservice.RefreshPeer502JSONResponse(apitypes.NewErrorResponse("DEVICE_REFRESH_FAILED", "refresh provider not configured")), nil
	}
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	result, online, err := s.PeerManager.RefreshPeer(ctx, publicKey)
	if err != nil {
		switch {
		case errors.Is(err, ErrPeerNotFound):
			return adminservice.RefreshPeer404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
		case !online:
			return adminservice.RefreshPeer409JSONResponse(apitypes.NewErrorResponse("DEVICE_OFFLINE", err.Error())), nil
		default:
			return adminservice.RefreshPeer502JSONResponse(apitypes.NewErrorResponse("DEVICE_REFRESH_FAILED", err.Error())), nil
		}
	}
	return adminservice.RefreshPeer200JSONResponse(result), nil
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

// GetServerInfo implements `serverpublic.StrictServerInterface.GetServerInfo`.
func (s *Server) GetServerInfo(_ context.Context, _ serverpublic.GetServerInfoRequestObject) (serverpublic.GetServerInfoResponseObject, error) {
	signalingPath := s.SignalingPath
	if signalingPath == "" {
		signalingPath = "/webrtc/v1/offer"
	}
	return serverpublic.GetServerInfo200JSONResponse(apitypes.ServerInfo{
		BuildCommit: s.BuildCommit,
		Endpoint:    s.Endpoint,
		Ice: struct {
			Tcp bool `json:"tcp"`
			Udp bool `json:"udp"`
		}{
			Tcp: false,
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
