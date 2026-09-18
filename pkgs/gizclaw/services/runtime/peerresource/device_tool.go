package peerresource

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

// ErrDeviceToolNotExposed reports a control Tool name that the caller's
// RuntimeProfile does not expose to the control app.
var ErrDeviceToolNotExposed = errors.New("peerresource: tool is not exposed to the control app")

type deviceToolService interface {
	GetToolByID(context.Context, string) (toolkit.Tool, error)
}

// ControlTool is one Tool the device owner's control app may list and invoke.
type ControlTool struct {
	Name    string
	Binding apitypes.RuntimeProfileBinding
	Tool    toolkit.Tool
}

// DeviceControlTools lists the Tools of the caller's RuntimeProfile whose
// binding sets control_access, sorted by name. Only enabled client_rpc Tools
// qualify: an http_request Tool runs on the Server, and a control app must not
// reach a Server-side integration through a device route. A caller with no
// RuntimeProfile bound exposes nothing. The list reads Server configuration
// only, so it never contacts the device.
func (r DeviceReads) DeviceControlTools(ctx context.Context) ([]ControlTool, error) {
	bindings, err := r.controlToolBindings(ctx)
	if err != nil {
		return nil, err
	}
	aliases := sortedBindingAliases(bindings)
	out := make([]ControlTool, 0, len(aliases))
	for _, alias := range aliases {
		tool, ok, err := r.controlTool(ctx, bindings[alias])
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, ControlTool{Name: alias, Binding: bindings[alias], Tool: tool})
		}
	}
	return out, nil
}

// DeviceControlTool resolves one Tool the caller's control app may invoke. A
// Tool that DeviceControlTools would not list answers ErrDeviceToolNotExposed,
// so an unexposed Tool can be neither discovered nor invoked.
func (r DeviceReads) DeviceControlTool(ctx context.Context, name string) (ControlTool, error) {
	bindings, err := r.controlToolBindings(ctx)
	if err != nil {
		return ControlTool{}, err
	}
	name = strings.TrimSpace(name)
	binding, ok := bindings[name]
	if !ok {
		return ControlTool{}, ErrDeviceToolNotExposed
	}
	tool, ok, err := r.controlTool(ctx, binding)
	if err != nil {
		return ControlTool{}, err
	}
	if !ok {
		return ControlTool{}, ErrDeviceToolNotExposed
	}
	return ControlTool{Name: name, Binding: binding, Tool: tool}, nil
}

func (r DeviceReads) controlToolBindings(ctx context.Context) (map[string]apitypes.RuntimeProfileBinding, error) {
	if r.Profiles == nil || r.Tools == nil {
		return nil, ErrDeviceServiceNotConfigured
	}
	profile, err := r.Profiles.ResolveOwnerProfile(ctx, r.Caller.String())
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]apitypes.RuntimeProfileBinding{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]apitypes.RuntimeProfileBinding{}
	for alias, binding := range bindingMap(profile.Spec.Resources.Tools) {
		if binding.ControlAccess != nil && binding.ControlAccess.Valid() {
			out[alias] = binding
		}
	}
	return out, nil
}

func (r DeviceReads) controlTool(ctx context.Context, binding apitypes.RuntimeProfileBinding) (toolkit.Tool, bool, error) {
	tool, err := r.Tools.GetToolByID(ctx, binding.ResourceId)
	if errors.Is(err, toolkit.ErrToolNotFound) {
		return toolkit.Tool{}, false, nil
	}
	if err != nil {
		return toolkit.Tool{}, false, err
	}
	return tool, tool.Enabled && tool.Type == toolkit.ToolTypeClientRPC, nil
}
