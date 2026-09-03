package peerresource

import (
	"context"
	"errors"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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

// DeviceReads exposes the caller's own device projection to Public HTTP
// adapters. Every read is pinned to Caller; adapters cannot select another Peer.
type DeviceReads struct {
	Caller    giznet.PublicKey
	Info      deviceInfoService
	Status    deviceStatusService
	Telemetry *peertelemetry.AdminService
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
