//go:build gizclaw_e2e

package internal

/*
#cgo CFLAGS: -I. -I../../../../c/gizclaw/include -I../../../../c/gizclaw/generated
#include "gzc_common.h"
#include "gzc_rpc_frame.h"
#include "sdk_client.h"
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unsafe"

	_ "github.com/GizClaw/gizclaw-go/c/gizclaw/cgobackend"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/stampedopus"
)

type Client struct {
	session *C.gzc_cgo_session_t
}

type StreamFrame struct {
	Type int
	Data []byte
}

type ServiceChannel struct {
	channel *C.gzc_service_channel_t
}

var errCSDKTimeout = errors.New("C SDK timeout")

func NewClient(identityDir string) (*Client, error) {
	cfg, err := readClientConfig(identityDir)
	if err != nil {
		return nil, err
	}
	cEndpoint := C.CString(cfg.endpoint)
	defer C.free(unsafe.Pointer(cEndpoint))
	cPrivateKey := C.CString(cfg.privateKey)
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

func (c *Client) CallJSON(method string, params json.RawMessage) (json.RawMessage, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("closed C SDK client")
	}
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	cParams := C.CString(string(params))
	defer C.free(unsafe.Pointer(cParams))
	errbuf := make([]byte, 1024)
	var result *C.char
	var resultLen C.ulong
	rc := C.gzc_cgo_session_call_json(
		c.session,
		cMethod,
		cParams,
		&result,
		&resultLen,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return nil, fmt.Errorf("call %s rc=%d: %s", method, int(rc), cString(errbuf))
	}
	defer C.gzc_cgo_free(unsafe.Pointer(result))
	return append([]byte(nil), C.GoBytes(unsafe.Pointer(result), C.int(resultLen))...), nil
}

func (c *Client) CallStream(method string, params json.RawMessage) ([]StreamFrame, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("closed C SDK client")
	}
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	cParams := C.CString(string(params))
	defer C.free(unsafe.Pointer(cParams))
	errbuf := make([]byte, 1024)
	var frames *C.gzc_cgo_stream_frame_t
	var frameCount C.ulong
	rc := C.gzc_cgo_session_call_stream_collect(
		c.session,
		cMethod,
		cParams,
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
	return out, nil
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
		return nil, fmt.Errorf("open service channel rc=%d: %s", int(rc), cString(errbuf))
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
	result, err := client.CallJSON("all.ping", json.RawMessage(`{"client_send_time":12345}`))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		ServerTime int64 `json:"server_time"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatalf("decode ping result: %v: %s", err, string(result))
	}
	if response.ServerTime <= 0 {
		t.Fatalf("invalid server_time: %d", response.ServerTime)
	}
}

func CSDKServerRuntime(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	result := mustCallJSON(t, client, "server.runtime.get", `{}`)
	var response struct {
		Online     bool   `json:"online"`
		LastSeenAt string `json:"last_seen_at"`
	}
	decodeJSON(t, "server.runtime.get", result, &response)
	if !response.Online || response.LastSeenAt == "" {
		t.Fatalf("invalid server.runtime.get: %s", string(result))
	}
}

func CSDKServerStatus(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()

	if err := client.SendFullTelemetry(); err != nil {
		t.Fatal(err)
	}
	var result json.RawMessage
	var getResponse struct {
		BatteryPercent *int  `json:"battery_percent"`
		Charging       *bool `json:"charging"`
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		result = mustCallJSON(t, client, "server.status.get", `{}`)
		decodeJSON(t, "server.status.get", result, &getResponse)
		if getResponse.BatteryPercent != nil && *getResponse.BatteryPercent == 91 &&
			getResponse.Charging != nil && *getResponse.Charging {
			return
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server.status.get did not reflect telemetry: %s", string(result))
}

func CSDKSpeedTest(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	frames, err := client.CallStream("all.speed_test.run", json.RawMessage(`{"down_content_length":4096,"up_content_length":0}`))
	if err != nil {
		t.Fatal(err)
	}
	var sawAck bool
	var binaryBytes int
	for _, frame := range frames {
		switch frame.Type {
		case int(C.GZC_RPC_FRAME_JSON):
			result := streamResultJSON(t, "all.speed_test.run", frame.Data)
			var response struct {
				DownContentLength int `json:"down_content_length"`
				UpContentLength   int `json:"up_content_length"`
			}
			decodeJSON(t, "all.speed_test.run", result, &response)
			if response.DownContentLength != 4096 || response.UpContentLength != 0 {
				t.Fatalf("invalid speed test ack: %s", string(result))
			}
			sawAck = true
		case int(C.GZC_RPC_FRAME_BINARY):
			binaryBytes += len(frame.Data)
		default:
			t.Fatalf("unexpected speed test frame type %d", frame.Type)
		}
	}
	if !sawAck || binaryBytes != 4096 {
		t.Fatalf("invalid speed test stream: saw_ack=%v binary_bytes=%d", sawAck, binaryBytes)
	}
}

func CSDKFirmwareJSON(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	result := mustCallJSON(t, client, "server.firmware.list", `{"limit":5}`)
	var listResponse struct {
		Items []json.RawMessage `json:"items"`
	}
	decodeJSON(t, "server.firmware.list", result, &listResponse)
	if len(listResponse.Items) == 0 {
		t.Fatalf("empty server.firmware.list: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.firmware.get", `{"firmware_id":"devkit-firmware-main"}`)
	var getResponse struct {
		Name  string          `json:"name"`
		Slots json.RawMessage `json:"slots"`
	}
	decodeJSON(t, "server.firmware.get", result, &getResponse)
	if getResponse.Name != "devkit-firmware-main" || len(getResponse.Slots) == 0 {
		t.Fatalf("invalid server.firmware.get: %s", string(result))
	}
}

func CSDKFirmwareDownload(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	frames, err := client.CallStream("server.firmware.files.download", json.RawMessage(`{"firmware_id":"devkit-firmware-main","channel":"stable","path":"firmware/main.bin"}`))
	if err != nil {
		t.Fatal(err)
	}
	var sawMetadata bool
	var binaryBytes int
	var sawMarker bool
	for _, frame := range frames {
		switch frame.Type {
		case int(C.GZC_RPC_FRAME_JSON):
			result := streamResultJSON(t, "server.firmware.files.download", frame.Data)
			var response struct {
				FirmwareID string `json:"firmware_id"`
				Path       string `json:"path"`
			}
			decodeJSON(t, "server.firmware.files.download", result, &response)
			if response.FirmwareID != "devkit-firmware-main" || response.Path != "firmware/main.bin" {
				t.Fatalf("invalid firmware download metadata: %s", string(result))
			}
			sawMetadata = true
		case int(C.GZC_RPC_FRAME_BINARY):
			binaryBytes += len(frame.Data)
			if bytes.Contains(frame.Data, []byte("GIZCLAW_MAIN_FIRMWARE_V1")) {
				sawMarker = true
			}
		default:
			t.Fatalf("unexpected firmware download frame type %d", frame.Type)
		}
	}
	if !sawMetadata || binaryBytes == 0 || !sawMarker {
		t.Fatalf("invalid firmware download stream: saw_metadata=%v binary_bytes=%d saw_marker=%v", sawMetadata, binaryBytes, sawMarker)
	}
}

func CSDKChatWorkspace(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	result := mustCallJSON(t, client, "server.workspace.get", `{"name":"direct-chatroom-workspace"}`)
	var workspace struct {
		Name         string `json:"name"`
		WorkflowName string `json:"workflow_name"`
	}
	decodeJSON(t, "server.workspace.get", result, &workspace)
	if workspace.Name != "direct-chatroom-workspace" || workspace.WorkflowName != "chatroom-direct" {
		t.Fatalf("invalid server.workspace.get: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.run.workspace.set", `{"workspace_name":"direct-chatroom-workspace"}`)
	var setResponse struct {
		WorkspaceName string `json:"workspace_name"`
	}
	decodeJSON(t, "server.run.workspace.set", result, &setResponse)
	if setResponse.WorkspaceName != "direct-chatroom-workspace" {
		t.Fatalf("invalid server.run.workspace.set: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.run.workspace.get", `{}`)
	var getResponse struct {
		WorkspaceName string          `json:"workspace_name"`
		RuntimeState  json.RawMessage `json:"runtime_state"`
	}
	decodeJSON(t, "server.run.workspace.get", result, &getResponse)
	if getResponse.WorkspaceName != "direct-chatroom-workspace" || len(getResponse.RuntimeState) == 0 {
		t.Fatalf("invalid server.run.workspace.get: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.run.status", `{}`)
	var statusResponse struct {
		State json.RawMessage `json:"state"`
	}
	decodeJSON(t, "server.run.status", result, &statusResponse)
	if len(statusResponse.State) == 0 {
		t.Fatalf("invalid server.run.status: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.run.workspace.history", `{"limit":5}`)
	var historyResponse struct {
		Items []json.RawMessage `json:"items"`
	}
	decodeJSON(t, "server.run.workspace.history", result, &historyResponse)
	if historyResponse.Items == nil {
		t.Fatalf("invalid server.run.workspace.history: %s", string(result))
	}
}

func CSDKChatRoundtrip(t *testing.T, identityDir, workspaceName, oggPath string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	setChatWorkspace(t, client, workspaceName)
	eventChannel, err := client.OpenServiceChannel(32, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer eventChannel.Close()
	if err := eventChannel.SendJSON(`{"v":1,"type":"bos","stream_id":"cgo-chat","label":"cgo-chat","kind":"audio","mime_type":"audio/opus"}`); err != nil {
		t.Fatalf("send chat BOS: %v", err)
	}
	timestamp := uint64(1)
	for _, packet := range opusPacketsFromOgg(t, oggPath) {
		if err := client.SendPacket(0x10, stampedopus.Pack(timestamp, packet)); err != nil {
			t.Fatalf("send chat opus packet: %v", err)
		}
		if err := client.Poll(20 * time.Millisecond); err != nil {
			t.Fatalf("pace chat opus packet: %v", err)
		}
		timestamp += 20
	}
	if err := eventChannel.SendJSON(`{"v":1,"type":"eos","stream_id":"cgo-chat","label":"cgo-chat","kind":"audio","mime_type":"audio/opus"}`); err != nil {
		t.Fatalf("send chat EOS: %v", err)
	}
	deadline := time.Now().Add(90 * time.Second)
	var sawText bool
	var sawEventEOS bool
	var eventFrames int
	var downlinkPackets int
	for time.Now().Before(deadline) {
		frame, err := eventChannel.ReadFrame(50 * time.Millisecond)
		if err == nil {
			eventFrames++
			if frame.Type == int(C.GZC_RPC_FRAME_JSON) || frame.Type == int(C.GZC_RPC_FRAME_TEXT) {
				if bytes.Contains(frame.Data, []byte(`"type":"text.`)) && bytes.Contains(frame.Data, []byte(`"text"`)) {
					sawText = true
				}
				if bytes.Contains(frame.Data, []byte(`"type":"eos"`)) {
					sawEventEOS = true
				}
			}
		} else if !errors.Is(err, errCSDKTimeout) {
			t.Fatalf("read chat event frame: %v", err)
		}
		protocol, payload, err := client.ReadPacket(50 * time.Millisecond)
		if err == nil {
			if protocol == 0x10 && len(payload) > stampedopus.HeaderSize && payload[0] == stampedopus.Version {
				downlinkPackets++
			}
		} else if !errors.Is(err, errCSDKTimeout) {
			t.Fatalf("read chat packet: %v", err)
		}
		if sawText && downlinkPackets > 0 && sawEventEOS {
			return
		}
	}
	if !sawText || downlinkPackets == 0 {
		t.Fatalf("chat roundtrip missing text or audio: events=%d saw_text=%v saw_eos=%v downlink_packets=%d", eventFrames, sawText, sawEventEOS, downlinkPackets)
	}
}

func CSDKSocialBasic(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	unique := time.Now().UnixMilli()
	contactName := fmt.Sprintf("C SDK Social Contact %d", unique)
	contactPhone := fmt.Sprintf("+1555%010d", unique%10000000000)
	groupName := fmt.Sprintf("c-sdk-social-group-%d", unique)

	result := mustCallJSON(t, client, "server.contact.create", fmt.Sprintf(`{"display_name":%q,"phone_number":%q}`, contactName, contactPhone))
	var contactCreate struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	}
	decodeJSON(t, "server.contact.create", result, &contactCreate)
	if contactCreate.ID == "" || contactCreate.DisplayName != contactName {
		t.Fatalf("invalid server.contact.create: %s", string(result))
	}
	contactID := contactCreate.ID
	result = mustCallJSON(t, client, "server.contact.get", fmt.Sprintf(`{"id":%q}`, contactID))
	if !bytes.Contains(result, []byte(contactID)) {
		t.Fatalf("invalid server.contact.get: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.contact.list", `{"limit":1000}`)
	if !bytes.Contains(result, []byte(contactID)) {
		t.Fatalf("invalid server.contact.list: %s", string(result))
	}

	result = mustCallJSON(t, client, "server.friend_group.create", fmt.Sprintf(`{"name":%q,"description":"created by cgo C SDK test"}`, groupName))
	var groupCreate struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		WorkspaceName string `json:"workspace_name"`
	}
	decodeJSON(t, "server.friend_group.create", result, &groupCreate)
	if groupCreate.ID == "" || groupCreate.Name != groupName || groupCreate.WorkspaceName == "" {
		t.Fatalf("invalid server.friend_group.create: %s", string(result))
	}
	groupID := groupCreate.ID
	result = mustCallJSON(t, client, "server.friend_group.get", fmt.Sprintf(`{"id":%q}`, groupID))
	if !bytes.Contains(result, []byte(groupID)) {
		t.Fatalf("invalid server.friend_group.get: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.friend_group.list", `{"limit":1000}`)
	if !bytes.Contains(result, []byte(groupID)) {
		t.Fatalf("invalid server.friend_group.list: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.friend_group.invite_token.create", fmt.Sprintf(`{"friend_group_id":%q}`, groupID))
	var tokenResponse struct {
		InviteToken string `json:"invite_token"`
		ExpiresAt   string `json:"expires_at"`
	}
	decodeJSON(t, "server.friend_group.invite_token.create", result, &tokenResponse)
	if tokenResponse.InviteToken == "" || tokenResponse.ExpiresAt == "" {
		t.Fatalf("invalid server.friend_group.invite_token.create: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.friend_group.messages.send", fmt.Sprintf(`{"friend_group_id":%q,"audio_content_type":"audio/opus","audio_base64":"bm90LXJlYWwtb3B1cy1idXQtcnBjLXBheWxvYWQ="}`, groupID))
	var messageSend struct {
		ID            string `json:"id"`
		FriendGroupID string `json:"friend_group_id"`
	}
	decodeJSON(t, "server.friend_group.messages.send", result, &messageSend)
	if messageSend.ID == "" || messageSend.FriendGroupID != groupID {
		t.Fatalf("invalid server.friend_group.messages.send: %s", string(result))
	}
	messageID := messageSend.ID
	result = mustCallJSON(t, client, "server.friend_group.messages.get", fmt.Sprintf(`{"friend_group_id":%q,"id":%q}`, groupID, messageID))
	if !bytes.Contains(result, []byte(messageID)) {
		t.Fatalf("invalid server.friend_group.messages.get: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.friend_group.messages.list", fmt.Sprintf(`{"friend_group_id":%q,"limit":1000}`, groupID))
	if !bytes.Contains(result, []byte(messageID)) {
		t.Fatalf("invalid server.friend_group.messages.list: %s", string(result))
	}
}

func CSDKSocialRelationships(t *testing.T, identityADir, identityBDir string) {
	t.Helper()
	clientA := newTestClient(t, identityADir)
	defer clientA.Close()
	clientB := newTestClient(t, identityBDir)
	defer clientB.Close()

	result := mustCallJSON(t, clientB, "server.friend.invite_token.create", `{}`)
	var friendToken struct {
		InviteToken string `json:"invite_token"`
		ExpiresAt   string `json:"expires_at"`
	}
	decodeJSON(t, "server.friend.invite_token.create", result, &friendToken)
	if friendToken.InviteToken == "" || friendToken.ExpiresAt == "" {
		t.Fatalf("invalid server.friend.invite_token.create: %s", string(result))
	}
	result = mustCallJSON(t, clientA, "server.friend.add", fmt.Sprintf(`{"invite_token":%q}`, friendToken.InviteToken))
	var friendAdd struct {
		ID            string `json:"id"`
		WorkspaceName string `json:"workspace_name"`
		PeerPublicKey string `json:"peer_public_key"`
	}
	decodeJSON(t, "server.friend.add", result, &friendAdd)
	if friendAdd.ID == "" || friendAdd.WorkspaceName == "" || friendAdd.PeerPublicKey == "" {
		t.Fatalf("invalid server.friend.add: %s", string(result))
	}

	result = mustCallJSON(t, clientA, "server.friend_group.create", `{"name":"c-sdk-cross-user-group","description":"created by cgo C SDK relationship test"}`)
	var groupCreate struct {
		ID            string `json:"id"`
		WorkspaceName string `json:"workspace_name"`
	}
	decodeJSON(t, "server.friend_group.create", result, &groupCreate)
	if groupCreate.ID == "" || groupCreate.WorkspaceName == "" {
		t.Fatalf("invalid server.friend_group.create: %s", string(result))
	}
	groupID := groupCreate.ID
	result = mustCallJSON(t, clientA, "server.friend_group.invite_token.create", fmt.Sprintf(`{"friend_group_id":%q}`, groupID))
	var groupToken struct {
		InviteToken string `json:"invite_token"`
		ExpiresAt   string `json:"expires_at"`
	}
	decodeJSON(t, "server.friend_group.invite_token.create", result, &groupToken)
	if groupToken.InviteToken == "" || groupToken.ExpiresAt == "" {
		t.Fatalf("invalid server.friend_group.invite_token.create: %s", string(result))
	}
	result = mustCallJSON(t, clientB, "server.friend_group.join", fmt.Sprintf(`{"invite_token":%q}`, groupToken.InviteToken))
	if !bytes.Contains(result, []byte(groupID)) {
		t.Fatalf("invalid server.friend_group.join: %s", string(result))
	}
	result = mustCallJSON(t, clientB, "server.friend_group.members.list", fmt.Sprintf(`{"friend_group_id":%q,"limit":1000}`, groupID))
	if !bytes.Contains(result, []byte(groupID)) {
		t.Fatalf("invalid server.friend_group.members.list: %s", string(result))
	}
	result = mustCallJSON(t, clientB, "server.friend_group.messages.send", fmt.Sprintf(`{"friend_group_id":%q,"audio_content_type":"audio/opus","audio_base64":"Yy1zZGstY3Jvc3MtdXNlci1zb2NpYWwtbWVzc2FnZQ=="}`, groupID))
	var messageSend struct {
		ID            string `json:"id"`
		FriendGroupID string `json:"friend_group_id"`
	}
	decodeJSON(t, "server.friend_group.messages.send", result, &messageSend)
	if messageSend.ID == "" || messageSend.FriendGroupID != groupID {
		t.Fatalf("invalid server.friend_group.messages.send: %s", string(result))
	}
	messageID := messageSend.ID
	result = mustCallJSON(t, clientA, "server.friend_group.messages.get", fmt.Sprintf(`{"friend_group_id":%q,"id":%q}`, groupID, messageID))
	if !bytes.Contains(result, []byte(messageID)) {
		t.Fatalf("invalid server.friend_group.messages.get: %s", string(result))
	}
	result = mustCallJSON(t, clientA, "server.friend_group.messages.list", fmt.Sprintf(`{"friend_group_id":%q,"limit":1000}`, groupID))
	if !bytes.Contains(result, []byte(messageID)) {
		t.Fatalf("invalid server.friend_group.messages.list: %s", string(result))
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

func mustCallJSON(t *testing.T, client *Client, method, params string) json.RawMessage {
	t.Helper()
	result, err := client.CallJSON(method, json.RawMessage(params))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func decodeJSON(t *testing.T, label string, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode %s: %v: %s", label, err, string(data))
	}
}

func streamResultJSON(t *testing.T, label string, frame []byte) json.RawMessage {
	t.Helper()
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeJSON(t, label+" envelope", frame, &envelope)
	if envelope.Error != nil {
		t.Fatalf("%s stream error: %d %s", label, envelope.Error.Code, envelope.Error.Message)
	}
	if len(envelope.Result) == 0 {
		t.Fatalf("%s stream envelope has empty result: %s", label, string(frame))
	}
	return envelope.Result
}

func setChatWorkspace(t *testing.T, client *Client, workspaceName string) {
	t.Helper()
	result := mustCallJSON(t, client, "server.run.workspace.set", fmt.Sprintf(`{"workspace_name":%q}`, workspaceName))
	var setResponse struct {
		WorkspaceName string `json:"workspace_name"`
	}
	decodeJSON(t, "server.run.workspace.set", result, &setResponse)
	if setResponse.WorkspaceName != workspaceName {
		t.Fatalf("invalid server.run.workspace.set: %s", string(result))
	}
	result = mustCallJSON(t, client, "server.run.workspace.reload", `{}`)
	var reloadResponse struct {
		WorkspaceName string `json:"workspace_name"`
	}
	decodeJSON(t, "server.run.workspace.reload", result, &reloadResponse)
	if reloadResponse.WorkspaceName != workspaceName {
		t.Fatalf("invalid server.run.workspace.reload: %s", string(result))
	}
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
