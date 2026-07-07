package cgobackend

/*
#cgo CFLAGS: -I. -I../include -I../generated
#include "gzc_cgo_backend.h"
#include <stdlib.h>
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"google.golang.org/protobuf/proto"
)

type cgoSink struct {
	cBackend unsafe.Pointer
}

func backendFromHandle(handle C.uint64_t) *Backend {
	if handle == 0 {
		return nil
	}
	h := cgo.Handle(uintptr(handle))
	b, _ := h.Value().(*Backend)
	return b
}

//export gzcGoBackendCreate
func gzcGoBackendCreate() C.uint64_t {
	b := New()
	return C.uint64_t(cgo.NewHandle(b))
}

//export gzcGoBackendDestroy
func gzcGoBackendDestroy(handle C.uint64_t) {
	if b := backendFromHandle(handle); b != nil {
		b.SetEventSink(nil)
		b.Close()
	}
	cgo.Handle(uintptr(handle)).Delete()
}

//export gzcGoBackendSetCBackend
func gzcGoBackendSetCBackend(handle C.uint64_t, cBackend *C.gzc_cgo_backend_t) {
	if b := backendFromHandle(handle); b != nil {
		b.SetEventSink(cgoSink{cBackend: unsafe.Pointer(cBackend)})
	}
}

//export gzcGoRandom
func gzcGoRandom(out *C.uint8_t, length C.size_t) C.int {
	if length == 0 {
		return C.GZC_OK
	}
	if out == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	n, ok := cIntLen(length)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := Random(unsafe.Slice((*byte)(unsafe.Pointer(out)), n)); err != nil {
		return C.GZC_ERR_SIGNALING
	}
	return C.GZC_OK
}

//export gzcGoTimeUnixMs
func gzcGoTimeUnixMs() C.int64_t {
	return C.int64_t(TimeUnixMs())
}

//export gzcGoHTTPRequest
func gzcGoHTTPRequest(handle C.uint64_t, method C.int, urlData *C.char, urlLen C.size_t, headers *C.gzc_http_header_t, headerCount C.size_t, data *C.uint8_t, length C.size_t, outStatus *C.int, outData **C.uint8_t, outLen *C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil || urlData == nil || outStatus == nil || outData == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	url, ok := goString(urlData, urlLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	body, ok := goBytes(unsafe.Pointer(data), length)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	headerList, ok := cHeaders(headers, headerCount)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	resp, err := b.HTTPRequest(int(method), url, headerList, body)
	if err != nil {
		return C.GZC_ERR_HTTP
	}
	*outStatus = C.int(resp.StatusCode)
	if len(resp.Body) > 0 {
		*outData = (*C.uint8_t)(C.CBytes(resp.Body))
		*outLen = C.size_t(len(resp.Body))
	} else {
		*outData = nil
		*outLen = 0
	}
	return C.GZC_OK
}

//export gzcGoTelemetryEncode
func gzcGoTelemetryEncode(frame *C.gzc_telemetry_frame_t, outData **C.uint8_t, outLen *C.size_t) C.int {
	if frame == nil || outData == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	observationsLen, ok := cIntLen(frame.observation_count)
	if !ok || observationsLen == 0 || frame.observations == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	observations := unsafe.Slice(frame.observations, observationsLen)
	msg := &telemetrypb.TelemetryFrame{
		Sequence:         uint32(frame.sequence),
		ObservedAtUnixMs: int64(frame.observed_at_unix_ms),
		Observations:     make([]*telemetrypb.Observation, 0, observationsLen),
	}
	for _, observation := range observations {
		next, ok := cTelemetryObservation(observation)
		if !ok {
			return C.GZC_ERR_INVALID_ARGUMENT
		}
		msg.Observations = append(msg.Observations, next)
	}
	encoded, err := proto.Marshal(msg)
	if err != nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if len(encoded) == 0 {
		*outData = nil
		*outLen = 0
		return C.GZC_OK
	}
	*outData = (*C.uint8_t)(C.CBytes(encoded))
	*outLen = C.size_t(len(encoded))
	return C.GZC_OK
}

func cTelemetryObservation(observation C.gzc_telemetry_observation_t) (*telemetrypb.Observation, bool) {
	out := &telemetrypb.Observation{ObservedAtDeltaMs: int32(observation.observed_at_delta_ms)}
	switch observation.kind {
	case C.GZC_TELEMETRY_OBSERVATION_BATTERY:
		out.Body = &telemetrypb.Observation_Battery{Battery: cTelemetryBattery(observation.battery)}
	case C.GZC_TELEMETRY_OBSERVATION_GNSS:
		out.Body = &telemetrypb.Observation_Gnss{Gnss: cTelemetryGNSS(observation.gnss)}
	case C.GZC_TELEMETRY_OBSERVATION_NETWORK:
		network, ok := cTelemetryNetwork(observation.network)
		if !ok {
			return nil, false
		}
		out.Body = &telemetrypb.Observation_Network{Network: network}
	case C.GZC_TELEMETRY_OBSERVATION_SYSTEM:
		system, ok := cTelemetrySystem(observation.system)
		if !ok {
			return nil, false
		}
		out.Body = &telemetrypb.Observation_System{System: system}
	default:
		return nil, false
	}
	return out, true
}

func cTelemetryBattery(value C.gzc_telemetry_battery_t) *telemetrypb.BatteryObservation {
	out := &telemetrypb.BatteryObservation{}
	if value.has_percent {
		v := float64(value.percent)
		out.Percent = &v
	}
	if value.has_charging {
		v := bool(value.charging)
		out.Charging = &v
	}
	if value.has_voltage_mv {
		v := float64(value.voltage_mv)
		out.VoltageMv = &v
	}
	return out
}

func cTelemetryGNSS(value C.gzc_telemetry_gnss_t) *telemetrypb.GnssObservation {
	out := &telemetrypb.GnssObservation{
		Latitude:  float64(value.latitude),
		Longitude: float64(value.longitude),
	}
	if value.has_altitude_m {
		v := float64(value.altitude_m)
		out.AltitudeM = &v
	}
	if value.has_accuracy_m {
		v := float64(value.accuracy_m)
		out.AccuracyM = &v
	}
	return out
}

func cTelemetryNetwork(value C.gzc_telemetry_network_t) (*telemetrypb.NetworkObservation, bool) {
	out := &telemetrypb.NetworkObservation{}
	if value.has_rssi_dbm {
		v := float64(value.rssi_dbm)
		out.RssiDbm = &v
	}
	if value.has_signal_level {
		v := float64(value.signal_level)
		out.SignalLevel = &v
	}
	if value.has_rat {
		v, ok := goString(value.rat.data, value.rat.len)
		if !ok {
			return nil, false
		}
		out.Rat = &v
	}
	if value.has_operator_name {
		v, ok := goString(value.operator_name.data, value.operator_name.len)
		if !ok {
			return nil, false
		}
		out.Operator = &v
	}
	if value.has_connected {
		v := bool(value.connected)
		out.Connected = &v
	}
	return out, true
}

func cTelemetrySystem(value C.gzc_telemetry_system_t) (*telemetrypb.SystemObservation, bool) {
	out := &telemetrypb.SystemObservation{}
	if value.has_uptime_seconds {
		v := float64(value.uptime_seconds)
		out.UptimeSeconds = &v
	}
	if value.has_free_memory_bytes {
		v := float64(value.free_memory_bytes)
		out.FreeMemoryBytes = &v
	}
	if value.has_temperature_c {
		v := float64(value.temperature_c)
		out.TemperatureC = &v
	}
	if value.has_firmware_version {
		v, ok := goString(value.firmware_version.data, value.firmware_version.len)
		if !ok {
			return nil, false
		}
		out.FirmwareVersion = &v
	}
	if value.has_software_version {
		v, ok := goString(value.software_version.data, value.software_version.len)
		if !ok {
			return nil, false
		}
		out.SoftwareVersion = &v
	}
	if value.has_hardware_version {
		v, ok := goString(value.hardware_version.data, value.hardware_version.len)
		if !ok {
			return nil, false
		}
		out.HardwareVersion = &v
	}
	return out, true
}

//export gzcGoKeyPairFromPrivate
func gzcGoKeyPairFromPrivate(privateKey *C.uint8_t, outPrivateKey *C.uint8_t, outPublicKey *C.uint8_t) C.int {
	if privateKey == nil || outPrivateKey == nil || outPublicKey == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	private := unsafe.Slice((*byte)(unsafe.Pointer(privateKey)), giznet.KeySize)
	kp, err := KeyPairFromPrivate(private)
	if err != nil {
		return C.GZC_ERR_SIGNALING
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(outPrivateKey)), giznet.KeySize), kp.Private[:])
	copy(unsafe.Slice((*byte)(unsafe.Pointer(outPublicKey)), giznet.KeySize), kp.Public[:])
	return C.GZC_OK
}

//export gzcGoDH
func gzcGoDH(privateKey *C.uint8_t, remotePublicKey *C.uint8_t, outShared *C.uint8_t) C.int {
	if privateKey == nil || remotePublicKey == nil || outShared == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	shared, err := DH(
		unsafe.Slice((*byte)(unsafe.Pointer(privateKey)), giznet.KeySize),
		unsafe.Slice((*byte)(unsafe.Pointer(remotePublicKey)), giznet.KeySize),
	)
	if err != nil {
		return C.GZC_ERR_SIGNALING
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(outShared)), giznet.KeySize), shared[:])
	return C.GZC_OK
}

//export gzcGoHKDFSHA256
func gzcGoHKDFSHA256(secret *C.uint8_t, secretLen C.size_t, salt *C.uint8_t, saltLen C.size_t, info *C.char, infoLen C.size_t, out *C.uint8_t, outLen C.size_t) C.int {
	if secret == nil || info == nil || out == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	secretBytes, ok := goBytes(unsafe.Pointer(secret), secretLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	saltBytes, ok := goBytes(unsafe.Pointer(salt), saltLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	infoText, ok := goString(info, infoLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	outLenInt, ok := cIntLen(outLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := HKDFSHA256(secretBytes, saltBytes, infoText, unsafe.Slice((*byte)(unsafe.Pointer(out)), outLenInt)); err != nil {
		return C.GZC_ERR_SIGNALING
	}
	return C.GZC_OK
}

//export gzcGoAEADSeal
func gzcGoAEADSeal(mode C.int, key *C.uint8_t, keyLen C.size_t, nonce *C.uint8_t, nonceLen C.size_t, plaintext *C.uint8_t, plaintextLen C.size_t, aad *C.uint8_t, aadLen C.size_t, outData **C.uint8_t, outLen *C.size_t) C.int {
	return gzcGoAEAD(true, mode, key, keyLen, nonce, nonceLen, plaintext, plaintextLen, aad, aadLen, outData, outLen)
}

//export gzcGoAEADOpen
func gzcGoAEADOpen(mode C.int, key *C.uint8_t, keyLen C.size_t, nonce *C.uint8_t, nonceLen C.size_t, ciphertext *C.uint8_t, ciphertextLen C.size_t, aad *C.uint8_t, aadLen C.size_t, outData **C.uint8_t, outLen *C.size_t) C.int {
	return gzcGoAEAD(false, mode, key, keyLen, nonce, nonceLen, ciphertext, ciphertextLen, aad, aadLen, outData, outLen)
}

func gzcGoAEAD(seal bool, mode C.int, key *C.uint8_t, keyLen C.size_t, nonce *C.uint8_t, nonceLen C.size_t, input *C.uint8_t, inputLen C.size_t, aad *C.uint8_t, aadLen C.size_t, outData **C.uint8_t, outLen *C.size_t) C.int {
	if key == nil || nonce == nil || outData == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	keyBytes, ok := goBytes(unsafe.Pointer(key), keyLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	nonceBytes, ok := goBytes(unsafe.Pointer(nonce), nonceLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	inputBytes, ok := goBytes(unsafe.Pointer(input), inputLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	aadBytes, ok := goBytes(unsafe.Pointer(aad), aadLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	var out []byte
	var err error
	if seal {
		out, err = AEADSeal(int(mode), keyBytes, nonceBytes, inputBytes, aadBytes)
	} else {
		out, err = AEADOpen(int(mode), keyBytes, nonceBytes, inputBytes, aadBytes)
	}
	if err != nil {
		return C.GZC_ERR_SIGNALING
	}
	if len(out) > 0 {
		*outData = (*C.uint8_t)(C.CBytes(out))
		*outLen = C.size_t(len(out))
	} else {
		*outData = nil
		*outLen = 0
	}
	return C.GZC_OK
}

//export gzcGoPeerCreate
func gzcGoPeerCreate(handle C.uint64_t) C.int {
	if b := backendFromHandle(handle); b != nil {
		if err := b.CreatePeer(); err != nil {
			return C.GZC_ERR_WEBRTC
		}
		return C.GZC_OK
	}
	return C.GZC_ERR_INVALID_ARGUMENT
}

//export gzcGoPeerStartOffer
func gzcGoPeerStartOffer(handle C.uint64_t, outSDP **C.char, outLen *C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil || outSDP == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	sdp, err := b.StartOffer()
	if err != nil {
		return C.GZC_ERR_WEBRTC
	}
	mem := C.CBytes([]byte(sdp))
	*outSDP = (*C.char)(mem)
	*outLen = C.size_t(len(sdp))
	return C.GZC_OK
}

//export gzcGoPeerSetRemoteSDP
func gzcGoPeerSetRemoteSDP(handle C.uint64_t, sdp *C.char, length C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	value, ok := goString(sdp, length)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.SetRemoteSDP(value); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoPeerCreateDataChannel
func gzcGoPeerCreateDataChannel(handle C.uint64_t, label *C.char, length C.size_t, channelID C.int, ordered C.bool, reliable C.bool) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	labelValue, ok := goString(label, length)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.CreateDataChannel(labelValue, int(channelID), bool(ordered), bool(reliable)); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoPeerPoll
func gzcGoPeerPoll(handle C.uint64_t, timeoutMS C.int) C.int {
	if b := backendFromHandle(handle); b != nil {
		b.Poll(int(timeoutMS))
		return C.GZC_OK
	}
	return C.GZC_ERR_INVALID_ARGUMENT
}

//export gzcGoChannelSend
func gzcGoChannelSend(handle C.uint64_t, channelID C.int, data *C.uint8_t, length C.size_t, isText C.bool) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	payload, ok := goBytes(unsafe.Pointer(data), length)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.Send(int(channelID), payload, bool(isText)); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoChannelClose
func gzcGoChannelClose(handle C.uint64_t, channelID C.int) {
	if b := backendFromHandle(handle); b != nil {
		b.CloseDataChannel(int(channelID))
	}
}

//export gzcGoPeerClose
func gzcGoPeerClose(handle C.uint64_t) {
	if b := backendFromHandle(handle); b != nil {
		b.Close()
	}
}

func (s cgoSink) ChannelState(channelID int, state int) {
	if s.cBackend == nil {
		return
	}
	C.gzc_cgo_emit_channel_state(
		(*C.gzc_cgo_backend_t)(s.cBackend),
		C.int(channelID),
		C.gzc_rtc_channel_state_t(state),
	)
}

func (s cgoSink) ChannelMessage(channelID int, data []byte, isText bool) {
	if s.cBackend == nil {
		return
	}
	raw := C.CBytes(data)
	defer C.free(raw)
	C.gzc_cgo_emit_channel_message(
		(*C.gzc_cgo_backend_t)(s.cBackend),
		C.int(channelID),
		(*C.uint8_t)(raw),
		C.size_t(len(data)),
		C.bool(isText),
	)
}

func writeCString(dst *C.char, cap C.size_t, value string) C.int {
	if dst == nil || cap == 0 {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	capInt, ok := cIntLen(cap)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if len(value)+1 > capInt {
		return C.GZC_ERR_NO_MEMORY
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(dst)), capInt)
	copy(buf, value)
	buf[len(value)] = 0
	return C.GZC_OK
}

func goBytes(ptr unsafe.Pointer, n C.size_t) ([]byte, bool) {
	if n == 0 {
		return nil, true
	}
	if ptr == nil {
		return nil, false
	}
	i, ok := cIntLen(n)
	if !ok {
		return nil, false
	}
	return C.GoBytes(ptr, C.int(i)), true
}

func goString(ptr *C.char, n C.size_t) (string, bool) {
	if n == 0 {
		return "", true
	}
	if ptr == nil {
		return "", false
	}
	i, ok := cIntLen(n)
	if !ok {
		return "", false
	}
	return C.GoStringN(ptr, C.int(i)), true
}

func cIntLen(n C.size_t) (int, bool) {
	const maxCInt = int64(1<<31 - 1)
	if uint64(n) > uint64(maxCInt) {
		return 0, false
	}
	return int(n), true
}

func cHeaders(headers *C.gzc_http_header_t, count C.size_t) ([]HTTPHeader, bool) {
	if headers == nil || count == 0 {
		return nil, true
	}
	countInt, ok := cIntLen(count)
	if !ok {
		return nil, false
	}
	out := make([]HTTPHeader, 0, countInt)
	size := unsafe.Sizeof(*headers)
	base := uintptr(unsafe.Pointer(headers))
	for i := 0; i < countInt; i++ {
		header := (*C.gzc_http_header_t)(unsafe.Pointer(base + uintptr(i)*size))
		name, ok := goString(header.name.data, C.size_t(header.name.len))
		if !ok {
			return nil, false
		}
		value, ok := goString(header.value.data, C.size_t(header.value.len))
		if !ok {
			return nil, false
		}
		out = append(out, HTTPHeader{Name: name, Value: value})
	}
	return out, true
}
