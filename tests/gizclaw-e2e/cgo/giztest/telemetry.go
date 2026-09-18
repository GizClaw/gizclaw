package main

/*
#include "bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"google.golang.org/protobuf/encoding/protojson"
)

// decodeTelemetryFrame reads a step's protobuf-JSON TelemetryFrame with the
// same rules as the Go runner: at least one observation, each with a body.
func decodeTelemetryFrame(input any) (*telemetrypb.TelemetryFrame, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	frame := new(telemetrypb.TelemetryFrame)
	if err := protojson.Unmarshal(data, frame); err != nil {
		return nil, err
	}
	if len(frame.Observations) == 0 {
		return nil, fmt.Errorf("telemetry requires observations")
	}
	for _, observation := range frame.Observations {
		if observation == nil || observation.Body == nil {
			return nil, fmt.Errorf("telemetry requires observation bodies")
		}
	}
	return frame, nil
}

/*
cTelemetry is one TelemetryFrame mapped onto the C SDK's typed telemetry API.

The C SDK sends OTA through its own additive frame call, so a document frame
that mixes OTA with other observations becomes one gzc_telemetry_frame_t for
the rest plus one gzc_telemetry_ota_frame_t per OTA observation, all stamped
with the document's sequence and observed_at_unix_ms. Every pointer lives in C
memory the arena owns until free.
*/
type cTelemetry struct {
	frame *C.gzc_telemetry_frame_t
	otas  []*C.gzc_telemetry_ota_frame_t
	arena []unsafe.Pointer
}

func (t *cTelemetry) alloc(size C.size_t) unsafe.Pointer {
	ptr := C.calloc(1, size)
	if ptr == nil {
		panic("calloc failed")
	}
	t.arena = append(t.arena, ptr)
	return ptr
}

func (t *cTelemetry) str(value string) C.gzc_str_t {
	if value == "" {
		// A non-NULL pointer keeps an explicitly empty string encodable.
		return C.gzc_str_t{data: (*C.char)(t.alloc(1)), len: 0}
	}
	data := C.CString(value)
	t.arena = append(t.arena, unsafe.Pointer(data))
	return C.gzc_str_t{data: data, len: C.size_t(len(value))}
}

func (t *cTelemetry) free() {
	for _, ptr := range t.arena {
		C.free(ptr)
	}
	t.arena = nil
	t.frame = nil
	t.otas = nil
}

// newCTelemetry maps frame onto the C structs. Callers must call free.
func newCTelemetry(frame *telemetrypb.TelemetryFrame) (*cTelemetry, error) {
	t := &cTelemetry{}
	var rest []*telemetrypb.Observation
	for _, observation := range frame.GetObservations() {
		if ota := observation.GetOta(); ota != nil {
			t.otas = append(t.otas, t.otaFrame(frame, observation.GetObservedAtDeltaMs(), ota))
			continue
		}
		rest = append(rest, observation)
	}
	if len(rest) == 0 {
		return t, nil
	}
	size := C.size_t(unsafe.Sizeof(C.gzc_telemetry_observation_t{}))
	array := (*C.gzc_telemetry_observation_t)(t.alloc(size * C.size_t(len(rest))))
	observations := unsafe.Slice(array, len(rest))
	for i, observation := range rest {
		if err := t.fillObservation(&observations[i], observation); err != nil {
			t.free()
			return nil, err
		}
	}
	t.frame = (*C.gzc_telemetry_frame_t)(t.alloc(C.size_t(unsafe.Sizeof(C.gzc_telemetry_frame_t{}))))
	t.frame.sequence = C.uint32_t(frame.GetSequence())
	t.frame.observed_at_unix_ms = C.int64_t(frame.GetObservedAtUnixMs())
	t.frame.observations = array
	t.frame.observation_count = C.size_t(len(rest))
	return t, nil
}

func (t *cTelemetry) otaFrame(
	frame *telemetrypb.TelemetryFrame, delta int32, ota *telemetrypb.OtaObservation,
) *C.gzc_telemetry_ota_frame_t {
	out := (*C.gzc_telemetry_ota_frame_t)(t.alloc(C.size_t(unsafe.Sizeof(C.gzc_telemetry_ota_frame_t{}))))
	out.sequence = C.uint32_t(frame.GetSequence())
	out.observed_at_unix_ms = C.int64_t(frame.GetObservedAtUnixMs())
	out.observed_at_delta_ms = C.int32_t(delta)
	out.ota.state = C.gzc_ota_state_t(ota.GetState())
	out.ota.update_id = t.str(ota.GetUpdateId())
	if ota.TargetVersion != nil {
		out.ota.has_target_version, out.ota.target_version = true, t.str(ota.GetTargetVersion())
	}
	if ota.DownloadPercent != nil {
		out.ota.has_download_percent, out.ota.download_percent = true, C.double(ota.GetDownloadPercent())
	}
	if ota.ErrorCode != nil {
		out.ota.has_error_code, out.ota.error_code = true, t.str(ota.GetErrorCode())
	}
	if ota.ErrorMessage != nil {
		out.ota.has_error_message, out.ota.error_message = true, t.str(ota.GetErrorMessage())
	}
	return out
}

func (t *cTelemetry) fillObservation(out *C.gzc_telemetry_observation_t, observation *telemetrypb.Observation) error {
	out.observed_at_delta_ms = C.int32_t(observation.GetObservedAtDeltaMs())
	switch body := observation.Body.(type) {
	case *telemetrypb.Observation_Battery:
		out.kind = C.GZC_TELEMETRY_OBSERVATION_BATTERY
		battery := body.Battery
		if battery.Percent != nil {
			out.battery.has_percent, out.battery.percent = true, C.double(battery.GetPercent())
		}
		if battery.Charging != nil {
			out.battery.has_charging, out.battery.charging = true, C.bool(battery.GetCharging())
		}
		if battery.VoltageMv != nil {
			out.battery.has_voltage_mv, out.battery.voltage_mv = true, C.double(battery.GetVoltageMv())
		}
	case *telemetrypb.Observation_Gnss:
		out.kind = C.GZC_TELEMETRY_OBSERVATION_GNSS
		gnss := body.Gnss
		out.gnss.latitude, out.gnss.longitude = C.double(gnss.GetLatitude()), C.double(gnss.GetLongitude())
		if gnss.AltitudeM != nil {
			out.gnss.has_altitude_m, out.gnss.altitude_m = true, C.double(gnss.GetAltitudeM())
		}
		if gnss.AccuracyM != nil {
			out.gnss.has_accuracy_m, out.gnss.accuracy_m = true, C.double(gnss.GetAccuracyM())
		}
	case *telemetrypb.Observation_Network:
		out.kind = C.GZC_TELEMETRY_OBSERVATION_NETWORK
		network := body.Network
		if network.RssiDbm != nil {
			out.network.has_rssi_dbm, out.network.rssi_dbm = true, C.double(network.GetRssiDbm())
		}
		if network.SignalLevel != nil {
			out.network.has_signal_level, out.network.signal_level = true, C.double(network.GetSignalLevel())
		}
		if network.Rat != nil {
			out.network.has_rat, out.network.rat = true, t.str(network.GetRat())
		}
		if network.Operator != nil {
			out.network.has_operator_name, out.network.operator_name = true, t.str(network.GetOperator())
		}
		if network.Connected != nil {
			out.network.has_connected, out.network.connected = true, C.bool(network.GetConnected())
		}
		if network.Imei != nil {
			out.network.has_imei, out.network.imei = true, t.str(network.GetImei())
		}
		if network.Imsi != nil {
			out.network.has_imsi, out.network.imsi = true, t.str(network.GetImsi())
		}
	case *telemetrypb.Observation_System:
		out.kind = C.GZC_TELEMETRY_OBSERVATION_SYSTEM
		system := body.System
		if system.UptimeSeconds != nil {
			out.system.has_uptime_seconds, out.system.uptime_seconds = true, C.double(system.GetUptimeSeconds())
		}
		if system.FreeMemoryBytes != nil {
			out.system.has_free_memory_bytes, out.system.free_memory_bytes = true, C.double(system.GetFreeMemoryBytes())
		}
		if system.TemperatureC != nil {
			out.system.has_temperature_c, out.system.temperature_c = true, C.double(system.GetTemperatureC())
		}
		if system.FirmwareVersion != nil {
			out.system.has_firmware_version, out.system.firmware_version = true, t.str(system.GetFirmwareVersion())
		}
		if system.SoftwareVersion != nil {
			out.system.has_software_version, out.system.software_version = true, t.str(system.GetSoftwareVersion())
		}
		if system.HardwareVersion != nil {
			out.system.has_hardware_version, out.system.hardware_version = true, t.str(system.GetHardwareVersion())
		}
	case *telemetrypb.Observation_Audioplayer:
		out.kind = C.GZC_TELEMETRY_OBSERVATION_AUDIOPLAYER
		player := body.Audioplayer
		out.audioplayer.state = t.str(player.GetState())
		if player.CurrentIndex != nil {
			out.audioplayer.has_current_index, out.audioplayer.current_index = true, C.uint32_t(player.GetCurrentIndex())
		}
		out.audioplayer.position_ms = C.uint64_t(player.GetPositionMs())
		if player.DurationMs != nil {
			out.audioplayer.has_duration_ms, out.audioplayer.duration_ms = true, C.uint64_t(player.GetDurationMs())
		}
		out.audioplayer.repeat = t.str(player.GetRepeat())
		out.audioplayer.playlist_length = C.uint32_t(player.GetPlaylistLength())
		out.audioplayer.playlist_revision = C.uint32_t(player.GetPlaylistRevision())
		if player.ErrorCode != nil {
			out.audioplayer.has_error_code, out.audioplayer.error_code = true, t.str(player.GetErrorCode())
		}
		if player.ErrorMessage != nil {
			out.audioplayer.has_error_message, out.audioplayer.error_message = true, t.str(player.GetErrorMessage())
		}
	case *telemetrypb.Observation_Activity:
		out.kind = C.GZC_TELEMETRY_OBSERVATION_ACTIVITY
		activity := body.Activity
		out.activity.activity = t.str(activity.GetActivity())
		if activity.Detail != nil {
			out.activity.has_detail, out.activity.detail = true, t.str(activity.GetDetail())
		}
	default:
		return fmt.Errorf("the C SDK has no telemetry observation for %T", observation.Body)
	}
	return nil
}

// SendTelemetry sends frame through the C SDK's typed telemetry calls.
func (s *cSession) SendTelemetry(frame *telemetrypb.TelemetryFrame) error {
	mapped, err := newCTelemetry(frame)
	if err != nil {
		return err
	}
	defer mapped.free()
	errbuf, freeErr := newErrorBuffer()
	defer freeErr()
	if mapped.frame != nil {
		if rc := C.gzt_session_send_telemetry(s.handle, mapped.frame, errbuf, errorBufferSize); rc != 0 {
			return bridgeFailure("send telemetry", rc, errbuf)
		}
	}
	for _, ota := range mapped.otas {
		if rc := C.gzt_session_send_ota_telemetry(s.handle, ota, errbuf, errorBufferSize); rc != 0 {
			return bridgeFailure("send OTA telemetry", rc, errbuf)
		}
	}
	return nil
}

// encodeTelemetry encodes frame with the C SDK's encoders, returning the
// payload of the non-OTA frame and one payload per OTA frame. Tests use it to
// check the mapping against the Go protobuf decoding.
func encodeTelemetry(frame *telemetrypb.TelemetryFrame) ([]byte, [][]byte, error) {
	mapped, err := newCTelemetry(frame)
	if err != nil {
		return nil, nil, err
	}
	defer mapped.free()
	platform := C.gzc_default_platform()
	take := func(buf *C.gzc_buf_t) []byte {
		defer C.gzc_buf_free(buf, platform)
		return C.GoBytes(unsafe.Pointer(buf.data), C.int(buf.len))
	}
	var main []byte
	if mapped.frame != nil {
		var buf C.gzc_buf_t
		C.gzc_buf_init(&buf)
		if rc := C.gzc_telemetry_encode_frame(mapped.frame, platform, &buf); rc != 0 {
			C.gzc_buf_free(&buf, platform)
			return nil, nil, fmt.Errorf("encode telemetry frame: rc=%d", int(rc))
		}
		main = take(&buf)
	}
	otas := make([][]byte, 0, len(mapped.otas))
	for _, ota := range mapped.otas {
		var buf C.gzc_buf_t
		C.gzc_buf_init(&buf)
		if rc := C.gzc_telemetry_encode_ota_frame(ota, platform, &buf); rc != 0 {
			C.gzc_buf_free(&buf, platform)
			return nil, nil, fmt.Errorf("encode OTA telemetry frame: rc=%d", int(rc))
		}
		otas = append(otas, take(&buf))
	}
	return main, otas, nil
}
