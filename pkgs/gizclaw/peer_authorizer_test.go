package gizclaw

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestPeerAuthorizerFallsBackToConfiguredView(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	view := "play-openai"
	auth := peerAuthorizer{
		ACL:       fakePeerACL{allowedSubject: acl.ViewSubject(view)},
		Peers:     fakePeerConfigGetter{view: &view},
		PublicKey: key.Public,
	}
	err = auth.Authorize(context.Background(), acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(key.Public.String()),
		Resource:   acl.VoiceResource("openai-alloy"),
		Permission: apitypes.ACLPermissionRead,
	})
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
}

func TestPeerAuthorizerKeepsPKDenialWithoutView(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	auth := peerAuthorizer{
		ACL:       fakePeerACL{allowedSubject: acl.ViewSubject("play-openai")},
		Peers:     fakePeerConfigGetter{},
		PublicKey: key.Public,
	}
	err = auth.Authorize(context.Background(), acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(key.Public.String()),
		Resource:   acl.VoiceResource("openai-alloy"),
		Permission: apitypes.ACLPermissionRead,
	})
	if !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("Authorize() error = %v, want %v", err, acl.ErrDenied)
	}
}

func TestPeerAuthorizerDoesNotFallbackToViewCollectionResource(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	view := "e2e-client"
	auth := peerAuthorizer{
		ACL: fakePeerACL{
			allowedSubject:    acl.ViewSubject(view),
			allowedResource:   acl.CollectionResource(acl.ResourceKindWorkspace),
			allowedPermission: apitypes.ACLPermissionUse,
		},
		Peers:     fakePeerConfigGetter{view: &view},
		PublicKey: key.Public,
	}
	err = auth.Authorize(context.Background(), acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(key.Public.String()),
		Resource:   acl.WorkspaceResource("flowcraft-voice"),
		Permission: apitypes.ACLPermissionUse,
	})
	if !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("Authorize() error = %v, want %v", err, acl.ErrDenied)
	}
}

type fakePeerACL struct {
	allowedSubject    apitypes.ACLSubject
	allowedResource   apitypes.ACLResource
	allowedPermission apitypes.ACLPermission
}

func (a fakePeerACL) Authorize(_ context.Context, request acl.AuthorizeRequest) error {
	if request.Subject != a.allowedSubject {
		return acl.ErrDenied
	}
	if a.allowedResource.Kind != "" && request.Resource != a.allowedResource {
		return acl.ErrDenied
	}
	if a.allowedPermission != "" && request.Permission != a.allowedPermission {
		return acl.ErrDenied
	}
	if request.Subject == a.allowedSubject {
		return nil
	}
	return acl.ErrDenied
}

type fakePeerConfigGetter struct {
	view *string
}

func (g fakePeerConfigGetter) GetPeerConfig(context.Context, adminservice.GetPeerConfigRequestObject) (adminservice.GetPeerConfigResponseObject, error) {
	return adminservice.GetPeerConfig200JSONResponse(apitypes.Configuration{View: g.view}), nil
}
