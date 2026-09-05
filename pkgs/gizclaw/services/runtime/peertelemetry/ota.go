package peertelemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func validateOTA(obs *telemetrypb.OtaObservation) error {
	if obs == nil {
		return fmt.Errorf("%w: ota observation is nil", ErrInvalidFrame)
	}
	if obs.State < telemetrypb.OtaState_OTA_STATE_STARTED || obs.State > telemetrypb.OtaState_OTA_STATE_FAILED {
		return fmt.Errorf("%w: unsupported ota state %d", ErrInvalidFrame, obs.State)
	}
	if len(obs.UpdateId) == 0 || len(obs.UpdateId) > 128 || !utf8.ValidString(obs.UpdateId) {
		return fmt.Errorf("%w: ota update_id must contain 1..128 UTF-8 bytes", ErrInvalidFrame)
	}
	for _, field := range []struct {
		name, value string
		limit       int
	}{
		{"target_version", obs.GetTargetVersion(), 128},
		{"error_code", obs.GetErrorCode(), 128},
		{"error_message", obs.GetErrorMessage(), 512},
	} {
		if len(field.value) > field.limit || !utf8.ValidString(field.value) {
			return fmt.Errorf("%w: invalid ota %s", ErrInvalidFrame, field.name)
		}
	}
	if obs.State == telemetrypb.OtaState_OTA_STATE_DOWNLOADING && obs.DownloadPercent == nil {
		return fmt.Errorf("%w: ota downloading requires download_percent", ErrInvalidFrame)
	}
	if obs.DownloadPercent != nil {
		if err := validateFiniteRange("ota download_percent", *obs.DownloadPercent, 0, 100); err != nil {
			return err
		}
	}
	if obs.State != telemetrypb.OtaState_OTA_STATE_FAILED && (obs.ErrorCode != nil || obs.ErrorMessage != nil) {
		return fmt.Errorf("%w: ota error fields require failed state", ErrInvalidFrame)
	}
	return nil
}

// Emit only after the entire frame passes validation; OTA events do not mutate
// fixed peer status or create high-cardinality metric series.
func logOTA(ctx context.Context, peer giznet.PublicKey, frame *telemetrypb.TelemetryFrame, baseTime time.Time) {
	for _, observation := range frame.Observations {
		obs := observation.GetOta()
		if obs == nil {
			continue
		}
		attrs := []slog.Attr{
			slog.String("peer_public_key", peer.String()),
			slog.String("ota_state", obs.State.String()),
			slog.String("update_id", obs.UpdateId),
			slog.Int64("observed_at_unix_ms", baseTime.Add(time.Duration(observation.ObservedAtDeltaMs)*time.Millisecond).UnixMilli()),
			slog.Uint64("sequence", uint64(frame.Sequence)),
		}
		if obs.TargetVersion != nil {
			attrs = append(attrs, slog.String("target_version", *obs.TargetVersion))
		}
		if obs.DownloadPercent != nil {
			attrs = append(attrs, slog.Float64("download_percent", *obs.DownloadPercent))
		}
		if obs.ErrorCode != nil {
			attrs = append(attrs, slog.String("error_code", *obs.ErrorCode))
		}
		if obs.ErrorMessage != nil {
			attrs = append(attrs, slog.String("error_message", *obs.ErrorMessage))
		}
		level := slog.LevelInfo
		if obs.State == telemetrypb.OtaState_OTA_STATE_FAILED {
			level = slog.LevelWarn
		}
		slog.LogAttrs(ctx, level, "gizclaw: ota telemetry", attrs...)
	}
}
