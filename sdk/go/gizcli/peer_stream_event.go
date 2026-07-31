package gizcli

import (
	"fmt"
	"io"
	"net"

	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"google.golang.org/protobuf/proto"
)

// DialPeerEventStream opens a logical view of the connection-owned,
// bidirectional Peer Event stream.
func (c *Client) DialPeerEventStream() (net.Conn, error) {
	if c == nil {
		return nil, fmt.Errorf("gizclaw: nil client")
	}
	c.mu.RLock()
	events := c.events
	c.mu.RUnlock()
	if events == nil {
		return nil, fmt.Errorf("gizclaw: client is not connected")
	}
	return events.subscribe()
}

// ReadPeerStreamEvent reads one framed peer stream event.
func ReadPeerStreamEvent(r io.Reader) (*eventpb.PeerEvent, error) {
	frame, err := rpcapi.ReadFrame(r)
	if err != nil {
		return nil, err
	}
	if frame.Type == rpcapi.FrameTypeEOS {
		return nil, io.EOF
	}
	if frame.Type != rpcapi.FrameTypeBinary {
		return nil, fmt.Errorf("gizclaw: expected peer stream event binary frame, got type %d", frame.Type)
	}
	event := &eventpb.PeerEvent{}
	if err := proto.Unmarshal(frame.Payload, event); err != nil {
		return nil, fmt.Errorf("gizclaw: decode peer stream event: %w", err)
	}
	if err := event.ValidateReceived(); err != nil {
		return nil, fmt.Errorf("gizclaw: validate peer stream event: %w", err)
	}
	return event, nil
}

// WritePeerStreamEvent writes one framed peer stream event.
func WritePeerStreamEvent(w io.Writer, event *eventpb.PeerEvent) error {
	if event == nil {
		return fmt.Errorf("gizclaw: peer stream event is nil")
	}
	if event.Version == 0 {
		event = proto.Clone(event).(*eventpb.PeerEvent)
		event.Version = eventpb.Version
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("gizclaw: validate peer stream event: %w", err)
	}
	data, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("gizclaw: encode peer stream event: %w", err)
	}
	return rpcapi.WriteFrame(w, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: data})
}
