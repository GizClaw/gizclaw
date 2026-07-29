package gizclaw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	flowcraftagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type driveWorkspaceMemory struct {
	resolver interface {
		ResolveMemory(context.Context, string) (agenthost.Spec, error)
	}
	stores       *memorystore.Registry
	serverRoot   string
	genXForOwner func(context.Context, string) (*peergenx.Service, error)
}

func (delivery *driveWorkspaceMemory) Snapshot(ctx context.Context, workspaceName string) (gameplay.DriveFactTarget, error) {
	spec, err := delivery.resolve(ctx, workspaceName)
	if err != nil {
		return gameplay.DriveFactTarget{}, err
	}
	return driveFactTarget(spec)
}

func (delivery *driveWorkspaceMemory) Observe(ctx context.Context, target gameplay.DriveFactTarget, observation memory.Observation) (memory.ObserveResult, error) {
	store, closer, err := delivery.open(ctx, target)
	if err != nil {
		return memory.ObserveResult{}, err
	}
	if !memory.SupportsDirectFactObservation(store) {
		return memory.ObserveResult{}, errors.Join(
			fmt.Errorf("%w: Workspace Memory does not support direct Facts", memory.ErrUnsupported),
			closer.Close(),
		)
	}
	result, observeErr := store.Observe(ctx, observation)
	return result, errors.Join(observeErr, closer.Close())
}

func (delivery *driveWorkspaceMemory) Wait(ctx context.Context, target gameplay.DriveFactTarget, request memory.OperationRequest) (memory.ObserveResult, error) {
	store, closer, err := delivery.open(ctx, target)
	if err != nil {
		return memory.ObserveResult{}, err
	}
	waiter, ok := store.(memory.OperationWaiter)
	if !ok {
		return memory.ObserveResult{}, errors.Join(
			fmt.Errorf("%w: Workspace Memory does not expose operation waiting", memory.ErrUnsupported),
			closer.Close(),
		)
	}
	result, waitErr := waiter.Wait(ctx, request)
	return result, errors.Join(waitErr, closer.Close())
}

func (delivery *driveWorkspaceMemory) open(ctx context.Context, target gameplay.DriveFactTarget) (memory.Store, io.Closer, error) {
	spec, err := delivery.resolve(ctx, target.WorkspaceName)
	if err != nil {
		return nil, nil, err
	}
	current, err := driveFactTarget(spec)
	if err != nil {
		return nil, nil, err
	}
	if !sameDriveFactTarget(current, target) {
		return nil, nil, fmt.Errorf("%w: Workspace Memory binding changed after Drive submission", memory.ErrConflict)
	}
	var modelService *peergenx.Service
	owner := ""
	if spec.Workspace.OwnerPublicKey != nil {
		owner = strings.TrimSpace(*spec.Workspace.OwnerPublicKey)
	}
	if spec.MemoryBinding.Driver == apitypes.RuntimeProfileMemoryDriverFlowcraft &&
		owner != "" && delivery.genXForOwner != nil {
		modelService, err = delivery.genXForOwner(ctx, owner)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: resolve Workspace owner models: %v", memory.ErrUnavailable, err)
		}
	}
	request := memorystore.Request{
		WorkspaceName:   target.WorkspaceName,
		ProfileName:     target.ProfileName,
		ProfileRevision: target.ProfileRevision,
		BindingName:     target.BindingName,
		Layout:          *spec.MemoryLayout,
		Binding:         *spec.MemoryBinding,
		ModelLoader:     flowcraftagent.NewRuntimeMemoryLoader(modelService),
		ServerRoot:      delivery.serverRoot,
	}
	result, err := delivery.stores.Resolve(ctx, request)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: construct Workspace Memory: %v", memory.ErrUnavailable, err)
	}
	return result.Store, result.Closer, nil
}

func sameDriveFactTarget(current, submitted gameplay.DriveFactTarget) bool {
	return current.WorkspaceName == submitted.WorkspaceName &&
		current.ProfileName == submitted.ProfileName &&
		current.BindingName == submitted.BindingName &&
		current.BindingIdentity == submitted.BindingIdentity
}

func (delivery *driveWorkspaceMemory) resolve(ctx context.Context, workspaceName string) (agenthost.Spec, error) {
	if delivery == nil || delivery.resolver == nil || delivery.stores == nil {
		return agenthost.Spec{}, fmt.Errorf("%w: Workspace Memory resolver is not configured", memory.ErrInvalidInput)
	}
	spec, err := delivery.resolver.ResolveMemory(ctx, workspaceName)
	if err != nil {
		return agenthost.Spec{}, fmt.Errorf("%w: resolve Workspace Memory: %v", memory.ErrInvalidInput, err)
	}
	if spec.MemoryBinding == nil || spec.MemoryLayout == nil || strings.TrimSpace(spec.MemoryName) == "" {
		return agenthost.Spec{}, fmt.Errorf("%w: Workspace %q has no Memory binding", memory.ErrUnsupported, workspaceName)
	}
	return spec, nil
}

func driveFactTarget(spec agenthost.Spec) (gameplay.DriveFactTarget, error) {
	workspaceName := strings.TrimSpace(spec.Workspace.Name)
	if workspaceName == "" || spec.MemoryBinding == nil || spec.MemoryLayout == nil {
		return gameplay.DriveFactTarget{}, fmt.Errorf("%w: incomplete Workspace Memory binding", memory.ErrInvalidInput)
	}
	identityPayload := struct {
		Driver     apitypes.RuntimeProfileMemoryDriver     `json:"driver"`
		Connection apitypes.RuntimeProfileMemoryConnection `json:"connection"`
		LayoutID   string                                  `json:"layout_id"`
	}{
		Driver: spec.MemoryBinding.Driver, Connection: spec.MemoryBinding.Connection,
		LayoutID: spec.MemoryBinding.LayoutId,
	}
	data, err := json.Marshal(identityPayload)
	if err != nil {
		return gameplay.DriveFactTarget{}, fmt.Errorf("%w: encode Workspace Memory identity", memory.ErrInvalidInput)
	}
	sum := sha256.Sum256(data)
	return gameplay.DriveFactTarget{
		WorkspaceName: workspaceName, ProfileName: spec.MemoryProfileName,
		ProfileRevision: spec.MemoryProfileRevision, BindingName: spec.MemoryName,
		BindingIdentity: hex.EncodeToString(sum[:]),
	}, nil
}

func (m *Manager) ownerGenX(ctx context.Context, owner string) (*peergenx.Service, error) {
	if m == nil {
		return nil, errors.New("gizclaw: manager is not configured")
	}
	profile, err := m.runtimeProfileForOwner(ctx, owner)
	if err != nil {
		return nil, err
	}
	return m.ownerGenXForProfile(ctx, owner, profile)
}
