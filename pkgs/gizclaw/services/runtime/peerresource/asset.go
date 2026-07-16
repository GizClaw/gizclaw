package peerresource

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
)

// PrepareAssetDownload authorizes a Resource binding before opening asset bytes.
func (s *Server) PrepareAssetDownload(ctx context.Context, request rpcpb.AssetDownloadRequest) (rpcpb.AssetDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	if s == nil || s.Assets == nil {
		return rpcpb.AssetDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "asset service not configured"}, nil
	}
	ref, err := asset.ParseRef(strings.TrimSpace(request.GetRef()))
	if err != nil {
		return rpcpb.AssetDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "invalid asset reference"}, nil
	}
	bindings, err := s.Assets.LiveBindings(ctx, ref)
	if err != nil {
		if errors.Is(err, asset.ErrNotFound) {
			return rpcpb.AssetDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "asset not found"}, nil
		}
		return rpcpb.AssetDownloadResponse{}, nil, nil, err
	}
	authorized := false
	for _, binding := range bindings {
		resource, ok := assetACLResource(binding.Owner)
		if !ok {
			continue
		}
		if s.AssetDisplays == nil {
			continue
		}
		displayAsset, err := s.AssetDisplays.ResourceHasDisplayAsset(ctx, binding.Owner, ref)
		if err != nil {
			return rpcpb.AssetDownloadResponse{}, nil, nil, err
		}
		if !displayAsset {
			continue
		}
		if err := s.authorizeErr(ctx, resource, apitypes.ACLPermissionRead); err == nil {
			authorized = true
			break
		} else if !errors.Is(err, acl.ErrDenied) {
			return rpcpb.AssetDownloadResponse{}, nil, nil, err
		}
	}
	if !authorized {
		return rpcpb.AssetDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: "asset is not available to this peer"}, nil
	}
	stored, reader, err := s.Assets.Open(ctx, ref)
	if err != nil {
		if errors.Is(err, asset.ErrNotFound) {
			return rpcpb.AssetDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "asset not found"}, nil
		}
		return rpcpb.AssetDownloadResponse{}, nil, nil, err
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		_ = reader.Close()
		return rpcpb.AssetDownloadResponse{}, nil, nil, err
	}
	if err := reader.Close(); err != nil {
		return rpcpb.AssetDownloadResponse{}, nil, nil, err
	}
	stored, reader, err = s.Assets.Open(ctx, ref)
	if err != nil {
		if errors.Is(err, asset.ErrNotFound) {
			return rpcpb.AssetDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "asset not found"}, nil
		}
		return rpcpb.AssetDownloadResponse{}, nil, nil, err
	}
	metadata := stored.Metadata
	response := rpcpb.AssetDownloadResponse{Metadata: &rpcpb.AssetMetadata{
		Ref:       metadata.Ref.String(),
		MediaType: metadata.MediaType,
		SizeBytes: metadata.SizeBytes,
		Sha256:    hex.EncodeToString(metadata.SHA256[:]),
		CreatedAt: metadata.CreatedAt.UTC().Format(time.RFC3339Nano),
	}}
	if metadata.ExpiresAt != nil {
		expiresAt := metadata.ExpiresAt.UTC().Format(time.RFC3339Nano)
		response.Metadata.ExpiresAt = &expiresAt
	}
	return response, reader, nil, nil
}

func assetACLResource(owner asset.Owner) (apitypes.ACLResource, bool) {
	if owner.Kind != asset.OwnerKindResource {
		return apitypes.ACLResource{}, false
	}
	kind, id, ok := strings.Cut(owner.ID, "/")
	if !ok || kind == "" || id == "" {
		return apitypes.ACLResource{}, false
	}
	switch apitypes.ResourceKind(kind) {
	case apitypes.ResourceKindCredential:
		return acl.CredentialResource(id), true
	case apitypes.ResourceKindFirmware:
		return acl.FirmwareResource(id), true
	case apitypes.ResourceKindGameRuleset:
		return acl.GameRulesetResource(id), true
	case apitypes.ResourceKindModel:
		return acl.ModelResource(id), true
	case apitypes.ResourceKindTool:
		return acl.ToolResource(id), true
	case apitypes.ResourceKindVoice:
		return acl.VoiceResource(id), true
	case apitypes.ResourceKindWorkflow:
		return apitypes.ACLResource{Kind: acl.ResourceKindWorkflow, Id: id}, true
	case apitypes.ResourceKindWorkspace:
		return acl.WorkspaceResource(id), true
	default:
		return apitypes.ACLResource{}, false
	}
}
