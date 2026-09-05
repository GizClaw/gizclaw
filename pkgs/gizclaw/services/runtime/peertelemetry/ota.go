package peertelemetry

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
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

func otaStatus(obs *telemetrypb.OtaObservation, ts time.Time) apitypes.PeerOtaStatus {
	states := map[telemetrypb.OtaState]string{1: "started", 2: "downloading", 3: "succeeded", 4: "failed"}
	return apitypes.PeerOtaStatus{State: states[obs.State], UpdateId: obs.UpdateId, ObservedAt: ts,
		TargetVersion: obs.TargetVersion, DownloadPercent: obs.DownloadPercent, ErrorCode: obs.ErrorCode, ErrorMessage: obs.ErrorMessage}
}
