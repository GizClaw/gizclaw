package gizcli

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

func TestWebRTCOpusPacketRTPTicks(t *testing.T) {
	tests := []struct {
		name   string
		packet []byte
		want   uint32
	}{
		{name: "empty defaults to twenty milliseconds", packet: nil, want: 960},
		{name: "silk ten milliseconds", packet: []byte{0x00}, want: 480},
		{name: "silk sixty milliseconds", packet: []byte{0x18}, want: 2880},
		{name: "hybrid twenty milliseconds", packet: []byte{0x78}, want: 960},
		{name: "celt two point five milliseconds", packet: []byte{0x80}, want: 120},
		{name: "celt twenty milliseconds", packet: []byte{0x98}, want: 960},
		{name: "two cbr frames", packet: []byte{0x99}, want: 1920},
		{name: "arbitrary frame count", packet: []byte{0x9b, 0x03}, want: 2880},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := webRTCOpusPacketRTPTicks(tt.packet)
			if got != tt.want {
				t.Fatalf("webRTCOpusPacketRTPTicks() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsWebRTCRPCDataChannel(t *testing.T) {
	tests := []struct {
		label string
		want  bool
	}{
		{label: "rpc", want: true},
		{label: "rpc:play-1", want: true},
		{label: "rpc-bootstrap", want: false},
		{label: "chat", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := isWebRTCRPCDataChannel(tt.label)
			if got != tt.want {
				t.Fatalf("isWebRTCRPCDataChannel(%q) = %t, want %t", tt.label, got, tt.want)
			}
		})
	}
}

func TestIsWebRTCEventDataChannel(t *testing.T) {
	tests := []struct {
		label string
		want  bool
	}{
		{label: "event", want: true},
		{label: "event:play-1", want: true},
		{label: "event-bootstrap", want: false},
		{label: "rpc", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := isWebRTCEventDataChannel(tt.label)
			if got != tt.want {
				t.Fatalf("isWebRTCEventDataChannel(%q) = %t, want %t", tt.label, got, tt.want)
			}
		})
	}
}

func TestWebRTCPeerStreamEventFrameRoundTrip(t *testing.T) {
	text := "hello"
	streamID := "s1"
	event := apitypes.PeerStreamEvent{
		Type:     apitypes.PeerStreamEventTypeTextDelta,
		StreamId: &streamID,
		Text:     &text,
	}
	var buf bytes.Buffer
	if err := writeWebRTCPeerStreamEvent(&buf, event); err != nil {
		t.Fatalf("writeWebRTCPeerStreamEvent() error = %v", err)
	}
	got, err := readWebRTCPeerStreamEvent(&buf)
	if err != nil {
		t.Fatalf("readWebRTCPeerStreamEvent() error = %v", err)
	}
	if got.V != 1 || got.Type != event.Type || got.StreamId == nil || *got.StreamId != streamID || got.Text == nil || *got.Text != text {
		t.Fatalf("round trip event = %+v", got)
	}
}

func TestWebRTCRPCDataChannelProtobufFrames(t *testing.T) {
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(rpcapi.PingRequest{ClientSendTime: 123}); err != nil {
		t.Fatalf("FromPingRequest() error = %v", err)
	}
	req := &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     "req-1",
		Method: rpcapi.RPCMethodAllPing,
		Params: &params,
	}
	reqFrame, err := rpcapi.NewRequestFrame(req)
	if err != nil {
		t.Fatalf("NewRequestFrame() error = %v", err)
	}
	var reqBuf bytes.Buffer
	if err := rpcapi.WriteFrame(&reqBuf, reqFrame); err != nil {
		t.Fatalf("WriteFrame(request) error = %v", err)
	}
	if err := rpcapi.WriteEOS(&reqBuf); err != nil {
		t.Fatalf("WriteEOS(request) error = %v", err)
	}
	gotReq, err := readWebRTCRPCDataChannelRequest(reqBuf.Bytes())
	if err != nil {
		t.Fatalf("readWebRTCRPCDataChannelRequest() error = %v", err)
	}
	if gotReq.Id != req.Id || gotReq.Method != req.Method || gotReq.Params == nil {
		t.Fatalf("decoded request = %+v", gotReq)
	}

	var result rpcapi.RPCPayload
	if err := result.FromPingResponse(rpcapi.PingResponse{ServerTime: 456}); err != nil {
		t.Fatalf("FromPingResponse() error = %v", err)
	}
	resp := &rpcapi.RPCResponse{
		V:      rpcapi.RPCVersionV1,
		Id:     req.Id,
		Result: &result,
	}
	var respBuf bytes.Buffer
	if err := writeWebRTCRPCDataChannelResponse(&respBuf, req.Method, resp); err != nil {
		t.Fatalf("writeWebRTCRPCDataChannelResponse() error = %v", err)
	}
	gotResp, err := rpcapi.ReadResponseForMethod(&respBuf, req.Method)
	if err != nil {
		t.Fatalf("ReadResponseForMethod() error = %v", err)
	}
	if err := rpcapi.ReadEOS(&respBuf); err != nil {
		t.Fatalf("ReadEOS(response) error = %v", err)
	}
	gotResult, err := gotResp.Result.AsPingResponse()
	if err != nil {
		t.Fatalf("AsPingResponse() error = %v", err)
	}
	if gotResp.Id != req.Id || gotResult.ServerTime != 456 {
		t.Fatalf("decoded response = %+v result=%+v", gotResp, gotResult)
	}
}

func TestWebRTCRPCDataChannelRejectsOversizedContinuationEnvelope(t *testing.T) {
	var reqBuf bytes.Buffer
	chunk := bytes.Repeat([]byte("x"), rpcapi.MaxFrameSize)
	for written := 0; written <= webRTCRPCMaxEnvelopeSize; written += len(chunk) {
		if err := rpcapi.WriteFrame(&reqBuf, rpcapi.Frame{Type: rpcapi.FrameTypeText, Payload: chunk}); err != nil {
			t.Fatalf("WriteFrame() error = %v", err)
		}
	}
	if err := rpcapi.WriteEOS(&reqBuf); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}

	_, err := readWebRTCRPCDataChannelRequest(reqBuf.Bytes())
	if err == nil || !strings.Contains(err.Error(), "request envelope too large") {
		t.Fatalf("readWebRTCRPCDataChannelRequest() error = %v, want request envelope too large", err)
	}
}

func TestClientRegisterToWebRTCValidationAndClose(t *testing.T) {
	var nilClient *Client
	if _, err := nilClient.RegisterTo(&webrtc.PeerConnection{}); err == nil || err.Error() != "gizclaw: nil client" {
		t.Fatalf("nil RegisterTo() error = %v", err)
	}
	if _, err := (&Client{}).RegisterTo(nil); err == nil || err.Error() != "gizclaw: nil peer connection" {
		t.Fatalf("RegisterTo(nil pc) error = %v", err)
	}
	if track := (*ClientWebRTCRegistration)(nil).AudioTrack(); track != nil {
		t.Fatalf("nil registration AudioTrack() = %v, want nil", track)
	}
	if err := (*ClientWebRTCRegistration)(nil).Close(); err != nil {
		t.Fatalf("nil registration Close() error = %v", err)
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection() error = %v", err)
	}
	defer pc.Close()

	reg, err := (&Client{}).RegisterTo(pc)
	if err != nil {
		t.Fatalf("RegisterTo() error = %v", err)
	}
	if reg.AudioTrack() == nil {
		t.Fatal("AudioTrack() = nil")
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("registration Close() error = %v", err)
	}
}

func TestClientPeerPacketSubscriptionCopiesAndUnsubscribes(t *testing.T) {
	client := &Client{}
	packets, unsubscribe := client.subscribePeerPackets(giznet.ProtocolOpusPacket, 1)

	payload := []byte("frame")
	client.dispatchPeerPacket(giznet.ProtocolOpusPacket, payload)
	payload[0] = 'x'

	select {
	case got := <-packets:
		if !bytes.Equal(got, []byte("frame")) {
			t.Fatalf("packet = %q, want copied payload", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for packet")
	}

	unsubscribe()
	client.dispatchPeerPacket(giznet.ProtocolOpusPacket, []byte("dropped"))

	select {
	case got := <-packets:
		t.Fatalf("received packet after unsubscribe: %q", got)
	case <-time.After(10 * time.Millisecond):
	}
}

func TestIsPeerPacketReadClosed(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "eof", err: io.EOF, want: true},
		{name: "net closed", err: net.ErrClosed, want: true},
		{name: "conn closed", err: giznet.ErrConnClosed, want: true},
		{name: "udp closed", err: giznet.ErrClosed, want: true},
		{name: "service mux closed", err: giznet.ErrServiceMuxClosed, want: true},
		{name: "wrapped", err: errors.Join(errors.New("read failed"), giznet.ErrClosed), want: true},
		{name: "other", err: errors.New("boom"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPeerPacketReadClosed(tt.err)
			if got != tt.want {
				t.Fatalf("isPeerPacketReadClosed(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}
