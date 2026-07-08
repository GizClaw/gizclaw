package gizcli

import (
	"fmt"
	"io"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const peerStreamEventVersion = 1

// DialPeerEventStream opens the reliable bidirectional agent stream event channel.
func (c *Client) DialPeerEventStream() (net.Conn, error) {
	if c == nil {
		return nil, fmt.Errorf("gizclaw: nil client")
	}
	conn := c.PeerConn()
	if conn == nil {
		return nil, fmt.Errorf("gizclaw: client is not connected")
	}
	stream, err := conn.Dial(EventStreamAgent)
	if err != nil {
		return nil, fmt.Errorf("gizclaw: dial peer event stream: %w", err)
	}
	return stream, nil
}

// ReadPeerStreamEvent reads one framed peer stream event.
func ReadPeerStreamEvent(r io.Reader) (apitypes.PeerStreamEvent, error) {
	frame, err := rpcapi.ReadFrame(r)
	if err != nil {
		return apitypes.PeerStreamEvent{}, err
	}
	if frame.Type == rpcapi.FrameTypeEOS {
		return apitypes.PeerStreamEvent{}, io.EOF
	}
	var event apitypes.PeerStreamEvent
	if err := rpcapi.DecodeJSONFrame(frame, &event); err != nil {
		return apitypes.PeerStreamEvent{}, fmt.Errorf("gizclaw: decode peer stream event: %w", err)
	}
	return event, nil
}

// WritePeerStreamEvent writes one framed peer stream event.
func WritePeerStreamEvent(w io.Writer, event apitypes.PeerStreamEvent) error {
	if event.V == 0 {
		event.V = peerStreamEventVersion
	}
	frame, err := rpcapi.NewJSONFrame(event)
	if err != nil {
		return err
	}
	return rpcapi.WriteFrame(w, frame)
}
