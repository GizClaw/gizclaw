package peer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func (s *Server) DownloadPeerIcon(ctx context.Context, request adminhttp.DownloadPeerIconRequestObject) (adminhttp.DownloadPeerIconResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	format, err := iconasset.ParseFormat(string(request.Format))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	reader, size, err := s.DownloadSelfIcon(ctx, publicKey, format)
	if err != nil {
		if errors.Is(err, ErrPeerNotFound) || errors.Is(err, io.EOF) {
			return adminhttp.DownloadPeerIcon404JSONResponse(apitypes.NewErrorResponse("ICON_NOT_FOUND", "peer or icon not found")), nil
		}
		return adminhttp.DownloadPeerIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to open peer icon")), nil
	}
	if format == iconasset.FormatPNG {
		return adminhttp.DownloadPeerIcon200ImagepngResponse{Body: reader, ContentLength: size}, nil
	}
	return adminhttp.DownloadPeerIcon200ApplicationoctetStreamResponse{Body: reader, ContentLength: size}, nil
}

func (s *Server) UploadPeerIcon(ctx context.Context, request adminhttp.UploadPeerIconRequestObject) (adminhttp.UploadPeerIconResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return adminhttp.UploadPeerIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	format, err := iconasset.ParseFormat(string(request.Format))
	if err != nil {
		return adminhttp.UploadPeerIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	info, err := s.UploadSelfIcon(ctx, publicKey, format, request.Body)
	if errors.Is(err, iconasset.ErrTooLarge) {
		return adminhttp.UploadPeerIcon413JSONResponse(apitypes.NewErrorResponse("ICON_TOO_LARGE", err.Error())), nil
	}
	if errors.Is(err, iconasset.ErrInvalid) {
		return adminhttp.UploadPeerIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	if errors.Is(err, ErrPeerNotFound) {
		return adminhttp.UploadPeerIcon404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	if err != nil {
		return adminhttp.UploadPeerIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to update peer icon")), nil
	}
	return adminhttp.UploadPeerIcon200JSONResponse(info), nil
}

func (s *Server) DeletePeerIcon(ctx context.Context, request adminhttp.DeletePeerIconRequestObject) (adminhttp.DeletePeerIconResponseObject, error) {
	publicKey, err := parsePublicKeyParam(string(request.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	format, err := iconasset.ParseFormat(string(request.Format))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	info, err := s.DeleteSelfIcon(ctx, publicKey, format)
	if errors.Is(err, ErrPeerNotFound) {
		return adminhttp.DeletePeerIcon404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
	}
	if err != nil {
		return adminhttp.DeletePeerIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to delete peer icon")), nil
	}
	return adminhttp.DeletePeerIcon200JSONResponse(info), nil
}

func (s *Server) DownloadSelfIcon(ctx context.Context, publicKey giznet.PublicKey, format iconasset.Format) (io.ReadCloser, int64, error) {
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return nil, 0, err
	}
	if iconasset.Slot(peer.Device.Icon, format) == nil {
		return nil, 0, io.EOF
	}
	return iconasset.Open(s.Assets, iconasset.ObjectName(publicKey.String(), format))
}

func (s *Server) UploadSelfIcon(ctx context.Context, publicKey giznet.PublicKey, format iconasset.Format, body io.Reader) (apitypes.DeviceInfo, error) {
	data, err := iconasset.ReadValidated(body, format)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	unlock := s.IconLocks.Lock(publicKey.String() + ":" + string(format))
	defer unlock()
	_, err = s.get(ctx, publicKey)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	if s.Assets == nil {
		return apitypes.DeviceInfo{}, errors.New("peer: asset store not configured")
	}
	objectName := iconasset.ObjectName(publicKey.String(), format)
	if err := s.Assets.Put(objectName, bytes.NewReader(data)); err != nil {
		return apitypes.DeviceInfo{}, errors.New("peer: failed to store icon")
	}
	recordUnlock := s.IconLocks.LockRecord(publicKey.String())
	defer recordUnlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.DeviceInfo{}, errors.New("peer: failed to reload icon metadata")
	}
	peer.Device.Icon = iconasset.SetSlot(peer.Device.Icon, format, &objectName)
	saved, err := s.put(ctx, peer)
	if err != nil {
		return apitypes.DeviceInfo{}, errors.New("peer: failed to update icon")
	}
	return saved.Device, nil
}

func (s *Server) DeleteSelfIcon(ctx context.Context, publicKey giznet.PublicKey, format iconasset.Format) (apitypes.DeviceInfo, error) {
	unlock := s.IconLocks.Lock(publicKey.String() + ":" + string(format))
	defer unlock()
	_, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	if s.Assets == nil {
		return apitypes.DeviceInfo{}, errors.New("peer: asset store not configured")
	}
	if err := s.Assets.Delete(iconasset.ObjectName(publicKey.String(), format)); err != nil {
		return apitypes.DeviceInfo{}, errors.New("peer: failed to delete icon")
	}
	recordUnlock := s.IconLocks.LockRecord(publicKey.String())
	defer recordUnlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.DeviceInfo{}, errors.New("peer: failed to reload icon metadata")
	}
	peer.Device.Icon = iconasset.SetSlot(peer.Device.Icon, format, nil)
	saved, err := s.put(ctx, peer)
	if err != nil {
		return apitypes.DeviceInfo{}, errors.New("peer: failed to update icon")
	}
	return saved.Device, nil
}
