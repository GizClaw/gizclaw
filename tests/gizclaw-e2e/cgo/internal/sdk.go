//go:build gizclaw_e2e

package internal

/*
#cgo CFLAGS: -I. -I../../../../sdk/c/gizclaw/include -I../../../../sdk/c/gizclaw/generated -I../../../../third_party/nanopb/upstream
#include "gzc_common.h"
#include "gzc_rpc_frame.h"
#include "sdk_client.h"
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	_ "github.com/GizClaw/gizclaw-go/sdk/c/gizclaw/cgobackend"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	session *C.gzc_cgo_session_t
}

type StreamFrame struct {
	Type int
	Data []byte
}

type TransportSendCounts struct {
	PacketDataChannel uint64
	OpusRTP           uint64
}

type TransportSnapshot struct {
	BackendHandle      uint64
	ActiveRPCChannelID int
	EventChannelID     int
	MediaReady         bool
	NextLocalChannelID int
	PacketChannelID    int
	PacketReady        bool
	RPCChannelIDs      []int
}

const (
	RPCFrameEOS        = int(C.GZC_RPC_FRAME_EOS)
	StatusChannelLimit = int(C.GZC_ERR_CHANNEL_LIMIT)
)

type StatusError struct {
	Operation string
	Code      int
	Message   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s rc=%d: %s", e.Operation, e.Code, e.Message)
}

// Registration is the typed result decoded by the C server.register helper.
type Registration struct {
	RuntimeProfileName string
}

// RPCError preserves a server RPC error returned through the C test bridge.
type RPCError struct {
	Method  rpcpb.RpcMethod
	Code    rpcpb.RpcErrorCode
	Message string
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("%s: RPC error %d: %s", e.Method, e.Code, e.Message)
}

type ServiceChannel struct {
	channel *C.gzc_service_channel_t
}

type EventStream struct {
	stream *C.gzc_event_stream_t
}

var errCSDKTimeout = errors.New("C SDK timeout")

func NewClient(identityDir string) (*Client, error) {
	cfg, err := readClientConfig(identityDir)
	if err != nil {
		return nil, err
	}
	return NewClientWithCredentials(cfg.endpoint, cfg.privateKey)
}

func NewClientWithCredentials(endpoint, privateKey string) (*Client, error) {
	cEndpoint := C.CString(endpoint)
	defer C.free(unsafe.Pointer(cEndpoint))
	cPrivateKey := C.CString(privateKey)
	defer C.free(unsafe.Pointer(cPrivateKey))
	errbuf := make([]byte, 1024)
	var session *C.gzc_cgo_session_t
	rc := C.gzc_cgo_session_open(
		cEndpoint,
		cPrivateKey,
		&session,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return nil, fmt.Errorf("open C SDK session rc=%d: %s", int(rc), cString(errbuf))
	}
	return &Client{session: session}, nil
}

func (c *Client) Close() {
	if c == nil || c.session == nil {
		return
	}
	C.gzc_cgo_session_close(c.session)
	c.session = nil
}

func (c *Client) CallRPC(method rpcpb.RpcMethod, request proto.Message, response proto.Message) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("closed C SDK client")
	}
	paramsPayload, err := proto.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal %s request payload: %w", method, err)
	}
	var cParams *C.uchar
	if len(paramsPayload) > 0 {
		cParams = (*C.uchar)(unsafe.Pointer(&paramsPayload[0]))
	}
	errbuf := make([]byte, 1024)
	var result *C.uchar
	var resultLen C.ulong
	var rpcErrorCode C.int
	rc := C.gzc_cgo_session_call_rpc_payload(
		c.session,
		C.uint(method),
		cParams,
		C.ulong(len(paramsPayload)),
		&result,
		&resultLen,
		&rpcErrorCode,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc == C.GZC_ERR_RPC && rpcErrorCode != 0 {
		return &RPCError{
			Method:  method,
			Code:    rpcpb.RpcErrorCode(rpcErrorCode),
			Message: cString(errbuf),
		}
	}
	if rc != C.GZC_OK {
		return fmt.Errorf("call %s rc=%d: %s", method, int(rc), cString(errbuf))
	}
	defer C.gzc_cgo_free(unsafe.Pointer(result))
	resultPayload := C.GoBytes(unsafe.Pointer(result), C.int(resultLen))
	if response != nil {
		if err := proto.Unmarshal(resultPayload, response); err != nil {
			return fmt.Errorf("decode %s response payload: %w", method, err)
		}
	}
	return nil
}

func (c *Client) CallStream(method rpcpb.RpcMethod, request proto.Message) ([]StreamFrame, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("closed C SDK client")
	}
	paramsPayload, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal %s stream request payload: %w", method, err)
	}
	var cParams *C.uchar
	if len(paramsPayload) > 0 {
		cParams = (*C.uchar)(unsafe.Pointer(&paramsPayload[0]))
	}
	errbuf := make([]byte, 1024)
	var frames *C.gzc_cgo_stream_frame_t
	var frameCount C.ulong
	rc := C.gzc_cgo_session_call_stream_collect(
		c.session,
		C.uint(method),
		cParams,
		C.ulong(len(paramsPayload)),
		&frames,
		&frameCount,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return nil, fmt.Errorf("stream %s rc=%d: %s", method, int(rc), cString(errbuf))
	}
	defer C.gzc_cgo_stream_frames_free(frames, frameCount)
	cFrames := unsafe.Slice(frames, int(frameCount))
	out := make([]StreamFrame, 0, len(cFrames))
	for _, frame := range cFrames {
		var data []byte
		if frame.len > 0 {
			data = C.GoBytes(unsafe.Pointer(frame.data), C.int(frame.len))
		}
		out = append(out, StreamFrame{Type: int(frame._type), Data: data})
	}
	if rpcErr := streamRPCError(method, out); rpcErr != nil {
		return nil, rpcErr
	}
	return out, nil
}

// Register encodes the request and decodes the response with C nanopb.
func (c *Client) Register(token string) (Registration, error) {
	if c == nil || c.session == nil {
		return Registration{}, fmt.Errorf("closed C SDK client")
	}
	cToken := C.CString(token)
	defer C.free(unsafe.Pointer(cToken))
	runtimeProfileName := make([]byte, 256)
	errbuf := make([]byte, 1024)
	var rpcErrorCode C.int
	rc := C.gzc_cgo_session_register(
		c.session,
		cToken,
		(*C.char)(unsafe.Pointer(&runtimeProfileName[0])),
		C.ulong(len(runtimeProfileName)),
		&rpcErrorCode,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc == C.GZC_ERR_RPC && rpcErrorCode != 0 {
		return Registration{}, &RPCError{
			Method:  rpcpb.RpcMethod_RPC_METHOD_SERVER_REGISTER,
			Code:    rpcpb.RpcErrorCode(rpcErrorCode),
			Message: cString(errbuf),
		}
	}
	if rc != C.GZC_OK {
		return Registration{}, fmt.Errorf("register C SDK client rc=%d: %s", int(rc), cString(errbuf))
	}
	return Registration{RuntimeProfileName: cString(runtimeProfileName)}, nil
}

// FirmwareConfig is a channel package configuration decoded by C nanopb.
type FirmwareConfig struct {
	Channel        rpcpb.FirmwareChannelName
	HasDescription bool
	Description    string
	URL            string
	SHA256         string
	Size           int64
}

// GetFirmware decodes the selected bound firmware package response with C nanopb.
func (c *Client) GetFirmware(channel rpcpb.FirmwareChannelName) (FirmwareConfig, error) {
	if c == nil || c.session == nil {
		return FirmwareConfig{}, fmt.Errorf("closed C SDK client")
	}
	description := make([]byte, 1025)
	url := make([]byte, 2049)
	sha256 := make([]byte, 65)
	errbuf := make([]byte, 1024)
	var size C.longlong
	var responseChannel C.int
	var hasDescription C.int
	var rpcErrorCode C.int
	rc := C.gzc_cgo_session_firmware_get(
		c.session,
		C.int(channel),
		&responseChannel,
		&hasDescription,
		(*C.char)(unsafe.Pointer(&description[0])),
		C.ulong(len(description)),
		(*C.char)(unsafe.Pointer(&url[0])),
		C.ulong(len(url)),
		(*C.char)(unsafe.Pointer(&sha256[0])),
		C.ulong(len(sha256)),
		&size,
		&rpcErrorCode,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc == C.GZC_ERR_RPC && rpcErrorCode != 0 {
		return FirmwareConfig{}, &RPCError{
			Method:  rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_GET,
			Code:    rpcpb.RpcErrorCode(rpcErrorCode),
			Message: cString(errbuf),
		}
	}
	if rc != C.GZC_OK {
		return FirmwareConfig{}, fmt.Errorf("get firmware with C SDK rc=%d: %s", int(rc), cString(errbuf))
	}
	return FirmwareConfig{
		Channel:        rpcpb.FirmwareChannelName(responseChannel),
		HasDescription: hasDescription != 0,
		Description:    cString(description),
		URL:            cString(url),
		SHA256:         cString(sha256),
		Size:           int64(size),
	}, nil
}

func streamRPCError(method rpcpb.RpcMethod, frames []StreamFrame) error {
	if len(frames) == 0 || frames[0].Type != int(C.GZC_RPC_FRAME_BINARY) {
		return nil
	}
	var envelope rpcpb.RpcResponse
	if err := proto.Unmarshal(frames[0].Data, &envelope); err != nil {
		return nil
	}
	rpcErr := envelope.GetError()
	if rpcErr == nil {
		return nil
	}
	return &RPCError{Method: method, Code: rpcErr.GetCode(), Message: rpcErr.GetMessage()}
}

func (c *Client) OpenServiceChannel(service uint64, timeout time.Duration) (*ServiceChannel, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("closed C SDK client")
	}
	errbuf := make([]byte, 1024)
	var channel *C.gzc_service_channel_t
	rc := C.gzc_cgo_session_open_service_channel(
		c.session,
		C.ulonglong(service),
		C.int(timeout.Milliseconds()),
		&channel,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return nil, &StatusError{
			Operation: "open service channel",
			Code:      int(rc),
			Message:   cString(errbuf),
		}
	}
	return &ServiceChannel{channel: channel}, nil
}

func (c *Client) SendPacket(protocol byte, payload []byte) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("closed C SDK client")
	}
	var ptr *C.uchar
	if len(payload) > 0 {
		ptr = (*C.uchar)(unsafe.Pointer(&payload[0]))
	}
	errbuf := make([]byte, 1024)
	rc := C.gzc_cgo_session_send_packet(
		c.session,
		C.uchar(protocol),
		ptr,
		C.ulong(len(payload)),
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return fmt.Errorf("send packet rc=%d: %s", int(rc), cString(errbuf))
	}
	return nil
}

func (c *Client) TransportSendCounts() (TransportSendCounts, error) {
	if c == nil || c.session == nil {
		return TransportSendCounts{}, fmt.Errorf("closed C SDK client")
	}
	var packetCalls C.ulonglong
	var opusCalls C.ulonglong
	rc := C.gzc_cgo_session_transport_send_counts(
		c.session,
		&packetCalls,
		&opusCalls,
	)
	if rc != C.GZC_OK {
		return TransportSendCounts{}, fmt.Errorf("read transport send counts rc=%d", int(rc))
	}
	return TransportSendCounts{
		PacketDataChannel: uint64(packetCalls),
		OpusRTP:           uint64(opusCalls),
	}, nil
}

func (c *Client) TransportSnapshot() (TransportSnapshot, error) {
	if c == nil || c.session == nil {
		return TransportSnapshot{}, fmt.Errorf("closed C SDK client")
	}
	var snapshot C.gzc_cgo_transport_snapshot_t
	rc := C.gzc_cgo_session_transport_snapshot(c.session, &snapshot)
	if rc != C.GZC_OK {
		return TransportSnapshot{}, fmt.Errorf("read transport snapshot rc=%d", int(rc))
	}
	count := int(snapshot.rpc_channel_count)
	ids := make([]int, count)
	for index := range count {
		ids[index] = int(snapshot.rpc_channel_ids[index])
	}
	return TransportSnapshot{
		BackendHandle:      uint64(snapshot.backend_handle),
		ActiveRPCChannelID: int(snapshot.active_rpc_channel_id),
		EventChannelID:     int(snapshot.event_channel_id),
		MediaReady:         snapshot.media_ready != 0,
		NextLocalChannelID: int(snapshot.next_local_channel_id),
		PacketChannelID:    int(snapshot.packet_channel_id),
		PacketReady:        snapshot.packet_ready != 0,
		RPCChannelIDs:      ids,
	}, nil
}

func (c *Client) SendBatteryTelemetry(percent float64, charging bool) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("closed C SDK client")
	}
	errbuf := make([]byte, 1024)
	chargingFlag := C.int(0)
	if charging {
		chargingFlag = 1
	}
	rc := C.gzc_cgo_session_send_battery_telemetry(
		c.session,
		C.double(percent),
		chargingFlag,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return fmt.Errorf("send battery telemetry rc=%d: %s", int(rc), cString(errbuf))
	}
	return nil
}

func (c *Client) SendFullTelemetry() error {
	if c == nil || c.session == nil {
		return fmt.Errorf("closed C SDK client")
	}
	errbuf := make([]byte, 1024)
	rc := C.gzc_cgo_session_send_full_telemetry(
		c.session,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return fmt.Errorf("send full telemetry rc=%d: %s", int(rc), cString(errbuf))
	}
	return nil
}

func (c *Client) ReadPacket(timeout time.Duration) (byte, []byte, error) {
	if c == nil || c.session == nil {
		return 0, nil, fmt.Errorf("closed C SDK client")
	}
	errbuf := make([]byte, 1024)
	var protocol C.uchar
	var payload *C.uchar
	var payloadLen C.ulong
	rc := C.gzc_cgo_session_read_packet(
		c.session,
		C.int(timeout.Milliseconds()),
		&protocol,
		&payload,
		&payloadLen,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc == C.GZC_ERR_TIMEOUT {
		return 0, nil, errCSDKTimeout
	}
	if rc != C.GZC_OK {
		return 0, nil, fmt.Errorf("read packet rc=%d: %s", int(rc), cString(errbuf))
	}
	defer C.gzc_cgo_free(unsafe.Pointer(payload))
	var data []byte
	if payloadLen > 0 {
		data = C.GoBytes(unsafe.Pointer(payload), C.int(payloadLen))
	}
	return byte(protocol), data, nil
}

func (c *Client) Poll(timeout time.Duration) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("closed C SDK client")
	}
	errbuf := make([]byte, 1024)
	rc := C.gzc_cgo_session_poll(
		c.session,
		C.int(timeout.Milliseconds()),
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return fmt.Errorf("poll rc=%d: %s", int(rc), cString(errbuf))
	}
	return nil
}

func (c *ServiceChannel) Close() {
	if c == nil || c.channel == nil {
		return
	}
	C.gzc_cgo_service_channel_close(c.channel)
	c.channel = nil
}

func (c *Client) OpenEventStream(timeout time.Duration) (*EventStream, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("closed C SDK client")
	}
	errbuf := make([]byte, 1024)
	var stream *C.gzc_event_stream_t
	rc := C.gzc_cgo_session_open_event_stream(
		c.session,
		C.int(timeout.Milliseconds()),
		&stream,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return nil, fmt.Errorf("open C SDK event stream rc=%d: %s", int(rc), cString(errbuf))
	}
	return &EventStream{stream: stream}, nil
}

func (s *EventStream) Close() {
	if s == nil || s.stream == nil {
		return
	}
	C.gzc_cgo_event_stream_close(s.stream)
	s.stream = nil
}

func (s *EventStream) SendAudioBoundary(streamID string, begin bool) error {
	if s == nil || s.stream == nil {
		return fmt.Errorf("closed C SDK event stream")
	}
	cStreamID := C.CString(streamID)
	defer C.free(unsafe.Pointer(cStreamID))
	errbuf := make([]byte, 1024)
	var cBegin C.int
	if begin {
		cBegin = 1
	}
	rc := C.gzc_cgo_event_stream_send_audio_boundary(
		s.stream,
		cStreamID,
		cBegin,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return fmt.Errorf("send C SDK event stream boundary rc=%d: %s", int(rc), cString(errbuf))
	}
	return nil
}

func (s *EventStream) ReadEvent(timeout time.Duration) (*eventpb.PeerEvent, error) {
	if s == nil || s.stream == nil {
		return nil, fmt.Errorf("closed C SDK event stream")
	}
	errbuf := make([]byte, 1024)
	var data *C.uchar
	var dataLen C.ulong
	rc := C.gzc_cgo_event_stream_read_encoded(
		s.stream,
		C.int(timeout.Milliseconds()),
		&data,
		&dataLen,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc == C.GZC_ERR_TIMEOUT {
		return nil, errCSDKTimeout
	}
	if rc != C.GZC_OK {
		return nil, fmt.Errorf("read C SDK event stream rc=%d: %s", int(rc), cString(errbuf))
	}
	defer C.gzc_cgo_free(unsafe.Pointer(data))
	event := &eventpb.PeerEvent{}
	if err := proto.Unmarshal(C.GoBytes(unsafe.Pointer(data), C.int(dataLen)), event); err != nil {
		return nil, fmt.Errorf("decode C SDK event stream payload: %w", err)
	}
	return event, nil
}

func (c *ServiceChannel) SendJSON(raw string) error {
	if c == nil || c.channel == nil {
		return fmt.Errorf("closed C SDK service channel")
	}
	cJSON := C.CString(raw)
	defer C.free(unsafe.Pointer(cJSON))
	errbuf := make([]byte, 1024)
	rc := C.gzc_cgo_service_channel_send_json(
		c.channel,
		cJSON,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return fmt.Errorf("send service json rc=%d: %s", int(rc), cString(errbuf))
	}
	return nil
}

func (c *ServiceChannel) SendFrame(frame StreamFrame) error {
	if c == nil || c.channel == nil {
		return fmt.Errorf("closed C SDK service channel")
	}
	var data *C.uchar
	if len(frame.Data) > 0 {
		data = (*C.uchar)(unsafe.Pointer(&frame.Data[0]))
	}
	errbuf := make([]byte, 1024)
	rc := C.gzc_cgo_service_channel_send_frame(
		c.channel,
		C.int(frame.Type),
		data,
		C.ulong(len(frame.Data)),
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return fmt.Errorf("send service frame rc=%d: %s", int(rc), cString(errbuf))
	}
	return nil
}

func (c *ServiceChannel) ReadFrame(timeout time.Duration) (StreamFrame, error) {
	if c == nil || c.channel == nil {
		return StreamFrame{}, fmt.Errorf("closed C SDK service channel")
	}
	errbuf := make([]byte, 1024)
	var frameType C.int
	var data *C.uchar
	var dataLen C.ulong
	rc := C.gzc_cgo_service_channel_read_frame(
		c.channel,
		C.int(timeout.Milliseconds()),
		&frameType,
		&data,
		&dataLen,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc == C.GZC_ERR_TIMEOUT {
		return StreamFrame{}, errCSDKTimeout
	}
	if rc != C.GZC_OK {
		return StreamFrame{}, fmt.Errorf("read service frame rc=%d: %s", int(rc), cString(errbuf))
	}
	defer C.gzc_cgo_free(unsafe.Pointer(data))
	var payload []byte
	if dataLen > 0 {
		payload = C.GoBytes(unsafe.Pointer(data), C.int(dataLen))
	}
	return StreamFrame{Type: int(frameType), Data: payload}, nil
}

func CSDKPing(t *testing.T, identityDir string) {
	t.Helper()
	client, err := NewClient(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var response rpcpb.PingResponse
	if err := client.CallRPC(rpcpb.RpcMethod_RPC_METHOD_ALL_PING, &rpcpb.PingRequest{ClientSendTime: 12345}, &response); err != nil {
		t.Fatal(err)
	}
	if response.GetServerTime() <= 0 {
		t.Fatalf("invalid server_time: %d", response.GetServerTime())
	}
}

func CSDKServerRuntime(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	var response rpcpb.ServerGetRuntimeResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_RUNTIME_GET, &rpcpb.ServerGetRuntimeRequest{}, &response)
	runtime := response.GetValue()
	if runtime == nil || !runtime.GetOnline() || runtime.GetLastSeenAt() == "" {
		t.Fatalf("invalid server.runtime.get: %s", response.String())
	}
}

func CSDKServerStatus(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()

	if err := client.SendFullTelemetry(); err != nil {
		t.Fatal(err)
	}
	var getResponse rpcpb.ServerGetStatusResponse
	deadline := time.Now().Add(5 * time.Second)
	for {
		mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_STATUS_GET, &rpcpb.ServerGetStatusRequest{}, &getResponse)
		status := getResponse.GetValue()
		if status != nil && status.BatteryPercent != nil && status.GetBatteryPercent() == 91 &&
			status.Charging != nil && status.GetCharging() {
			return
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server.status.get did not reflect telemetry: %s", getResponse.String())
}

func CSDKSpeedTest(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	frames, err := client.CallStream(rpcpb.RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN, &rpcpb.SpeedTestRequest{
		DownContentLength: 4096,
		UpContentLength:   0,
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawAck bool
	var binaryBytes int
	var sawEOS bool
	for _, frame := range frames {
		switch frame.Type {
		case int(C.GZC_RPC_FRAME_BINARY):
			if !sawAck {
				var response rpcpb.SpeedTestResponse
				decodeStreamResponse(t, rpcpb.RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN, frame.Data, &response)
				if response.GetDownContentLength() != 4096 || response.GetUpContentLength() != 0 {
					t.Fatalf("invalid speed test ack: %s", response.String())
				}
				sawAck = true
				continue
			}
			binaryBytes += len(frame.Data)
		case int(C.GZC_RPC_FRAME_EOS):
			if len(frame.Data) != 0 {
				t.Fatalf("speed test EOS has %d payload bytes", len(frame.Data))
			}
			sawEOS = true
		default:
			t.Fatalf("unexpected speed test frame type %d", frame.Type)
		}
	}
	if !sawAck || binaryBytes != 4096 || !sawEOS {
		t.Fatalf("invalid speed test stream: saw_ack=%v binary_bytes=%d saw_eos=%v", sawAck, binaryBytes, sawEOS)
	}
}

func CSDKFirmwareRPC(t *testing.T, identityDir, registrationToken string) {
	wants := []FirmwareConfig{
		{Channel: rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_STABLE, HasDescription: true, Description: "Devkit stable package", URL: "https://firmware.example.invalid/devkit/stable.tar.zlib", SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Size: 4096},
		{Channel: rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_BETA, HasDescription: true, Description: "Devkit beta package", URL: "https://firmware.example.invalid/devkit/beta.tar.zlib", SHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Size: 8192},
		{Channel: rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_DEVELOP, URL: "https://firmware.example.invalid/devkit/develop.tar.zlib", SHA256: "123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0", Size: 12288},
		{Channel: rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_PENDING, HasDescription: true, Description: "Devkit pending package", URL: "https://firmware.example.invalid/devkit/pending.tar.zlib", SHA256: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", Size: 16384},
	}
	CSDKFirmwareRPCPackages(t, identityDir, registrationToken, wants)
}

func CSDKFirmwareRPCPackage(t *testing.T, identityDir, registrationToken string, want FirmwareConfig) {
	CSDKFirmwareRPCPackages(t, identityDir, registrationToken, []FirmwareConfig{want})
}

func CSDKFirmwareRPCPackages(t *testing.T, identityDir, registrationToken string, wants []FirmwareConfig) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	registration := registerClient(t, client, registrationToken)
	_ = registration
	for _, want := range wants {
		firmware, err := client.GetFirmware(want.Channel)
		if err != nil {
			t.Fatal(err)
		}
		if firmware != want {
			t.Fatalf("server.firmware.get(%s) = %+v, want %+v", want.Channel, firmware, want)
		}
	}
}

func CSDKChatWorkspace(t *testing.T, identityDir, registrationToken string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	registerClient(t, client, registrationToken)
	workspaceName := fmt.Sprintf("cgo-direct-chatroom-%d", time.Now().UnixMilli())
	var createResponse rpcpb.WorkspaceCreateResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_CREATE, &rpcpb.WorkspaceCreateRequest{
		Value: &rpcpb.WorkspaceCreateBody{
			Name:         workspaceName,
			Collection:   "assistants",
			WorkflowName: "chatroom",
			Parameters: &rpcpb.WorkspaceParameters{Value: &rpcpb.WorkspaceParameters_ChatRoomWorkspaceParameters{
				ChatRoomWorkspaceParameters: &rpcpb.ChatRoomWorkspaceParameters{},
			}},
		},
	}, &createResponse)
	if createResponse.GetValue().GetName() != workspaceName {
		t.Fatalf("invalid server.workspace.create: %s", createResponse.String())
	}
	var workspaceResponse rpcpb.WorkspaceGetResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_GET, &rpcpb.WorkspaceGetRequest{Name: workspaceName}, &workspaceResponse)
	workspace := workspaceResponse.GetValue()
	if workspace == nil || workspace.GetName() != workspaceName || workspace.GetWorkflowName() != "chatroom" || !workspace.GetAvailable() {
		t.Fatalf("invalid server.workspace.get: %s", workspaceResponse.String())
	}
	setChatWorkspace(t, client, workspaceName)
	var getResponse rpcpb.ServerGetRunWorkspaceResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_GET, &rpcpb.ServerGetRunWorkspaceRequest{}, &getResponse)
	if getResponse.GetValue().GetWorkspaceName() != workspaceName ||
		getResponse.GetValue().GetRuntimeState() == rpcpb.PeerRunStatusState_PEER_RUN_STATUS_STATE_UNSPECIFIED {
		t.Fatalf("invalid server.run.workspace.get: %s", getResponse.String())
	}
	var statusResponse rpcpb.ServerGetRunStatusResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_STATUS, &rpcpb.ServerGetRunStatusRequest{}, &statusResponse)
	if statusResponse.GetValue().GetState() == rpcpb.PeerRunStatusState_PEER_RUN_STATUS_STATE_UNSPECIFIED {
		t.Fatalf("invalid server.run.status: %s", statusResponse.String())
	}
	var historyResponse rpcpb.ServerListRunWorkspaceHistoryResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY, &rpcpb.ServerListRunWorkspaceHistoryRequest{
		Value: &rpcpb.PeerRunHistoryListRequest{Limit: ptr(int64(5))},
	}, &historyResponse)
	if historyResponse.GetValue() == nil || !historyResponse.GetValue().GetAvailable() {
		t.Fatalf("invalid server.run.workspace.history: %s", historyResponse.String())
	}
}

func CSDKChatRoundtrip(t *testing.T, identityDir, registrationToken, workspaceName, oggPath string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	registerClient(t, client, registrationToken)
	setChatWorkspace(t, client, workspaceName)
	eventStream, err := client.OpenEventStream(15 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer eventStream.Close()
	runCSDKChatTurn(t, client, eventStream, "cgo-chat", oggPath)
}

func CSDKPeerStreamWorkspaceReloadContinuity(
	t *testing.T,
	identityDir string,
	registrationToken string,
	firstWorkspace string,
	alternateWorkspace string,
	oggPath string,
) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	registerClient(t, client, registrationToken)
	var original rpcpb.ServerGetRunWorkspaceResponse
	mustCallRPC(
		t,
		client,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_GET,
		&rpcpb.ServerGetRunWorkspaceRequest{},
		&original,
	)
	originalWorkspace := original.GetValue().GetWorkspaceName()
	restored := false
	defer func() {
		if !restored {
			if err := restoreCSDKRunWorkspace(client, originalWorkspace); err != nil {
				t.Errorf("restore original C SDK Workspace after failure: %v", err)
			}
		}
	}()
	eventStream, err := client.OpenEventStream(15 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer eventStream.Close()
	baseline, err := client.TransportSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	requireCSDKMandatoryTransports(t, baseline)
	requireCSDKDirectPacketTelemetry(t, client)
	requireStableCSDKMandatoryTransports(
		t,
		baseline,
		mustCSDKTransportSnapshot(t, client),
	)

	steps := []struct {
		workspace string
		set       bool
	}{
		{workspace: firstWorkspace, set: true},
		{workspace: firstWorkspace},
		{workspace: alternateWorkspace, set: true},
	}
	for index, step := range steps {
		if step.set {
			setChatWorkspace(t, client, step.workspace)
		} else {
			reloadCSDKRunWorkspace(t, client, step.workspace)
		}
		runCSDKChatTurn(
			t,
			client,
			eventStream,
			fmt.Sprintf("cgo-stream-lifecycle-%d", index+1),
			oggPath,
		)
		after, err := client.TransportSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		requireStableCSDKMandatoryTransports(t, baseline, after)
	}
	restoreErr := restoreCSDKRunWorkspace(client, originalWorkspace)
	if restoreErr != nil {
		t.Fatalf("restore original C SDK Workspace: %v", restoreErr)
	}
	restored = true
}

func runCSDKChatTurn(
	t *testing.T,
	client *Client,
	eventStream *EventStream,
	streamID string,
	oggPath string,
) {
	t.Helper()
	before, err := client.TransportSendCounts()
	if err != nil {
		t.Fatal(err)
	}
	if err := eventStream.SendAudioBoundary(streamID, true); err != nil {
		t.Fatalf("send chat BOS: %v", err)
	}
	for _, packet := range opusPacketsFromOgg(t, oggPath) {
		if err := client.SendPacket(0x10, packet); err != nil {
			t.Fatalf("send chat opus packet: %v", err)
		}
		if err := client.Poll(20 * time.Millisecond); err != nil {
			t.Fatalf("pace chat opus packet: %v", err)
		}
	}
	if err := eventStream.SendAudioBoundary(streamID, false); err != nil {
		t.Fatalf("send chat EOS: %v", err)
	}
	deadline := time.Now().Add(90 * time.Second)
	var sawText bool
	var sawEventEOS bool
	var eventFrames int
	var downlinkPackets int
	for time.Now().Before(deadline) {
		event, err := eventStream.ReadEvent(50 * time.Millisecond)
		if err == nil {
			eventFrames++
			if (event.GetType() == eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA ||
				event.GetType() == eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE) &&
				event.Text() != "" {
				sawText = true
			}
			if event.GetType() == eventpb.PeerEventType_PEER_EVENT_TYPE_EOS {
				sawEventEOS = true
			}
		} else if !errors.Is(err, errCSDKTimeout) {
			t.Fatalf("read chat event frame: %v", err)
		}
		protocol, payload, err := client.ReadPacket(50 * time.Millisecond)
		if err == nil {
			if protocol == 0x10 && len(payload) > 0 {
				downlinkPackets++
			}
		} else if !errors.Is(err, errCSDKTimeout) {
			t.Fatalf("read chat packet: %v", err)
		}
		if sawText && downlinkPackets > 0 && sawEventEOS {
			after, err := client.TransportSendCounts()
			if err != nil {
				t.Fatal(err)
			}
			if after.OpusRTP <= before.OpusRTP {
				t.Fatalf("C SDK Opus send did not use RTP: before=%+v after=%+v", before, after)
			}
			if after.PacketDataChannel != before.PacketDataChannel {
				t.Fatalf("protocol 0x10 leaked into packet DataChannel: before=%+v after=%+v", before, after)
			}
			return
		}
	}
	if !sawText || downlinkPackets == 0 || !sawEventEOS {
		t.Fatalf("chat roundtrip missing text or audio: events=%d saw_text=%v saw_eos=%v downlink_packets=%d", eventFrames, sawText, sawEventEOS, downlinkPackets)
	}
}

func reloadCSDKRunWorkspace(t *testing.T, client *Client, workspaceName string) {
	t.Helper()
	var response rpcpb.ServerReloadRunWorkspaceResponse
	mustCallRPC(
		t,
		client,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD,
		&rpcpb.ServerReloadRunWorkspaceRequest{},
		&response,
	)
	if response.GetValue().GetWorkspaceName() != workspaceName {
		t.Fatalf("invalid server.run.workspace.reload: %s", response.String())
	}
}

func restoreCSDKRunWorkspace(client *Client, workspaceName string) error {
	if strings.TrimSpace(workspaceName) != "" {
		return setCSDKRunWorkspace(client, workspaceName)
	}
	var response rpcpb.ServerStopRunResponse
	if err := client.CallRPC(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_STOP,
		&rpcpb.ServerStopRunRequest{},
		&response,
	); err != nil {
		return err
	}
	return nil
}

func requireCSDKDirectPacketTelemetry(t *testing.T, client *Client) {
	t.Helper()
	const batteryPercent = 67
	if err := client.SendBatteryTelemetry(batteryPercent, true); err != nil {
		t.Fatalf("send C SDK Direct Packet telemetry: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var response rpcpb.ServerGetStatusResponse
		if err := client.CallRPC(
			rpcpb.RpcMethod_RPC_METHOD_SERVER_STATUS_GET,
			&rpcpb.ServerGetStatusRequest{},
			&response,
		); err != nil {
			t.Fatalf("read C SDK telemetry through server.status.get: %v", err)
		}
		status := response.GetValue()
		if status != nil &&
			status.BatteryPercent != nil &&
			status.GetBatteryPercent() == batteryPercent &&
			status.Charging != nil &&
			status.GetCharging() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"server.status.get did not reflect C SDK Direct Packet telemetry: %s",
				response.String(),
			)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func mustCSDKTransportSnapshot(t *testing.T, client *Client) TransportSnapshot {
	t.Helper()
	snapshot, err := client.TransportSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func requireCSDKMandatoryTransports(t *testing.T, snapshot TransportSnapshot) {
	t.Helper()
	if snapshot.BackendHandle == 0 ||
		!snapshot.PacketReady ||
		snapshot.EventChannelID == 0 ||
		!snapshot.MediaReady {
		t.Fatalf("mandatory C transports are not ready: %+v", snapshot)
	}
}

func requireStableCSDKMandatoryTransports(
	t *testing.T,
	want TransportSnapshot,
	got TransportSnapshot,
) {
	t.Helper()
	if got.BackendHandle != want.BackendHandle ||
		got.PacketChannelID != want.PacketChannelID ||
		got.PacketReady != want.PacketReady ||
		got.EventChannelID != want.EventChannelID ||
		got.MediaReady != want.MediaReady {
		t.Fatalf(
			"mandatory C transport identities changed: before=%+v after=%+v",
			want,
			got,
		)
	}
}

func CSDKSocialBasic(t *testing.T, identityDir, registrationToken string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	requireDefaultGameplayRegistration(t, client, registrationToken)
	unique := time.Now().UnixMilli()
	contactName := fmt.Sprintf("C SDK Social Contact %d", unique)
	contactPhone := fmt.Sprintf("+1555%010d", unique%10000000000)
	groupName := fmt.Sprintf("c-sdk-social-group-%d", unique)

	var contactCreate rpcpb.ContactCreateResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_CREATE, &rpcpb.ContactCreateRequest{
		Name:        contactName,
		DisplayName: ptr(contactName),
		PhoneNumber: ptr(contactPhone),
	}, &contactCreate)
	if contactCreate.GetValue().GetName() != contactName || contactCreate.GetValue().GetDisplayName() != contactName {
		t.Fatalf("invalid server.contact.create: %s", contactCreate.String())
	}
	contactID := contactCreate.GetValue().GetName()
	defer cleanupCSDKRPC(
		t,
		client,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_DELETE,
		&rpcpb.ContactDeleteRequest{Name: contactID},
		&rpcpb.ContactDeleteResponse{},
		"Contact",
	)
	var contactGet rpcpb.ContactGetResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_GET, &rpcpb.ContactGetRequest{Name: contactID}, &contactGet)
	if contactGet.GetValue().GetName() != contactID {
		t.Fatalf("invalid server.contact.get: %s", contactGet.String())
	}
	var contactList rpcpb.ContactListResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_LIST, &rpcpb.ContactListRequest{Limit: ptr(int64(1000))}, &contactList)
	if !contactListContains(contactList.GetItems(), contactID) {
		t.Fatalf("invalid server.contact.list: %s", contactList.String())
	}

	var groupCreate rpcpb.FriendGroupCreateResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_CREATE, &rpcpb.FriendGroupCreateRequest{
		Name:        groupName,
		Description: ptr("created by cgo C SDK test"),
	}, &groupCreate)
	if groupCreate.GetValue().GetName() != groupName || groupCreate.GetValue().GetWorkspaceName() == "" {
		t.Fatalf("invalid server.friend_group.create: %s", groupCreate.String())
	}
	groupID := groupCreate.GetValue().GetName()
	defer cleanupCSDKRPC(
		t,
		client,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_DELETE,
		&rpcpb.FriendGroupDeleteRequest{Name: groupID},
		&rpcpb.FriendGroupDeleteResponse{},
		"Friend Group",
	)
	var groupGet rpcpb.FriendGroupGetResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_GET, &rpcpb.FriendGroupGetRequest{Name: groupID}, &groupGet)
	if groupGet.GetValue().GetName() != groupID {
		t.Fatalf("invalid server.friend_group.get: %s", groupGet.String())
	}
	var groupList rpcpb.FriendGroupListResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_LIST, &rpcpb.FriendGroupListRequest{Limit: ptr(int64(1000))}, &groupList)
	if !friendGroupListContains(groupList.GetItems(), groupID) {
		t.Fatalf("invalid server.friend_group.list: %s", groupList.String())
	}
	var tokenResponse rpcpb.FriendGroupInviteTokenCreateResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE, &rpcpb.FriendGroupInviteTokenCreateRequest{FriendGroupName: groupID}, &tokenResponse)
	if tokenResponse.GetInviteToken() == "" || tokenResponse.GetExpiresAt() == "" {
		t.Fatalf("invalid server.friend_group.invite_token.create: %s", tokenResponse.String())
	}
	defer cleanupCSDKRPC(
		t,
		client,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CLEAR,
		&rpcpb.FriendGroupInviteTokenClearRequest{FriendGroupName: groupID},
		&rpcpb.FriendGroupInviteTokenClearResponse{},
		"Friend Group invite token",
	)
	var messageList rpcpb.FriendGroupMessageListResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST, &rpcpb.FriendGroupMessageListRequest{
		FriendGroupName: groupID,
		Limit:           ptr(int64(1000)),
	}, &messageList)
	for _, message := range messageList.GetItems() {
		if message.GetFriendGroupName() != groupID || message.GetName() == "" {
			t.Fatalf("invalid server.friend_group.messages.list: %s", messageList.String())
		}
	}
}

func CSDKSocialRelationships(
	t *testing.T,
	identityADir,
	identityBDir,
	registrationToken string,
) {
	t.Helper()
	peerAPublicKey := identityPublicKey(t, identityADir)
	peerBPublicKey := identityPublicKey(t, identityBDir)
	clientA := newTestClient(t, identityADir)
	defer clientA.Close()
	clientB := newTestClient(t, identityBDir)
	defer clientB.Close()
	requireDefaultGameplayRegistration(t, clientA, registrationToken)
	requireDefaultGameplayRegistration(t, clientB, registrationToken)

	var friendToken rpcpb.FriendInviteTokenCreateResponse
	mustCallRPC(t, clientB, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE, &rpcpb.FriendInviteTokenCreateRequest{}, &friendToken)
	if friendToken.GetInviteToken() == "" || friendToken.GetExpiresAt() == "" {
		t.Fatalf("invalid server.friend.invite_token.create: %s", friendToken.String())
	}
	defer cleanupCSDKRPC(
		t,
		clientB,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CLEAR,
		&rpcpb.FriendInviteTokenClearRequest{},
		&rpcpb.FriendInviteTokenClearResponse{},
		"Friend invite token",
	)
	var friendAdd rpcpb.FriendAddResponse
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_ADD, &rpcpb.FriendAddRequest{InviteToken: friendToken.GetInviteToken()}, &friendAdd)
	if friendAdd.GetValue().GetName() == "" || friendAdd.GetValue().GetWorkspaceName() == "" || friendAdd.GetValue().GetPeerPublicKey() != peerBPublicKey {
		t.Fatalf("invalid server.friend.add: %s", friendAdd.String())
	}
	friendName := friendAdd.GetValue().GetName()
	friendWorkspace := friendAdd.GetValue().GetWorkspaceName()
	defer cleanupCSDKRPC(
		t,
		clientA,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_DELETE,
		&rpcpb.FriendDeleteRequest{Name: friendName},
		&rpcpb.FriendDeleteResponse{},
		"Friend relationship",
	)
	var repeatedFriendAdd rpcpb.FriendAddResponse
	mustCallRPC(
		t,
		clientA,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_ADD,
		&rpcpb.FriendAddRequest{InviteToken: friendToken.GetInviteToken()},
		&repeatedFriendAdd,
	)
	if repeatedFriendAdd.GetValue().GetName() != friendName ||
		repeatedFriendAdd.GetValue().GetWorkspaceName() != friendWorkspace ||
		repeatedFriendAdd.GetValue().GetPeerPublicKey() != peerBPublicKey {
		t.Fatalf("repeated server.friend.add was not idempotent: %s", repeatedFriendAdd.String())
	}
	for name, peer := range map[string]struct {
		client                *Client
		expectedPeerPublicKey string
	}{
		"Peer A": {client: clientA, expectedPeerPublicKey: peerBPublicKey},
		"Peer B": {client: clientB, expectedPeerPublicKey: peerAPublicKey},
	} {
		var friends rpcpb.FriendListResponse
		mustCallRPC(
			t,
			peer.client,
			rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_LIST,
			&rpcpb.FriendListRequest{Limit: ptr(int64(1000))},
			&friends,
		)
		if !friendListContains(friends.GetItems(), peer.expectedPeerPublicKey, friendWorkspace) {
			t.Fatalf("%s did not list the shared Friend relationship: %s", name, friends.String())
		}
	}

	var groupCreate rpcpb.FriendGroupCreateResponse
	groupName := fmt.Sprintf("c-sdk-cross-user-group-%d", time.Now().UnixNano())
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_CREATE, &rpcpb.FriendGroupCreateRequest{
		Name:        groupName,
		Description: ptr("created by cgo C SDK relationship test"),
	}, &groupCreate)
	if groupCreate.GetValue().GetName() == "" || groupCreate.GetValue().GetWorkspaceName() == "" {
		t.Fatalf("invalid server.friend_group.create: %s", groupCreate.String())
	}
	groupID := groupCreate.GetValue().GetName()
	defer cleanupCSDKRPC(
		t,
		clientA,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_DELETE,
		&rpcpb.FriendGroupDeleteRequest{Name: groupID},
		&rpcpb.FriendGroupDeleteResponse{},
		"Friend Group",
	)
	var groupToken rpcpb.FriendGroupInviteTokenCreateResponse
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE, &rpcpb.FriendGroupInviteTokenCreateRequest{FriendGroupName: groupID}, &groupToken)
	if groupToken.GetInviteToken() == "" || groupToken.GetExpiresAt() == "" {
		t.Fatalf("invalid server.friend_group.invite_token.create: %s", groupToken.String())
	}
	defer cleanupCSDKRPC(
		t,
		clientA,
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CLEAR,
		&rpcpb.FriendGroupInviteTokenClearRequest{FriendGroupName: groupID},
		&rpcpb.FriendGroupInviteTokenClearResponse{},
		"Friend Group invite token",
	)
	var groupJoin rpcpb.FriendGroupJoinResponse
	mustCallRPC(t, clientB, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_JOIN, &rpcpb.FriendGroupJoinRequest{Name: groupID, InviteToken: groupToken.GetInviteToken()}, &groupJoin)
	if groupJoin.GetGroup().GetName() != groupID {
		t.Fatalf("invalid server.friend_group.join: %s", groupJoin.String())
	}
	var memberList rpcpb.FriendGroupMemberListResponse
	mustCallRPC(t, clientB, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST, &rpcpb.FriendGroupMemberListRequest{
		FriendGroupName: ptr(groupID),
		Limit:           ptr(int64(1000)),
	}, &memberList)
	if !friendGroupMemberListContains(memberList.GetItems(), groupID) {
		t.Fatalf("invalid server.friend_group.members.list: %s", memberList.String())
	}
	var messageList rpcpb.FriendGroupMessageListResponse
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST, &rpcpb.FriendGroupMessageListRequest{
		FriendGroupName: groupID,
		Limit:           ptr(int64(1000)),
	}, &messageList)
	for _, message := range messageList.GetItems() {
		if message.GetFriendGroupName() != groupID || message.GetName() == "" {
			t.Fatalf("invalid server.friend_group.messages.list: %s", messageList.String())
		}
	}
}

func newTestClient(t *testing.T, identityDir string) *Client {
	t.Helper()
	client, err := NewClient(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func identityPublicKey(t *testing.T, identityDir string) string {
	t.Helper()
	cfg, err := readClientConfig(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	var privateKey giznet.Key
	if err := privateKey.UnmarshalText([]byte(cfg.privateKey)); err != nil {
		t.Fatalf("parse C SDK identity private key: %v", err)
	}
	keyPair, err := giznet.NewKeyPair(privateKey)
	if err != nil {
		t.Fatalf("derive C SDK identity public key: %v", err)
	}
	return keyPair.Public.String()
}

func registerClient(t *testing.T, client *Client, registrationToken string) Registration {
	t.Helper()
	response, err := client.Register(registrationToken)
	if err != nil {
		t.Fatal(err)
	}
	if response.RuntimeProfileName == "" {
		t.Fatalf("invalid server.register response: %+v", response)
	}
	return response
}

func requireDefaultGameplayRegistration(
	t *testing.T,
	client *Client,
	registrationToken string,
) {
	t.Helper()
	registered := registerClient(t, client, registrationToken)
	if registered.RuntimeProfileName != "default-gameplay" {
		t.Fatalf(
			"registered C RuntimeProfile = %q, want default-gameplay",
			registered.RuntimeProfileName,
		)
	}
}

func cleanupCSDKRPC(
	t *testing.T,
	client *Client,
	method rpcpb.RpcMethod,
	request proto.Message,
	response proto.Message,
	resource string,
) {
	t.Helper()
	if err := client.CallRPC(method, request, response); err != nil {
		t.Errorf("cleanup C SDK %s: %v", resource, err)
	}
}

func mustCallRPC(t *testing.T, client *Client, method rpcpb.RpcMethod, request proto.Message, response proto.Message) {
	t.Helper()
	if err := client.CallRPC(method, request, response); err != nil {
		t.Fatal(err)
	}
}

func decodeStreamResponse(t *testing.T, method rpcpb.RpcMethod, frame []byte, response proto.Message) {
	t.Helper()
	var envelope rpcpb.RpcResponse
	if err := proto.Unmarshal(frame, &envelope); err != nil {
		t.Fatalf("decode %s protobuf stream envelope: %v", method, err)
	}
	if rpcErr := envelope.GetError(); rpcErr != nil {
		t.Fatalf("%s stream error: %d %s", method, rpcErr.GetCode(), rpcErr.GetMessage())
	}
	resultPayload := envelope.GetPayload()
	if resultPayload == nil {
		t.Fatalf("%s stream envelope has empty result", method)
	}
	if err := proto.Unmarshal(resultPayload, response); err != nil {
		t.Fatalf("decode %s stream response payload: %v", method, err)
	}
}

func setChatWorkspace(t *testing.T, client *Client, workspaceName string) {
	t.Helper()
	if err := setCSDKRunWorkspace(client, workspaceName); err != nil {
		t.Fatal(err)
	}
}

func setCSDKRunWorkspace(client *Client, workspaceName string) error {
	var setResponse rpcpb.ServerSetRunWorkspaceResponse
	if err := client.CallRPC(rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_SET, &rpcpb.ServerSetRunWorkspaceRequest{
		Value: &rpcpb.AgentSelection{WorkspaceName: workspaceName},
	}, &setResponse); err != nil {
		return err
	}
	if setResponse.GetValue().GetWorkspaceName() != workspaceName {
		return fmt.Errorf("invalid server.run.workspace.set: %s", setResponse.String())
	}
	var reloadResponse rpcpb.ServerReloadRunWorkspaceResponse
	if err := client.CallRPC(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD,
		&rpcpb.ServerReloadRunWorkspaceRequest{},
		&reloadResponse,
	); err != nil {
		return err
	}
	if reloadResponse.GetValue().GetWorkspaceName() != workspaceName {
		return fmt.Errorf("invalid server.run.workspace.reload: %s", reloadResponse.String())
	}
	return nil
}

func ptr[T any](value T) *T {
	return &value
}

func contactListContains(items []*rpcpb.ContactObject, id string) bool {
	for _, item := range items {
		if item.GetName() == id {
			return true
		}
	}
	return false
}

func friendListContains(items []*rpcpb.FriendObject, peerPublicKey, workspaceName string) bool {
	for _, item := range items {
		if item.GetName() != "" &&
			item.GetPeerPublicKey() == peerPublicKey &&
			item.GetWorkspaceName() == workspaceName {
			return true
		}
	}
	return false
}

func friendGroupListContains(items []*rpcpb.FriendGroupObject, id string) bool {
	for _, item := range items {
		if item.GetName() == id {
			return true
		}
	}
	return false
}

func friendGroupMemberListContains(items []*rpcpb.FriendGroupMemberObject, groupID string) bool {
	for _, item := range items {
		if item.GetFriendGroupName() == groupID {
			return true
		}
	}
	return false
}

func opusPacketsFromOgg(t *testing.T, path string) [][]byte {
	t.Helper()
	audio, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read opus fixture: %v", err)
	}
	var packets [][]byte
	for packet, err := range ogg.Packets(bytes.NewReader(audio)) {
		if err != nil {
			t.Fatalf("read ogg opus packets: %v", err)
		}
		if len(packet.Data) == 0 || bytes.HasPrefix(packet.Data, []byte("OpusHead")) || bytes.HasPrefix(packet.Data, []byte("OpusTags")) {
			continue
		}
		packets = append(packets, append([]byte(nil), packet.Data...))
	}
	if len(packets) == 0 {
		t.Fatal("opus fixture has no opus payload packets")
	}
	return packets
}

func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return fmt.Sprintf("%q", string(buf))
}

type clientConfig struct {
	endpoint   string
	privateKey string
}

func readClientConfig(identityDir string) (clientConfig, error) {
	data, err := os.ReadFile(filepath.Join(identityDir, "config.yaml"))
	if err != nil {
		return clientConfig{}, err
	}
	config := string(data)
	endpoint := matchConfigValue(config, `endpoint:\s*"?([^"\s]+)"?`)
	privateKey := matchConfigValue(config, `private-key:\s*"?([^"\s]+)"?`)
	if endpoint == "" || privateKey == "" {
		return clientConfig{}, fmt.Errorf("incomplete C SDK identity config %s", filepath.Join(identityDir, "config.yaml"))
	}
	return clientConfig{
		endpoint:   endpoint,
		privateKey: privateKey,
	}, nil
}

func matchConfigValue(config, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(config)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
