package gizclaw

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

type aclAuthorizer interface {
	Authorize(context.Context, acl.AuthorizeRequest) error
}

type aclPolicyBindingLister interface {
	ListPolicyBindings(context.Context, acl.ListPolicyBindingsRequest) ([]apitypes.ACLPolicyBinding, bool, *string, error)
}

type peerConfigGetter interface {
	GetPeerConfig(context.Context, adminservice.GetPeerConfigRequestObject) (adminservice.GetPeerConfigResponseObject, error)
}

type peerAuthorizer struct {
	ACL       aclAuthorizer
	Peers     peerConfigGetter
	PublicKey giznet.PublicKey
}

func (a peerAuthorizer) Authorize(ctx context.Context, request acl.AuthorizeRequest) error {
	if a.ACL == nil {
		return errors.New("acl service not configured")
	}
	err := a.authorizeWithCollectionFallback(ctx, request)
	if err == nil || !errors.Is(err, acl.ErrDenied) || !a.shouldTryView(request) {
		return err
	}
	view, ok := a.peerView(ctx)
	if !ok {
		return err
	}
	request.Subject = acl.ViewSubject(view)
	viewErr := a.authorizeWithCollectionFallback(ctx, request)
	if viewErr == nil {
		return nil
	}
	if errors.Is(viewErr, acl.ErrDenied) {
		return err
	}
	return viewErr
}

func (a peerAuthorizer) ListPolicyBindings(ctx context.Context, request acl.ListPolicyBindingsRequest) ([]apitypes.ACLPolicyBinding, bool, *string, error) {
	lister, ok := a.ACL.(aclPolicyBindingLister)
	if !ok {
		return nil, false, nil, errors.New("acl policy binding listing not configured")
	}
	return lister.ListPolicyBindings(ctx, request)
}

func (a peerAuthorizer) authorizeWithCollectionFallback(ctx context.Context, request acl.AuthorizeRequest) error {
	err := a.ACL.Authorize(ctx, request)
	if err == nil || !errors.Is(err, acl.ErrDenied) || !isCollectionFallbackResource(request.Resource) {
		return err
	}
	request.Resource.Id = acl.CollectionResourceID
	return a.ACL.Authorize(ctx, request)
}

func (a peerAuthorizer) shouldTryView(request acl.AuthorizeRequest) bool {
	return request.Subject.Kind == acl.SubjectKindPublicKey && request.Subject.Id == a.PublicKey.String()
}

func (a peerAuthorizer) peerView(ctx context.Context) (string, bool) {
	if a.Peers == nil {
		return "", false
	}
	resp, err := a.Peers.GetPeerConfig(ctx, adminservice.GetPeerConfigRequestObject{PublicKey: a.PublicKey.String()})
	if err != nil {
		return "", false
	}
	config, ok := resp.(adminservice.GetPeerConfig200JSONResponse)
	if !ok || config.View == nil {
		return "", false
	}
	view := strings.TrimSpace(*config.View)
	return view, view != ""
}

func isCollectionFallbackResource(resource apitypes.ACLResource) bool {
	switch resource.Kind {
	case apitypes.ACLResourceKindWorkflow, apitypes.ACLResourceKindWorkspace:
		return resource.Id != "" && resource.Id != acl.CollectionResourceID
	default:
		return false
	}
}
