package peerresource

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

func (s *Server) grantWorkspaceOwner(ctx context.Context, workspaceName string) error {
	if s.ResourceACL == nil {
		return errors.New("workspace ACL service not configured")
	}
	if err := s.ensureResourceOwnerRole(ctx); err != nil {
		return err
	}
	_, err := s.ResourceACL.PutPolicyBinding(
		ctx,
		workspaceOwnerBindingID(workspaceName),
		0,
		apitypes.ACLPolicy{
			Subject:  acl.PublicKeySubject(s.Caller.String()),
			Resource: acl.WorkspaceResource(workspaceName),
			Role:     resourceOwnerRole,
		},
	)
	return err
}

func (s *Server) deleteWorkspaceOwnerBinding(ctx context.Context, workspaceName string) error {
	if s.ResourceACL == nil {
		return nil
	}
	_, err := s.ResourceACL.DeletePolicyBinding(ctx, workspaceOwnerBindingID(workspaceName))
	if errors.Is(err, acl.ErrPolicyBindingNotFound) {
		return nil
	}
	return err
}

func workspaceOwnerBindingID(workspaceName string) string {
	return toolkit.ResourceOwnerPolicyBindingID(acl.ResourceKindWorkspace, workspaceName)
}
