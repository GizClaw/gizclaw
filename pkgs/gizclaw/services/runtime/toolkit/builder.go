package toolkit

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

type Authorizer interface {
	Authorize(context.Context, acl.AuthorizeRequest) error
}

type AvailabilityChecker interface {
	ToolAvailable(context.Context, Tool) (bool, error)
}

type BuildRequest struct {
	Subject        apitypes.ACLSubject
	AllowedToolIDs []string
}

type Builder struct {
	Tools        *Server
	Authorizer   Authorizer
	Availability AvailabilityChecker
}

func (b *Builder) Build(ctx context.Context, req BuildRequest) (ToolKit, error) {
	if b == nil || b.Tools == nil {
		return ToolKit{}, ErrNotConfigured
	}
	tools, err := b.Tools.ListTools(ctx)
	if err != nil {
		return ToolKit{}, err
	}
	allowedPolicy := toolIDSet(req.AllowedToolIDs)
	out := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if !tool.Enabled {
			continue
		}
		if allowedPolicy != nil && !allowedPolicy[tool.ID] {
			continue
		}
		if err := b.authorizeUse(ctx, req.Subject, tool); err != nil {
			if errors.Is(err, acl.ErrDenied) {
				continue
			}
			return ToolKit{}, err
		}
		available, err := b.available(ctx, tool)
		if err != nil {
			return ToolKit{}, err
		}
		if !available {
			continue
		}
		out = append(out, tool)
	}
	return ToolKit{Tools: cloneTools(out)}, nil
}

func (b *Builder) authorizeUse(ctx context.Context, subject apitypes.ACLSubject, tool Tool) error {
	if b.Authorizer == nil {
		return nil
	}
	if subject.Kind == "" {
		return fmt.Errorf("toolkit: subject is required when authorizer is configured")
	}
	return b.Authorizer.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    subject,
		Resource:   ToolResource(tool.ID),
		Permission: apitypes.ACLPermissionUse,
	})
}

func (b *Builder) available(ctx context.Context, tool Tool) (bool, error) {
	if b.Availability == nil {
		return true, nil
	}
	return b.Availability.ToolAvailable(ctx, tool)
}

func toolIDSet(ids []string) map[string]bool {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (tk ToolKit) Find(id string) (Tool, bool) {
	idx := slices.IndexFunc(tk.Tools, func(tool Tool) bool {
		return tool.ID == id
	})
	if idx < 0 {
		return Tool{}, false
	}
	return cloneTool(tk.Tools[idx]), true
}
