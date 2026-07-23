package gizcli

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	WebRTCDataChannelRPCLabel   = "rpc"
	WebRTCDataChannelEventLabel = "event"

	webRTCAudioTrackID     = "gizclaw-audio"
	webRTCAudioStreamID    = "gizclaw"
	webRTCOpusClockRate    = 48000
	webRTCOpusPayloadType  = 111
	webRTCRPCTimeout       = 30 * time.Second
	webRTCMessageChunkSize = 1400

	webRTCRPCMaxEnvelopeSize = rpcapi.MaxFrameSize * 16
)

// ClientWebRTCRegistration is the live bridge between one Pion PeerConnection
// and the connected GizClaw peer transport.
type ClientWebRTCRegistration struct {
	client *Client
	pc     *webrtc.PeerConnection

	ctx    context.Context
	cancel context.CancelFunc

	audioTrack  *webrtc.TrackLocalStaticRTP
	audioSender *webrtc.RTPSender
}

// RegisterTo wires this client into a Pion PeerConnection.
//
// The browser-facing contract is intentionally transport-shaped rather than
// signaling-shaped: desktop/frontend callers can use their own signaling
// mechanism, then call RegisterTo before applying the offer/answer.
func (c *Client) RegisterTo(pc *webrtc.PeerConnection) (*ClientWebRTCRegistration, error) {
	if c == nil {
		return nil, fmt.Errorf("gizclaw: nil client")
	}
	if pc == nil {
		return nil, fmt.Errorf("gizclaw: nil peer connection")
	}

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  MediaStreamOpus,
			ClockRate: webRTCOpusClockRate,
			Channels:  2,
		},
		webRTCAudioTrackID,
		webRTCAudioStreamID,
	)
	if err != nil {
		return nil, fmt.Errorf("gizclaw: create webrtc audio track: %w", err)
	}

	audioSender, err := pc.AddTrack(audioTrack)
	if err != nil {
		return nil, fmt.Errorf("gizclaw: add webrtc audio track: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &ClientWebRTCRegistration{
		client:      c,
		pc:          pc,
		ctx:         ctx,
		cancel:      cancel,
		audioTrack:  audioTrack,
		audioSender: audioSender,
	}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		r.registerDataChannel(dc)
	})
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		r.registerRemoteTrack(track)
	})

	go r.forwardPeerOpusToWebRTCAudio()
	go drainWebRTCRTCP(audioSender)

	return r, nil
}

// AudioTrack returns the local WebRTC audio track that receives server-side
// Opus packets.
func (r *ClientWebRTCRegistration) AudioTrack() *webrtc.TrackLocalStaticRTP {
	if r == nil {
		return nil
	}
	return r.audioTrack
}

// Close stops registration-owned forwarding loops. It does not close the
// PeerConnection or the GizClaw Client.
func (r *ClientWebRTCRegistration) Close() error {
	if r == nil {
		return nil
	}
	r.cancel()
	if r.pc != nil && r.audioSender != nil {
		return r.pc.RemoveTrack(r.audioSender)
	}
	return nil
}

func (r *ClientWebRTCRegistration) registerDataChannel(dc *webrtc.DataChannel) {
	if r == nil || dc == nil {
		return
	}
	switch {
	case isWebRTCRPCDataChannel(dc.Label()):
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			go func() {
				r.handleRPCDataChannelMessage(dc, msg)
			}()
		})
	case isWebRTCEventDataChannel(dc.Label()):
		r.registerEventDataChannel(dc)
	}
}

func isWebRTCRPCDataChannel(label string) bool {
	return label == WebRTCDataChannelRPCLabel || strings.HasPrefix(label, WebRTCDataChannelRPCLabel+":")
}

func isWebRTCEventDataChannel(label string) bool {
	return label == WebRTCDataChannelEventLabel || strings.HasPrefix(label, WebRTCDataChannelEventLabel+":")
}

func (r *ClientWebRTCRegistration) registerEventDataChannel(dc *webrtc.DataChannel) {
	var (
		mu      sync.Mutex
		stream  net.Conn
		pending []*eventpb.PeerEvent
		receive bytes.Buffer
		once    sync.Once
	)
	closeStream := func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			pending = nil
			if stream != nil {
				_ = stream.Close()
				stream = nil
			}
		})
	}
	dc.OnOpen(func() {
		conn := r.client.PeerConn()
		if conn == nil {
			_ = dc.Close()
			return
		}
		eventStream, err := conn.Dial(EventStreamAgent)
		if err != nil {
			slog.Debug("gizclaw: dial event stream for webrtc failed", "error", err)
			_ = dc.Close()
			return
		}
		mu.Lock()
		stream = eventStream
		pendingEvents := append([]*eventpb.PeerEvent(nil), pending...)
		pending = nil
		mu.Unlock()
		for _, event := range pendingEvents {
			if err := writeWebRTCPeerStreamEvent(eventStream, event); err != nil {
				slog.Debug("gizclaw: write pending webrtc event to peer failed", "error", err)
				closeStream()
				_ = dc.Close()
				return
			}
		}
		go func() {
			defer func() {
				closeStream()
				_ = dc.Close()
			}()
			r.forwardPeerEventsToWebRTC(dc, eventStream)
		}()
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		mu.Lock()
		events, err := appendWebRTCPeerEventFrames(&receive, msg.Data)
		if err != nil {
			mu.Unlock()
			slog.Debug("gizclaw: decode webrtc event failed", "error", err)
			closeStream()
			_ = dc.Close()
			return
		}
		eventStream := stream
		if eventStream == nil {
			pending = append(pending, events...)
		}
		mu.Unlock()
		if eventStream == nil {
			return
		}
		for _, event := range events {
			if err := writeWebRTCPeerStreamEvent(eventStream, event); err != nil {
				slog.Debug("gizclaw: write webrtc event to peer failed", "error", err)
				closeStream()
				_ = dc.Close()
				return
			}
		}
	})
	dc.OnClose(closeStream)
}

func (r *ClientWebRTCRegistration) forwardPeerEventsToWebRTC(dc *webrtc.DataChannel, stream net.Conn) {
	for {
		if err := r.ctx.Err(); err != nil {
			return
		}
		event, err := readWebRTCPeerStreamEvent(stream)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Debug("gizclaw: read peer event for webrtc failed", "error", err)
			return
		}
		var frame bytes.Buffer
		if err := WritePeerStreamEvent(&frame, event); err != nil {
			slog.Debug("gizclaw: encode peer event for webrtc failed", "error", err)
			return
		}
		data := frame.Bytes()
		for offset := 0; offset < len(data); offset += webRTCMessageChunkSize {
			end := min(offset+webRTCMessageChunkSize, len(data))
			if err := dc.Send(data[offset:end]); err != nil {
				slog.Debug("gizclaw: send peer event to webrtc failed", "error", err)
				return
			}
		}
	}
}

func appendWebRTCPeerEventFrames(buffer *bytes.Buffer, chunk []byte) ([]*eventpb.PeerEvent, error) {
	if buffer == nil {
		return nil, errors.New("gizclaw: nil WebRTC event receive buffer")
	}
	if len(chunk) > 0 {
		_, _ = buffer.Write(chunk)
	}
	var events []*eventpb.PeerEvent
	for buffer.Len() >= 4 {
		header := buffer.Bytes()[:4]
		frameSize := 4 + int(binary.LittleEndian.Uint16(header[:2]))
		if buffer.Len() < frameSize {
			break
		}
		frame := append([]byte(nil), buffer.Next(frameSize)...)
		event, err := ReadPeerStreamEvent(bytes.NewReader(frame))
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func writeWebRTCPeerStreamEvent(w io.Writer, event *eventpb.PeerEvent) error {
	return WritePeerStreamEvent(w, event)
}

func readWebRTCPeerStreamEvent(r io.Reader) (*eventpb.PeerEvent, error) {
	return ReadPeerStreamEvent(r)
}

func (r *ClientWebRTCRegistration) handleRPCDataChannelMessage(dc *webrtc.DataChannel, msg webrtc.DataChannelMessage) {
	req, err := readWebRTCRPCDataChannelRequest(msg.Data)
	if err != nil {
		r.sendRPCDataChannelResponse(dc, "", rpcapi.Error{Code: rpcapi.RPCErrorCode(-32700), Message: fmt.Sprintf("invalid rpc protobuf: %v", err)}.RPCResponse())
		return
	}

	ctx, cancel := context.WithTimeout(r.ctx, webRTCRPCTimeout)
	defer cancel()

	resp, err := r.client.callRPCRequest(ctx, req)
	if err != nil {
		resp = rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCode(-32000), Message: err.Error()}.RPCResponse()
	}
	r.sendRPCDataChannelResponse(dc, req.Method, resp)
}

func readWebRTCRPCDataChannelRequest(data []byte) (*rpcapi.RPCRequest, error) {
	reader := bytes.NewReader(data)
	frame, err := rpcapi.ReadFrame(reader)
	if err != nil {
		return nil, err
	}
	var req *rpcapi.RPCRequest
	switch frame.Type {
	case rpcapi.FrameTypeBinary:
		req, err = rpcapi.DecodeRequestFrame(frame)
		if err != nil {
			return nil, err
		}
		if err := rpcapi.ReadEOS(reader); err != nil {
			return nil, err
		}
	case rpcapi.FrameTypeText:
		var payload bytes.Buffer
		payload.Write(frame.Payload)
		if payload.Len() > webRTCRPCMaxEnvelopeSize {
			return nil, fmt.Errorf("rpc: protobuf request envelope too large")
		}
		for {
			next, err := rpcapi.ReadFrame(reader)
			if err != nil {
				return nil, err
			}
			if next.Type == rpcapi.FrameTypeEOS {
				break
			}
			if next.Type != rpcapi.FrameTypeText {
				return nil, fmt.Errorf("rpc: expected protobuf continuation frame, got type %d", next.Type)
			}
			if payload.Len()+len(next.Payload) > webRTCRPCMaxEnvelopeSize {
				return nil, fmt.Errorf("rpc: protobuf request envelope too large")
			}
			payload.Write(next.Payload)
		}
		req, err = rpcapi.DecodeRequestFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload.Bytes()})
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("rpc: expected protobuf binary frame, got type %d", frame.Type)
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("rpc: trailing data after EOS")
	}
	return req, nil
}

func writeWebRTCRPCDataChannelResponse(out *bytes.Buffer, method rpcapi.RPCMethod, resp *rpcapi.RPCResponse) error {
	var frame rpcapi.Frame
	var err error
	if method != "" {
		frame, err = rpcapi.NewResponseFrameForMethod(method, resp)
	} else {
		frame, err = rpcapi.NewResponseFrame(resp)
	}
	if err != nil {
		return err
	}
	if frame.Type != rpcapi.FrameTypeBinary {
		return fmt.Errorf("rpc: expected protobuf binary frame, got type %d", frame.Type)
	}
	if len(frame.Payload) <= rpcapi.MaxFrameSize {
		if err := rpcapi.WriteFrame(out, frame); err != nil {
			return err
		}
		return rpcapi.WriteEOS(out)
	}
	for payload := frame.Payload; len(payload) > 0; {
		n := min(len(payload), rpcapi.MaxFrameSize)
		if err := rpcapi.WriteFrame(out, rpcapi.Frame{Type: rpcapi.FrameTypeText, Payload: payload[:n]}); err != nil {
			return err
		}
		payload = payload[n:]
	}
	return rpcapi.WriteEOS(out)
}

func (r *ClientWebRTCRegistration) sendRPCDataChannelResponse(dc *webrtc.DataChannel, method rpcapi.RPCMethod, resp *rpcapi.RPCResponse) {
	if dc == nil || resp == nil {
		return
	}
	defer func() {
		if err := dc.Close(); err != nil {
			slog.Debug("gizclaw: close webrtc rpc data channel failed", "error", err)
		}
	}()
	if resp.V == 0 {
		resp.V = rpcapi.RPCVersionV1
	}

	var data bytes.Buffer
	err := writeWebRTCRPCDataChannelResponse(&data, method, resp)
	if err != nil {
		slog.Debug("gizclaw: marshal webrtc rpc protobuf response failed", "error", err)
		return
	}
	if err := dc.Send(data.Bytes()); err != nil {
		slog.Debug("gizclaw: send webrtc rpc binary response failed", "error", err)
	}
}

func (r *ClientWebRTCRegistration) registerRemoteTrack(track *webrtc.TrackRemote) {
	if r == nil || track == nil {
		return
	}

	codec := track.Codec()
	switch {
	case track.Kind() == webrtc.RTPCodecTypeAudio && strings.EqualFold(codec.MimeType, MediaStreamOpus):
		go func() {
			if err := r.forwardWebRTCAudioTrackToPeerOpus(track); err != nil && !errors.Is(err, context.Canceled) {
				slog.Debug("gizclaw: forward webrtc opus track failed", "error", err)
			}
		}()
	default:
		go func() {
			drainWebRTCRemoteTrack(r.ctx, track)
		}()
	}
}

func (r *ClientWebRTCRegistration) forwardWebRTCAudioTrackToPeerOpus(track *webrtc.TrackRemote) error {
	if track == nil {
		return nil
	}
	conn := r.client.PeerConn()
	if conn == nil {
		return fmt.Errorf("gizclaw: client is not connected")
	}
	for {
		if err := r.ctx.Err(); err != nil {
			return err
		}

		packet, _, err := track.ReadRTP()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if len(packet.Payload) == 0 {
			continue
		}

		if _, err := conn.Write(giznet.ProtocolOpusPacket, packet.Payload); err != nil {
			return err
		}
	}
}

func (r *ClientWebRTCRegistration) forwardPeerOpusToWebRTCAudio() {
	packets, unsubscribe := r.client.subscribePeerPackets(giznet.ProtocolOpusPacket, 32)
	defer unsubscribe()

	var sequenceNumber uint16
	var rtpTimestamp uint32
	for {
		select {
		case <-r.ctx.Done():
			return
		case frame := <-packets:
			if len(frame) == 0 {
				continue
			}
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    webRTCOpusPayloadType,
					SequenceNumber: sequenceNumber,
					Timestamp:      rtpTimestamp,
				},
				Payload: append([]byte(nil), frame...),
			}
			if err := r.audioTrack.WriteRTP(packet); err != nil {
				slog.Debug("gizclaw: write webrtc opus rtp failed", "error", err)
				return
			}
			sequenceNumber++
			rtpTimestamp += webRTCOpusPacketRTPTicks(frame)
		}
	}
}

func (c *Client) callRPCRequest(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("rpc: nil request")
	}
	if req.Id == "" {
		return nil, fmt.Errorf("rpc: request id required")
	}
	stream, err := c.rpcConn()
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	return callRPC(ctx, stream, req)
}

func webRTCOpusPacketRTPTicks(packet []byte) uint32 {
	ticks := webRTCOpusFrameRTPTicks(packet)
	if ticks == 0 {
		return 960
	}
	return ticks * uint32(webRTCOpusPacketFrameCount(packet))
}

func webRTCOpusFrameRTPTicks(packet []byte) uint32 {
	if len(packet) == 0 {
		return 0
	}
	config := packet[0] >> 3
	switch {
	case config < 12:
		switch config % 4 {
		case 0:
			return 480
		case 1:
			return 960
		case 2:
			return 1920
		default:
			return 2880
		}
	case config < 16:
		if config%2 == 0 {
			return 480
		}
		return 960
	default:
		switch config % 4 {
		case 0:
			return 120
		case 1:
			return 240
		case 2:
			return 480
		default:
			return 960
		}
	}
}

func webRTCOpusPacketFrameCount(packet []byte) int {
	if len(packet) == 0 {
		return 1
	}
	switch packet[0] & 0x03 {
	case 0:
		return 1
	case 1, 2:
		return 2
	default:
		if len(packet) < 2 {
			return 1
		}
		count := int(packet[1] & 0x3f)
		if count < 1 {
			return 1
		}
		return count
	}
}

func drainWebRTCRTCP(sender *webrtc.RTPSender) {
	if sender == nil {
		return
	}
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func drainWebRTCRemoteTrack(ctx context.Context, track *webrtc.TrackRemote) {
	if track == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if _, _, err := track.ReadRTP(); err != nil {
			return
		}
	}
}
