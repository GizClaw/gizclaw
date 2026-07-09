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
	"github.com/GizClaw/gizclaw-go/pkgs/audio/stampedopus"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
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
	rc := C.gzc_cgo_session_call_rpc_payload(
		c.session,
		C.uint(method),
		cParams,
		C.ulong(len(paramsPayload)),
		&result,
		&resultLen,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
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
		default:
			t.Fatalf("unexpected speed test frame type %d", frame.Type)
		}
	}
	if !sawAck || binaryBytes != 4096 {
		t.Fatalf("invalid speed test stream: saw_ack=%v binary_bytes=%d", sawAck, binaryBytes)
	}
}

func CSDKFirmwareRPC(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	var listResponse rpcpb.FirmwareListResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_LIST, &rpcpb.FirmwareListRequest{Limit: ptr(int64(5))}, &listResponse)
	if len(listResponse.GetItems()) == 0 {
		t.Fatalf("empty server.firmware.list: %s", listResponse.String())
	}
	var getResponse rpcpb.FirmwareGetResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_GET, &rpcpb.FirmwareGetRequest{FirmwareId: "devkit-firmware-main"}, &getResponse)
	firmware := getResponse.GetValue()
	if firmware == nil || firmware.GetName() != "devkit-firmware-main" || firmware.GetSlots() == nil {
		t.Fatalf("invalid server.firmware.get: %s", getResponse.String())
	}
}

func CSDKFirmwareDownload(t *testing.T, identityDir string) {
	t.Helper()
	client := newTestClient(t, identityDir)
	defer client.Close()
	frames, err := client.CallStream(rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_FILES_DOWNLOAD, &rpcpb.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit-firmware-main",
		Channel:    rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_STABLE,
		Path:       "firmware/main.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawMetadata bool
	var binaryBytes int
	var sawMarker bool
	for _, frame := range frames {
		switch frame.Type {
		case int(C.GZC_RPC_FRAME_BINARY):
			if !sawMetadata {
				var response rpcpb.FirmwareFilesDownloadResponse
				decodeStreamResponse(t, rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_FILES_DOWNLOAD, frame.Data, &response)
				if response.GetFirmwareId() != "devkit-firmware-main" || response.GetPath() != "firmware/main.bin" {
					t.Fatalf("invalid firmware download metadata: %s", response.String())
				}
				sawMetadata = true
				continue
			}
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
	var workspaceResponse rpcpb.WorkspaceGetResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_GET, &rpcpb.WorkspaceGetRequest{Name: "direct-chatroom-workspace"}, &workspaceResponse)
	workspace := workspaceResponse.GetValue()
	if workspace == nil || workspace.GetName() != "direct-chatroom-workspace" || workspace.GetWorkflowName() != "chatroom-direct" {
		t.Fatalf("invalid server.workspace.get: %s", workspaceResponse.String())
	}
	var setResponse rpcpb.ServerSetRunWorkspaceResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_SET, &rpcpb.ServerSetRunWorkspaceRequest{
		Value: &rpcpb.AgentSelection{WorkspaceName: "direct-chatroom-workspace"},
	}, &setResponse)
	if setResponse.GetValue().GetWorkspaceName() != "direct-chatroom-workspace" {
		t.Fatalf("invalid server.run.workspace.set: %s", setResponse.String())
	}
	var getResponse rpcpb.ServerGetRunWorkspaceResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_GET, &rpcpb.ServerGetRunWorkspaceRequest{}, &getResponse)
	if getResponse.GetValue().GetWorkspaceName() != "direct-chatroom-workspace" ||
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
	if historyResponse.GetValue() == nil || historyResponse.GetValue().GetItems() == nil {
		t.Fatalf("invalid server.run.workspace.history: %s", historyResponse.String())
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

	var contactCreate rpcpb.ContactCreateResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_CREATE, &rpcpb.ContactCreateRequest{
		DisplayName: ptr(contactName),
		PhoneNumber: ptr(contactPhone),
	}, &contactCreate)
	if contactCreate.GetValue().GetId() == "" || contactCreate.GetValue().GetDisplayName() != contactName {
		t.Fatalf("invalid server.contact.create: %s", contactCreate.String())
	}
	contactID := contactCreate.GetValue().GetId()
	var contactGet rpcpb.ContactGetResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_GET, &rpcpb.ContactGetRequest{Id: contactID}, &contactGet)
	if contactGet.GetValue().GetId() != contactID {
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
	if groupCreate.GetValue().GetId() == "" || groupCreate.GetValue().GetName() != groupName || groupCreate.GetValue().GetWorkspaceName() == "" {
		t.Fatalf("invalid server.friend_group.create: %s", groupCreate.String())
	}
	groupID := groupCreate.GetValue().GetId()
	var groupGet rpcpb.FriendGroupGetResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_GET, &rpcpb.FriendGroupGetRequest{Id: groupID}, &groupGet)
	if groupGet.GetValue().GetId() != groupID {
		t.Fatalf("invalid server.friend_group.get: %s", groupGet.String())
	}
	var groupList rpcpb.FriendGroupListResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_LIST, &rpcpb.FriendGroupListRequest{Limit: ptr(int64(1000))}, &groupList)
	if !friendGroupListContains(groupList.GetItems(), groupID) {
		t.Fatalf("invalid server.friend_group.list: %s", groupList.String())
	}
	var tokenResponse rpcpb.FriendGroupInviteTokenCreateResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE, &rpcpb.FriendGroupInviteTokenCreateRequest{FriendGroupId: groupID}, &tokenResponse)
	if tokenResponse.GetInviteToken() == "" || tokenResponse.GetExpiresAt() == "" {
		t.Fatalf("invalid server.friend_group.invite_token.create: %s", tokenResponse.String())
	}
	var messageSend rpcpb.FriendGroupMessageSendResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND, &rpcpb.FriendGroupMessageSendRequest{
		FriendGroupId:    groupID,
		AudioContentType: "audio/opus",
		AudioBase64:      []byte("not-real-opus-but-rpc-payload"),
	}, &messageSend)
	if messageSend.GetValue().GetId() == "" || messageSend.GetValue().GetFriendGroupId() != groupID {
		t.Fatalf("invalid server.friend_group.messages.send: %s", messageSend.String())
	}
	messageID := messageSend.GetValue().GetId()
	var messageGet rpcpb.FriendGroupMessageGetResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET, &rpcpb.FriendGroupMessageGetRequest{FriendGroupId: groupID, Id: messageID}, &messageGet)
	if messageGet.GetValue().GetId() != messageID {
		t.Fatalf("invalid server.friend_group.messages.get: %s", messageGet.String())
	}
	var messageList rpcpb.FriendGroupMessageListResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST, &rpcpb.FriendGroupMessageListRequest{
		FriendGroupId: ptr(groupID),
		Limit:         ptr(int64(1000)),
	}, &messageList)
	if !friendGroupMessageListContains(messageList.GetItems(), messageID) {
		t.Fatalf("invalid server.friend_group.messages.list: %s", messageList.String())
	}
}

func CSDKSocialRelationships(t *testing.T, identityADir, identityBDir string) {
	t.Helper()
	clientA := newTestClient(t, identityADir)
	defer clientA.Close()
	clientB := newTestClient(t, identityBDir)
	defer clientB.Close()

	var friendToken rpcpb.FriendInviteTokenCreateResponse
	mustCallRPC(t, clientB, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE, &rpcpb.FriendInviteTokenCreateRequest{}, &friendToken)
	if friendToken.GetInviteToken() == "" || friendToken.GetExpiresAt() == "" {
		t.Fatalf("invalid server.friend.invite_token.create: %s", friendToken.String())
	}
	var friendAdd rpcpb.FriendAddResponse
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_ADD, &rpcpb.FriendAddRequest{InviteToken: friendToken.GetInviteToken()}, &friendAdd)
	if friendAdd.GetValue().GetId() == "" || friendAdd.GetValue().GetWorkspaceName() == "" || friendAdd.GetValue().GetPeerPublicKey() == "" {
		t.Fatalf("invalid server.friend.add: %s", friendAdd.String())
	}

	var groupCreate rpcpb.FriendGroupCreateResponse
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_CREATE, &rpcpb.FriendGroupCreateRequest{
		Name:        "c-sdk-cross-user-group",
		Description: ptr("created by cgo C SDK relationship test"),
	}, &groupCreate)
	if groupCreate.GetValue().GetId() == "" || groupCreate.GetValue().GetWorkspaceName() == "" {
		t.Fatalf("invalid server.friend_group.create: %s", groupCreate.String())
	}
	groupID := groupCreate.GetValue().GetId()
	var groupToken rpcpb.FriendGroupInviteTokenCreateResponse
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE, &rpcpb.FriendGroupInviteTokenCreateRequest{FriendGroupId: groupID}, &groupToken)
	if groupToken.GetInviteToken() == "" || groupToken.GetExpiresAt() == "" {
		t.Fatalf("invalid server.friend_group.invite_token.create: %s", groupToken.String())
	}
	var groupJoin rpcpb.FriendGroupJoinResponse
	mustCallRPC(t, clientB, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_JOIN, &rpcpb.FriendGroupJoinRequest{InviteToken: groupToken.GetInviteToken()}, &groupJoin)
	if groupJoin.GetGroup().GetId() != groupID {
		t.Fatalf("invalid server.friend_group.join: %s", groupJoin.String())
	}
	var memberList rpcpb.FriendGroupMemberListResponse
	mustCallRPC(t, clientB, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST, &rpcpb.FriendGroupMemberListRequest{
		FriendGroupId: ptr(groupID),
		Limit:         ptr(int64(1000)),
	}, &memberList)
	if !friendGroupMemberListContains(memberList.GetItems(), groupID) {
		t.Fatalf("invalid server.friend_group.members.list: %s", memberList.String())
	}
	var messageSend rpcpb.FriendGroupMessageSendResponse
	mustCallRPC(t, clientB, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND, &rpcpb.FriendGroupMessageSendRequest{
		FriendGroupId:    groupID,
		AudioContentType: "audio/opus",
		AudioBase64:      []byte("c-sdk-cross-user-social-message"),
	}, &messageSend)
	if messageSend.GetValue().GetId() == "" || messageSend.GetValue().GetFriendGroupId() != groupID {
		t.Fatalf("invalid server.friend_group.messages.send: %s", messageSend.String())
	}
	messageID := messageSend.GetValue().GetId()
	var messageGet rpcpb.FriendGroupMessageGetResponse
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET, &rpcpb.FriendGroupMessageGetRequest{
		FriendGroupId: groupID,
		Id:            messageID,
	}, &messageGet)
	if messageGet.GetValue().GetId() != messageID {
		t.Fatalf("invalid server.friend_group.messages.get: %s", messageGet.String())
	}
	var messageList rpcpb.FriendGroupMessageListResponse
	mustCallRPC(t, clientA, rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST, &rpcpb.FriendGroupMessageListRequest{
		FriendGroupId: ptr(groupID),
		Limit:         ptr(int64(1000)),
	}, &messageList)
	if !friendGroupMessageListContains(messageList.GetItems(), messageID) {
		t.Fatalf("invalid server.friend_group.messages.list: %s", messageList.String())
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
	var setResponse rpcpb.ServerSetRunWorkspaceResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_SET, &rpcpb.ServerSetRunWorkspaceRequest{
		Value: &rpcpb.AgentSelection{WorkspaceName: workspaceName},
	}, &setResponse)
	if setResponse.GetValue().GetWorkspaceName() != workspaceName {
		t.Fatalf("invalid server.run.workspace.set: %s", setResponse.String())
	}
	var reloadResponse rpcpb.ServerReloadRunWorkspaceResponse
	mustCallRPC(t, client, rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD, &rpcpb.ServerReloadRunWorkspaceRequest{}, &reloadResponse)
	if reloadResponse.GetValue().GetWorkspaceName() != workspaceName {
		t.Fatalf("invalid server.run.workspace.reload: %s", reloadResponse.String())
	}
}

func ptr[T any](value T) *T {
	return &value
}

func contactListContains(items []*rpcpb.ContactObject, id string) bool {
	for _, item := range items {
		if item.GetId() == id {
			return true
		}
	}
	return false
}

func friendGroupListContains(items []*rpcpb.FriendGroupObject, id string) bool {
	for _, item := range items {
		if item.GetId() == id {
			return true
		}
	}
	return false
}

func friendGroupMemberListContains(items []*rpcpb.FriendGroupMemberObject, groupID string) bool {
	for _, item := range items {
		if item.GetFriendGroupId() == groupID {
			return true
		}
	}
	return false
}

func friendGroupMessageListContains(items []*rpcpb.FriendGroupMessageObject, id string) bool {
	for _, item := range items {
		if item.GetId() == id {
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
