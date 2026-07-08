package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyPeerConfig(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Peers == nil {
		return apitypes.ApplyResult{}, missingService("peers")
	}
	item, err := resource.AsPeerConfigResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_PEER_CONFIG_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	publicKey := string(pathParam(item.Metadata.Name))
	existing, err := m.getPeerConfig(ctx, publicKey)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	same, err := semanticEqual(existing, item.Spec)
	if err != nil {
		return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
	}
	if same {
		return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindPeerConfig, item.Metadata.Name), nil
	}
	if err := m.putPeerConfig(ctx, publicKey, item.Spec); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindPeerConfig, item.Metadata.Name), nil
}

func (m *Manager) getPeerConfig(ctx context.Context, publicKey string) (apitypes.Configuration, error) {
	response, err := m.services.Peers.GetPeerConfig(ctx, adminhttp.GetPeerConfigRequestObject{PublicKey: publicKey})
	if err != nil {
		return apitypes.Configuration{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetPeerConfig200JSONResponse:
		return apitypes.Configuration(response), nil
	case adminhttp.GetPeerConfig404JSONResponse:
		return apitypes.Configuration{}, responseError(404, "PEER_CONFIG_NOT_FOUND", "peer config not found", response)
	default:
		return apitypes.Configuration{}, unexpectedResponse("GetPeerConfig", response)
	}
}

func (m *Manager) putPeerConfig(ctx context.Context, publicKey string, body apitypes.Configuration) error {
	response, err := m.services.Peers.PutPeerConfig(ctx, adminhttp.PutPeerConfigRequestObject{PublicKey: publicKey, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutPeerConfig200JSONResponse:
		return nil
	case adminhttp.PutPeerConfig400JSONResponse:
		return responseError(400, "PUT_PEER_CONFIG_FAILED", "failed to put peer config", response)
	case adminhttp.PutPeerConfig404JSONResponse:
		return responseError(404, "PUT_PEER_CONFIG_FAILED", "failed to put peer config", response)
	default:
		return unexpectedResponse("PutPeerConfig", response)
	}
}

func resourceFromPeerConfig(name string, item apitypes.Configuration) (apitypes.Resource, error) {
	return marshalResource(apitypes.PeerConfigResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.PeerConfigResourceKind(apitypes.ResourceKindPeerConfig),
		Metadata:   apitypes.ResourceMetadata{Name: name},
		Spec:       item,
	})
}
