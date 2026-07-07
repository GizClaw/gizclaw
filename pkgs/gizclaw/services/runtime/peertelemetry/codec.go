// Package peertelemetry decodes peer telemetry packets and projects them into
// metrics plus the fixed runtime peer status snapshot.
package peertelemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"google.golang.org/protobuf/proto"
)

var (
	ErrInvalidPeer      = errors.New("peertelemetry: invalid peer")
	ErrInvalidFrame     = errors.New("peertelemetry: invalid frame")
	ErrServiceNil       = errors.New("peertelemetry: service is nil")
	ErrStatusServiceNil = errors.New("peertelemetry: status service is nil")
)

type StatusService interface {
	SyncTelemetryStatus(context.Context, giznet.PublicKey, StatusPatch) error
}

type Service struct {
	Metrics metrics.Store
	Status  StatusService
	Now     func() time.Time
}

func (s *Service) ReportPacket(ctx context.Context, peer giznet.PublicKey, payload []byte) error {
	frame, err := Decode(payload)
	if err != nil {
		return err
	}
	return s.Report(ctx, peer, frame)
}

func (s *Service) Report(ctx context.Context, peer giznet.PublicKey, frame *telemetrypb.TelemetryFrame) error {
	if peer.IsZero() {
		return ErrInvalidPeer
	}
	if frame == nil {
		return ErrInvalidFrame
	}
	now := time.Now
	if s != nil && s.Now != nil {
		now = s.Now
	}
	baseTime := observedAt(frame.GetObservedAtUnixMs(), now)
	samples, status, err := MapFrame(peer, frame, baseTime)
	if err != nil {
		return err
	}
	if s == nil {
		return ErrServiceNil
	}
	var errs []error
	if !status.Empty() {
		if s.Status == nil {
			errs = append(errs, ErrStatusServiceNil)
		} else if err := s.Status.SyncTelemetryStatus(ctx, peer, status); err != nil {
			errs = append(errs, fmt.Errorf("peertelemetry: sync status: %w", err))
		}
	}
	if len(samples) > 0 && s.Metrics != nil {
		if err := s.Metrics.Append(ctx, samples); err != nil {
			errs = append(errs, fmt.Errorf("peertelemetry: append metrics: %w", err))
		}
	}
	return errors.Join(errs...)
}

func Decode(payload []byte) (*telemetrypb.TelemetryFrame, error) {
	if len(payload) == 0 {
		return nil, ErrInvalidFrame
	}
	var frame telemetrypb.TelemetryFrame
	if err := proto.Unmarshal(payload, &frame); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFrame, err)
	}
	if len(frame.GetObservations()) == 0 {
		return nil, fmt.Errorf("%w: observations are required", ErrInvalidFrame)
	}
	for _, observation := range frame.GetObservations() {
		if observation == nil || observation.GetBody() == nil {
			return nil, fmt.Errorf("%w: observation body is required", ErrInvalidFrame)
		}
	}
	return &frame, nil
}

func observedAt(unixMillis int64, now func() time.Time) time.Time {
	if unixMillis == 0 {
		return now().UTC()
	}
	return time.UnixMilli(unixMillis).UTC()
}
