package peertelemetry

import (
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
)

func mapAudioPlayer(obs *telemetrypb.AudioPlayerObservation, at time.Time) (StatusPatch, error) {
	if obs == nil {
		return StatusPatch{}, fmt.Errorf("%w: audioplayer observation is nil", ErrInvalidFrame)
	}
	wire := &rpcpb.AudioPlayerStatus{
		State: obs.State, CurrentIndex: obs.CurrentIndex, PositionMs: obs.PositionMs,
		DurationMs: obs.DurationMs, Repeat: obs.Repeat, PlaylistLength: obs.PlaylistLength,
		PlaylistRevision: obs.PlaylistRevision, ErrorCode: obs.ErrorCode, ErrorMessage: obs.ErrorMessage,
		ObservedAtUnixMs: at.UnixMilli(),
	}
	if err := rpcapi.ValidateAudioPlayerStatus(wire); err != nil {
		return StatusPatch{}, fmt.Errorf("%w: %v", ErrInvalidFrame, err)
	}
	status := AudioPlayerStatus(wire)
	return StatusPatch{ReportedAt: at, AudioPlayer: &status}, nil
}

// AudioPlayerStatus projects a validated device response into the stored HTTP
// snapshot. The caller validates the wire status and supplies its report time.
func AudioPlayerStatus(wire *rpcpb.AudioPlayerStatus) apitypes.AudioPlayerStatus {
	status := apitypes.AudioPlayerStatus{
		State: wire.State, PositionMs: int64(wire.PositionMs), Repeat: wire.Repeat,
		PlaylistLength: int(wire.PlaylistLength), PlaylistRevision: int64(wire.PlaylistRevision),
		ErrorCode: wire.ErrorCode, ErrorMessage: wire.ErrorMessage, ObservedAtUnixMs: wire.ObservedAtUnixMs,
	}
	if wire.CurrentIndex != nil {
		index := int(*wire.CurrentIndex)
		status.CurrentIndex = &index
	}
	if wire.DurationMs != nil {
		duration := int64(*wire.DurationMs)
		status.DurationMs = &duration
	}
	return status
}

func validAudioPlayerSnapshot(status apitypes.AudioPlayerStatus) bool {
	if status.PositionMs < 0 || status.PlaylistLength < 0 || status.PlaylistLength > rpcapi.MaxAudioPlayerItems || status.PlaylistRevision < 0 || status.PlaylistRevision > 1<<32-1 || (status.DurationMs != nil && *status.DurationMs < 0) || (status.CurrentIndex != nil && (*status.CurrentIndex < 0 || *status.CurrentIndex >= status.PlaylistLength)) {
		return false
	}
	wire := &rpcpb.AudioPlayerStatus{State: status.State, PositionMs: uint64(status.PositionMs), Repeat: status.Repeat, PlaylistLength: uint32(status.PlaylistLength), PlaylistRevision: uint32(status.PlaylistRevision), ErrorCode: status.ErrorCode, ErrorMessage: status.ErrorMessage, ObservedAtUnixMs: status.ObservedAtUnixMs}
	if status.CurrentIndex != nil {
		wire.CurrentIndex = new(uint32(*status.CurrentIndex))
	}
	if status.DurationMs != nil {
		wire.DurationMs = new(uint64(*status.DurationMs))
	}
	return rpcapi.ValidateAudioPlayerStatus(wire) == nil
}
