package peerrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"database/sql"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// PutOTAStatus atomically retains the latest OTA attempt in the peer runtime
// store. Terminal states cannot regress within an attempt. A later attempt
// replaces the entire snapshot, clearing the previous attempt's diagnostics.
func (s *Server) PutOTAStatus(ctx context.Context, peer giznet.PublicKey, next apitypes.PeerOtaStatus) error {
	if err := validateOTAStatus(next); err != nil {
		return err
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	if peer.IsZero() {
		return ErrInvalidPublicKey
	}

	for range 16 {
		if err := ctx.Err(); err != nil {
			return err
		}
		var previous sql.NullString
		err := db.QueryRowContext(ctx, db.Rebind(`SELECT ota_json FROM peer_runs WHERE public_key=?`), peer.String()).Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("peerrun: read ota: %w", err)
		}
		data := []byte(previous.String)

		candidate := next
		if len(data) > 0 {
			var current apitypes.PeerOtaStatus
			if err := json.Unmarshal(data, &current); err != nil {
				return fmt.Errorf("peerrun: decode ota: %w", err)
			}
			if !newerOTA(current, next) {
				return nil
			}
			if candidate.UpdateId == current.UpdateId && candidate.TargetVersion == nil {
				candidate.TargetVersion = current.TargetVersion
			}
			if candidate.UpdateId == current.UpdateId && candidate.DownloadPercent == nil {
				candidate.DownloadPercent = current.DownloadPercent
			}
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return fmt.Errorf("peerrun: encode ota: %w", err)
		}
		var result sql.Result
		if !previous.Valid {
			result, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO peer_runs(public_key,ota_json) VALUES (?,?) ON CONFLICT(public_key) DO UPDATE SET ota_json=excluded.ota_json WHERE peer_runs.ota_json IS NULL`), peer.String(), string(encoded))
		} else {
			result, err = db.ExecContext(ctx, db.Rebind(`UPDATE peer_runs SET ota_json=? WHERE public_key=? AND ota_json=?`), string(encoded), peer.String(), previous.String)
		}
		if err != nil {
			return fmt.Errorf("peerrun: update ota: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed == 1 {
			return nil
		}

	}
	return fmt.Errorf("peerrun: update ota: concurrent update retry limit reached")
}

func newerOTA(current, next apitypes.PeerOtaStatus) bool {
	if next.ObservedAt.Before(current.ObservedAt) {
		return false
	}
	if next.UpdateId != current.UpdateId {
		return next.ObservedAt.After(current.ObservedAt)
	}
	if current.State == "succeeded" || current.State == "failed" {
		return false
	}
	if current.State == "downloading" && next.State == "started" {
		return false
	}
	if current.DownloadPercent != nil && next.DownloadPercent != nil && *next.DownloadPercent < *current.DownloadPercent {
		return false
	}
	return true
}

func validateOTAStatus(ota apitypes.PeerOtaStatus) error {
	switch ota.State {
	case "started", "downloading", "succeeded", "failed":
	default:
		return fmt.Errorf("%w: invalid ota state", ErrInvalidStatus)
	}
	if ota.ObservedAt.IsZero() || len(ota.UpdateId) == 0 || len(ota.UpdateId) > 128 || !utf8.ValidString(ota.UpdateId) {
		return fmt.Errorf("%w: invalid ota identity or timestamp", ErrInvalidStatus)
	}
	for _, field := range []struct {
		value *string
		limit int
	}{{ota.TargetVersion, 128}, {ota.ErrorCode, 128}, {ota.ErrorMessage, 512}} {
		if field.value != nil && (len(*field.value) > field.limit || !utf8.ValidString(*field.value)) {
			return fmt.Errorf("%w: invalid ota string", ErrInvalidStatus)
		}
	}
	if ota.State == "downloading" && ota.DownloadPercent == nil {
		return fmt.Errorf("%w: ota progress required", ErrInvalidStatus)
	}
	if ota.DownloadPercent != nil && (math.IsNaN(*ota.DownloadPercent) || math.IsInf(*ota.DownloadPercent, 0) || *ota.DownloadPercent < 0 || *ota.DownloadPercent > 100) {
		return fmt.Errorf("%w: invalid ota progress", ErrInvalidStatus)
	}
	if ota.State != "failed" && (ota.ErrorCode != nil || ota.ErrorMessage != nil) {
		return fmt.Errorf("%w: ota errors require failed state", ErrInvalidStatus)
	}
	return nil
}
