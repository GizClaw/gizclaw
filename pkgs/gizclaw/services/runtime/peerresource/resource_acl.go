package peerresource

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

func (s *Server) grantResourceOwner(ctx context.Context, resource apitypes.ACLResource) error {
	if s.ResourceACL == nil {
		return errors.New("resource ACL service not configured")
	}
	if err := s.ensureResourceOwnerRole(ctx); err != nil {
		return err
	}
	_, err := s.ResourceACL.PutPolicyBinding(
		ctx,
		resourceOwnerBindingID(resource),
		0,
		apitypes.ACLPolicy{
			Subject:  acl.PublicKeySubject(s.Caller.String()),
			Resource: resource,
			Role:     resourceOwnerRole,
		},
	)
	return err
}

func (s *Server) deleteResourceOwnerBinding(ctx context.Context, resource apitypes.ACLResource) error {
	if s.ResourceACL == nil {
		return nil
	}
	_, err := s.ResourceACL.DeletePolicyBinding(ctx, resourceOwnerBindingID(resource))
	if errors.Is(err, acl.ErrPolicyBindingNotFound) {
		return nil
	}
	return err
}

func resourceOwnerBindingID(resource apitypes.ACLResource) string {
	return toolkit.ResourceOwnerPolicyBindingID(resource.Kind, resource.Id)
}
