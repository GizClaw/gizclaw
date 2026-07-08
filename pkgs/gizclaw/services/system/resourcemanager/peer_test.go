package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestApplyPeerConfigUpdatesResource(t *testing.T) {
	peers := newFakePeers()
	peers.configs["peer-key"] = apitypes.Configuration{}
	manager := New(Services{Peers: peers})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "PeerConfig",
		"metadata": {"name": "peer-key"},
		"spec": {
			"view": "under-12"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("action = %q, want updated", result.Action)
	}
	if peers.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", peers.putCount)
	}
	if peers.configs["peer-key"].View == nil || *peers.configs["peer-key"].View != "under-12" {
		t.Fatalf("stored view = %+v, want under-12", peers.configs["peer-key"].View)
	}
}

func TestGetPeerConfigReturnsResource(t *testing.T) {
	view := "under-12"
	peers := newFakePeers()
	peers.configs["peer-key"] = apitypes.Configuration{
		View: &view,
	}
	manager := New(Services{Peers: peers})

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindPeerConfig, "peer-key")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	config, err := resource.AsPeerConfigResource()
	if err != nil {
		t.Fatalf("AsPeerConfigResource returned error: %v", err)
	}
	if config.Metadata.Name != "peer-key" {
		t.Fatalf("metadata.name = %q, want peer-key", config.Metadata.Name)
	}
	if config.Spec.View == nil || *config.Spec.View != "under-12" {
		t.Fatalf("view = %#v, want under-12", config.Spec.View)
	}
}

func TestPutPeerConfigWritesResource(t *testing.T) {
	peers := newFakePeers()
	peers.configs["peer-key"] = apitypes.Configuration{}
	manager := New(Services{Peers: peers})

	_, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "PeerConfig",
		"metadata": {"name": "peer-key"},
		"spec": {
			"view": "under-18"
		}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if peers.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", peers.putCount)
	}
}

func TestApplyPeerConfigUnchangedSkipsPut(t *testing.T) {
	view := "under-12"
	peers := newFakePeers()
	peers.configs["peer-key"] = apitypes.Configuration{
		View: &view,
	}
	manager := New(Services{Peers: peers})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "PeerConfig",
		"metadata": {"name": "peer-key"},
		"spec": {
			"view": "under-12"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("action = %q, want unchanged", result.Action)
	}
	if peers.putCount != 0 {
		t.Fatalf("putCount = %d, want 0", peers.putCount)
	}
}

func TestPeerServiceErrorResponses(t *testing.T) {
	peers := newFakePeers()
	manager := New(Services{Peers: peers})

	_, err := manager.getPeerConfig(context.Background(), "missing")
	assertResourceError(t, err, 404, "GEAR_NOT_FOUND")

	peers.configs["peer-key"] = apitypes.Configuration{}
	peers.putStatus = 400
	err = manager.putPeerConfig(context.Background(), "peer-key", apitypes.Configuration{})
	assertResourceError(t, err, 400, "INVALID_PARAMS")

	peers.putStatus = 404
	err = manager.putPeerConfig(context.Background(), "peer-key", apitypes.Configuration{})
	assertResourceError(t, err, 404, "GEAR_NOT_FOUND")
}

type fakePeers struct {
	configs   map[string]apitypes.Configuration
	putCount  int
	putStatus int
}

func newFakePeers() *fakePeers {
	return &fakePeers{configs: map[string]apitypes.Configuration{}}
}

func (f *fakePeers) ListPeers(context.Context, adminhttp.ListPeersRequestObject) (adminhttp.ListPeersResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) FindPubKeyByIMEI(context.Context, adminhttp.FindPubKeyByIMEIRequestObject) (adminhttp.FindPubKeyByIMEIResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) FindPubKeyBySN(context.Context, adminhttp.FindPubKeyBySNRequestObject) (adminhttp.FindPubKeyBySNResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) DeletePeer(context.Context, adminhttp.DeletePeerRequestObject) (adminhttp.DeletePeerResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) GetPeer(context.Context, adminhttp.GetPeerRequestObject) (adminhttp.GetPeerResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) GetPeerConfig(_ context.Context, request adminhttp.GetPeerConfigRequestObject) (adminhttp.GetPeerConfigResponseObject, error) {
	config, ok := f.configs[string(request.PublicKey)]
	if !ok {
		return adminhttp.GetPeerConfig404JSONResponse(apitypes.NewErrorResponse("GEAR_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetPeerConfig200JSONResponse(config), nil
}

func (f *fakePeers) PutPeerConfig(_ context.Context, request adminhttp.PutPeerConfigRequestObject) (adminhttp.PutPeerConfigResponseObject, error) {
	switch f.putStatus {
	case 400:
		return adminhttp.PutPeerConfig400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", "invalid")), nil
	case 404:
		return adminhttp.PutPeerConfig404JSONResponse(apitypes.NewErrorResponse("GEAR_NOT_FOUND", "not found")), nil
	}
	f.putCount++
	f.configs[string(request.PublicKey)] = *request.Body
	return adminhttp.PutPeerConfig200JSONResponse(*request.Body), nil
}

func (f *fakePeers) GetPeerInfo(context.Context, adminhttp.GetPeerInfoRequestObject) (adminhttp.GetPeerInfoResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) PutPeerInfo(context.Context, adminhttp.PutPeerInfoRequestObject) (adminhttp.PutPeerInfoResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) GetPeerRuntime(context.Context, adminhttp.GetPeerRuntimeRequestObject) (adminhttp.GetPeerRuntimeResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) ApprovePeer(context.Context, adminhttp.ApprovePeerRequestObject) (adminhttp.ApprovePeerResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) BlockPeer(context.Context, adminhttp.BlockPeerRequestObject) (adminhttp.BlockPeerResponseObject, error) {
	return nil, nil
}

func (f *fakePeers) RefreshPeer(context.Context, adminhttp.RefreshPeerRequestObject) (adminhttp.RefreshPeerResponseObject, error) {
	return nil, nil
}
