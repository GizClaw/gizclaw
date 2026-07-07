package peertelemetry

import (
	"fmt"
	"math"
	"time"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

const (
	MetricBatteryPercent    = "gizclaw_peer_battery_percent"
	MetricBatteryCharging   = "gizclaw_peer_battery_charging"
	MetricBatteryVoltageMv  = "gizclaw_peer_battery_voltage_mv"
	MetricGNSSLatitude      = "gizclaw_peer_gnss_latitude"
	MetricGNSSLongitude     = "gizclaw_peer_gnss_longitude"
	MetricGNSSAltitudeM     = "gizclaw_peer_gnss_altitude_m"
	MetricGNSSAccuracyM     = "gizclaw_peer_gnss_accuracy_m"
	MetricNetworkRSSIDbm    = "gizclaw_peer_network_rssi_dbm"
	MetricNetworkSignal     = "gizclaw_peer_network_signal_level"
	MetricNetworkConnected  = "gizclaw_peer_network_connected"
	MetricSystemUptime      = "gizclaw_peer_system_uptime_seconds"
	MetricSystemFreeMemory  = "gizclaw_peer_system_free_memory_bytes"
	MetricSystemTemperature = "gizclaw_peer_system_temperature_c"
)

type StatusPatch struct {
	ReportedAt     time.Time
	BatteryPercent *int
	Charging       *bool
}

func (p StatusPatch) Empty() bool {
	return p.BatteryPercent == nil && p.Charging == nil
}

func MapFrame(peer giznet.PublicKey, frame *telemetrypb.TelemetryFrame, baseTime time.Time) ([]metrics.Sample, StatusPatch, error) {
	if peer.IsZero() {
		return nil, StatusPatch{}, ErrInvalidPeer
	}
	if frame == nil {
		return nil, StatusPatch{}, ErrInvalidFrame
	}
	labels := map[string]string{"peer_id": peer.String()}
	samples := make([]metrics.Sample, 0, len(frame.GetObservations())*2)
	status := StatusPatch{}
	for _, observation := range frame.GetObservations() {
		if observation == nil || observation.GetBody() == nil {
			return nil, StatusPatch{}, fmt.Errorf("%w: observation body is required", ErrInvalidFrame)
		}
		ts := baseTime.Add(time.Duration(observation.GetObservedAtDeltaMs()) * time.Millisecond).UTC()
		switch body := observation.GetBody().(type) {
		case *telemetrypb.Observation_Battery:
			next, patch, err := mapBattery(body.Battery, labels, ts)
			if err != nil {
				return nil, StatusPatch{}, err
			}
			samples = append(samples, next...)
			mergeStatusPatch(&status, patch)
		case *telemetrypb.Observation_Gnss:
			next, err := mapGNSS(body.Gnss, labels, ts)
			if err != nil {
				return nil, StatusPatch{}, err
			}
			samples = append(samples, next...)
		case *telemetrypb.Observation_Network:
			next, err := mapNetwork(body.Network, labels, ts)
			if err != nil {
				return nil, StatusPatch{}, err
			}
			samples = append(samples, next...)
		case *telemetrypb.Observation_System:
			next, err := mapSystem(body.System, labels, ts)
			if err != nil {
				return nil, StatusPatch{}, err
			}
			samples = append(samples, next...)
		default:
			return nil, StatusPatch{}, fmt.Errorf("%w: unsupported observation body %T", ErrInvalidFrame, body)
		}
	}
	return samples, status, nil
}

func mapBattery(obs *telemetrypb.BatteryObservation, labels map[string]string, ts time.Time) ([]metrics.Sample, StatusPatch, error) {
	if obs == nil {
		return nil, StatusPatch{}, fmt.Errorf("%w: battery observation is nil", ErrInvalidFrame)
	}
	samples := make([]metrics.Sample, 0, 3)
	patch := StatusPatch{ReportedAt: ts}
	if obs.Percent != nil {
		percent := *obs.Percent
		if err := validateFiniteRange("battery percent", percent, 0, 100); err != nil {
			return nil, StatusPatch{}, err
		}
		samples = append(samples, sample(MetricBatteryPercent, labels, ts, percent))
		asInt := int(math.Round(percent))
		patch.BatteryPercent = &asInt
	}
	if obs.Charging != nil {
		samples = append(samples, sample(MetricBatteryCharging, labels, ts, boolValue(*obs.Charging)))
		charging := *obs.Charging
		patch.Charging = &charging
	}
	if obs.VoltageMv != nil {
		voltage := *obs.VoltageMv
		if err := validateFinite("battery voltage_mv", voltage); err != nil {
			return nil, StatusPatch{}, err
		}
		samples = append(samples, sample(MetricBatteryVoltageMv, labels, ts, voltage))
	}
	return samples, patch, nil
}

func mapGNSS(obs *telemetrypb.GnssObservation, labels map[string]string, ts time.Time) ([]metrics.Sample, error) {
	if obs == nil {
		return nil, fmt.Errorf("%w: gnss observation is nil", ErrInvalidFrame)
	}
	if err := validateFiniteRange("gnss latitude", obs.GetLatitude(), -90, 90); err != nil {
		return nil, err
	}
	if err := validateFiniteRange("gnss longitude", obs.GetLongitude(), -180, 180); err != nil {
		return nil, err
	}
	samples := []metrics.Sample{
		sample(MetricGNSSLatitude, labels, ts, obs.GetLatitude()),
		sample(MetricGNSSLongitude, labels, ts, obs.GetLongitude()),
	}
	if obs.AltitudeM != nil {
		if err := validateFinite("gnss altitude_m", *obs.AltitudeM); err != nil {
			return nil, err
		}
		samples = append(samples, sample(MetricGNSSAltitudeM, labels, ts, *obs.AltitudeM))
	}
	if obs.AccuracyM != nil {
		if err := validateFinite("gnss accuracy_m", *obs.AccuracyM); err != nil {
			return nil, err
		}
		samples = append(samples, sample(MetricGNSSAccuracyM, labels, ts, *obs.AccuracyM))
	}
	return samples, nil
}

func mapNetwork(obs *telemetrypb.NetworkObservation, labels map[string]string, ts time.Time) ([]metrics.Sample, error) {
	if obs == nil {
		return nil, fmt.Errorf("%w: network observation is nil", ErrInvalidFrame)
	}
	samples := make([]metrics.Sample, 0, 3)
	if obs.RssiDbm != nil {
		if err := validateFinite("network rssi_dbm", *obs.RssiDbm); err != nil {
			return nil, err
		}
		samples = append(samples, sample(MetricNetworkRSSIDbm, labels, ts, *obs.RssiDbm))
	}
	if obs.SignalLevel != nil {
		if err := validateFinite("network signal_level", *obs.SignalLevel); err != nil {
			return nil, err
		}
		samples = append(samples, sample(MetricNetworkSignal, labels, ts, *obs.SignalLevel))
	}
	if obs.Connected != nil {
		samples = append(samples, sample(MetricNetworkConnected, labels, ts, boolValue(*obs.Connected)))
	}
	return samples, nil
}

func mapSystem(obs *telemetrypb.SystemObservation, labels map[string]string, ts time.Time) ([]metrics.Sample, error) {
	if obs == nil {
		return nil, fmt.Errorf("%w: system observation is nil", ErrInvalidFrame)
	}
	samples := make([]metrics.Sample, 0, 3)
	if obs.UptimeSeconds != nil {
		if err := validateFinite("system uptime_seconds", *obs.UptimeSeconds); err != nil {
			return nil, err
		}
		samples = append(samples, sample(MetricSystemUptime, labels, ts, *obs.UptimeSeconds))
	}
	if obs.FreeMemoryBytes != nil {
		if err := validateFinite("system free_memory_bytes", *obs.FreeMemoryBytes); err != nil {
			return nil, err
		}
		samples = append(samples, sample(MetricSystemFreeMemory, labels, ts, *obs.FreeMemoryBytes))
	}
	if obs.TemperatureC != nil {
		if err := validateFinite("system temperature_c", *obs.TemperatureC); err != nil {
			return nil, err
		}
		samples = append(samples, sample(MetricSystemTemperature, labels, ts, *obs.TemperatureC))
	}
	return samples, nil
}

func sample(name string, labels map[string]string, ts time.Time, value float64) metrics.Sample {
	return metrics.Sample{
		Name:      name,
		Labels:    cloneLabels(labels),
		Timestamp: ts,
		Value:     value,
	}
}

func cloneLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		out[k] = v
	}
	return out
}

func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func mergeStatusPatch(dst *StatusPatch, src StatusPatch) {
	if !dst.ReportedAt.IsZero() && src.ReportedAt.Before(dst.ReportedAt) {
		return
	}
	if src.ReportedAt.After(dst.ReportedAt) || dst.ReportedAt.IsZero() {
		dst.ReportedAt = src.ReportedAt
	}
	if src.BatteryPercent != nil {
		dst.BatteryPercent = src.BatteryPercent
	}
	if src.Charging != nil {
		dst.Charging = src.Charging
	}
}

func validateFinite(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%w: %s must be finite", ErrInvalidFrame, name)
	}
	return nil
}

func validateFiniteRange(name string, value, min, max float64) error {
	if err := validateFinite(name, value); err != nil {
		return err
	}
	if value < min || value > max {
		return fmt.Errorf("%w: %s must be between %g and %g", ErrInvalidFrame, name, min, max)
	}
	return nil
}
