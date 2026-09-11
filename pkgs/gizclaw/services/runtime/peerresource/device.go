package peerresource

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// ErrDeviceServiceNotConfigured reports a device read whose backing service is
// missing from the Server composition.
var ErrDeviceServiceNotConfigured = errors.New("peerresource: device service not configured")

type deviceInfoService interface {
	GetSelfInfo(context.Context, giznet.PublicKey) (apitypes.DeviceInfo, error)
	GetSelfRuntime(context.Context, giznet.PublicKey) apitypes.Runtime
}

type deviceStatusService interface {
	GetStatus(context.Context, giznet.PublicKey) (apitypes.PeerStatus, error)
}

// ErrDeviceFirmwareNotBound reports a firmware read for a caller whose Peer has
// no Firmware configuration bound, or whose bound configuration is gone.
var ErrDeviceFirmwareNotBound = errors.New("peerresource: device has no firmware configuration bound")

type ownerProfileResolver interface {
	ResolveOwnerProfile(context.Context, string) (apitypes.RuntimeProfile, error)
}

// ErrDeviceRuntimeProfileNotBound reports a RuntimeProfile read for a caller
// whose owner has no RuntimeProfile binding.
var ErrDeviceRuntimeProfileNotBound = errors.New("peerresource: device has no runtime profile bound")

// DeviceReads exposes the caller's own device projection to Public HTTP
// adapters. Every read, and the Workspace deletion, is pinned to Caller;
// adapters cannot select another Peer.
type DeviceReads struct {
	Caller     giznet.PublicKey
	Info       deviceInfoService
	Status     deviceStatusService
	Peers      peerFirmwareBindingService
	Firmwares  firmwarePeerService
	Profiles   ownerProfileResolver
	Telemetry  *peertelemetry.AdminService
	Workspaces deviceWorkspaceService
}

// DeviceInfo returns the caller's authoritative device identity.
func (r DeviceReads) DeviceInfo(ctx context.Context) (apitypes.DeviceInfo, error) {
	if r.Info == nil {
		return apitypes.DeviceInfo{}, ErrDeviceServiceNotConfigured
	}
	return r.Info.GetSelfInfo(ctx, r.Caller)
}

// DeviceRuntime returns the caller's online runtime projection without
// touching the device or its last-seen state.
func (r DeviceReads) DeviceRuntime(ctx context.Context) (apitypes.Runtime, error) {
	if r.Info == nil {
		return apitypes.Runtime{}, ErrDeviceServiceNotConfigured
	}
	return r.Info.GetSelfRuntime(ctx, r.Caller), nil
}

// DeviceStatus returns the latest stored PeerStatus snapshot of the caller.
func (r DeviceReads) DeviceStatus(ctx context.Context) (apitypes.PeerStatus, error) {
	if r.Status == nil {
		return apitypes.PeerStatus{}, ErrDeviceServiceNotConfigured
	}
	return r.Status.GetStatus(ctx, r.Caller)
}

// DeviceFirmware returns every channel of the Firmware configuration bound to
// the caller. The projection reads Server configuration only, so it answers
// while the device is offline. Channel selection stays with the caller.
func (r DeviceReads) DeviceFirmware(ctx context.Context) (apitypes.Firmware, error) {
	if r.Peers == nil || r.Firmwares == nil {
		return apitypes.Firmware{}, ErrDeviceServiceNotConfigured
	}
	item, err := r.Peers.LoadPeer(ctx, r.Caller)
	if err != nil {
		if errors.Is(err, peer.ErrPeerNotFound) {
			return apitypes.Firmware{}, ErrDeviceFirmwareNotBound
		}
		return apitypes.Firmware{}, err
	}
	if item.FirmwareId == nil || customid.ValidateResourceID(*item.FirmwareId) != nil {
		return apitypes.Firmware{}, ErrDeviceFirmwareNotBound
	}
	response, err := r.Firmwares.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Id: *item.FirmwareId})
	if err != nil {
		return apitypes.Firmware{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetFirmware200JSONResponse:
		return apitypes.Firmware(response), nil
	case adminhttp.GetFirmware404JSONResponse:
		return apitypes.Firmware{}, ErrDeviceFirmwareNotBound
	default:
		return apitypes.Firmware{}, errors.New("peerresource: firmware lookup failed")
	}
}

// DeviceRuntimeProfile returns the workflow catalog of the RuntimeProfile
// currently bound to the caller: the profile name and revision, and the
// workflow names of every collection, all sorted by name. Resource bindings
// and every other profile section stay on the Server.
func (r DeviceReads) DeviceRuntimeProfile(ctx context.Context) (peerhttp.DeviceRuntimeProfile, error) {
	if r.Profiles == nil {
		return peerhttp.DeviceRuntimeProfile{}, ErrDeviceServiceNotConfigured
	}
	profile, err := r.Profiles.ResolveOwnerProfile(ctx, r.Caller.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return peerhttp.DeviceRuntimeProfile{}, ErrDeviceRuntimeProfileNotBound
		}
		return peerhttp.DeviceRuntimeProfile{}, err
	}
	collections := profile.Spec.Workflows.Collections
	names := make([]string, 0, len(collections))
	for name := range collections {
		names = append(names, name)
	}
	slices.Sort(names)
	result := peerhttp.DeviceRuntimeProfile{
		Name: profile.Id, Revision: profile.Revision,
		Collections: make([]peerhttp.DeviceRuntimeProfileCollection, 0, len(names)),
	}
	for _, name := range names {
		aliases := sortedBindingAliases(collections[name])
		workflows := make([]peerhttp.DeviceRuntimeProfileWorkflow, len(aliases))
		for i, alias := range aliases {
			workflows[i] = peerhttp.DeviceRuntimeProfileWorkflow{Name: alias}
		}
		result.Collections = append(result.Collections, peerhttp.DeviceRuntimeProfileCollection{Name: name, Workflows: workflows})
	}
	return result, nil
}

// DeviceTelemetryLatest returns the latest sampled telemetry values of the caller.
func (r DeviceReads) DeviceTelemetryLatest(ctx context.Context, fields []apitypes.PeerTelemetryField) (apitypes.PeerTelemetryLatestResponse, error) {
	if r.Telemetry == nil {
		return apitypes.PeerTelemetryLatestResponse{}, ErrDeviceServiceNotConfigured
	}
	return r.Telemetry.Latest(ctx, r.Caller, fields)
}

// DeviceTelemetryRange returns sampled telemetry points of the caller.
func (r DeviceReads) DeviceTelemetryRange(ctx context.Context, field apitypes.PeerTelemetryField, start, end time.Time, step time.Duration, limit int, order apitypes.PeerTelemetryOrder) (apitypes.PeerTelemetryRangeResponse, error) {
	if r.Telemetry == nil {
		return apitypes.PeerTelemetryRangeResponse{}, ErrDeviceServiceNotConfigured
	}
	return r.Telemetry.QueryRange(ctx, r.Caller, field, start, end, step, limit, order)
}

// DeviceTelemetryAggregate returns bucketed aggregate telemetry of the caller.
func (r DeviceReads) DeviceTelemetryAggregate(ctx context.Context, field apitypes.PeerTelemetryField, start, end time.Time, bucket time.Duration, aggregate apitypes.PeerTelemetryAggregate) (apitypes.PeerTelemetryAggregateResponse, error) {
	if r.Telemetry == nil {
		return apitypes.PeerTelemetryAggregateResponse{}, ErrDeviceServiceNotConfigured
	}
	return r.Telemetry.Aggregate(ctx, r.Caller, field, start, end, bucket, aggregate)
}
