package gizwebrtc

import (
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/datachannel"
)

var errPacketProtocolIgnored = errors.New("gizwebrtc: packet protocol ignored")

type directPacket struct {
	protocol byte
	payload  []byte
}

func writePacket(raw datachannel.ReadWriteCloserDeadliner, protocol byte, payload []byte) (int, error) {
	if err := validatePacketProtocol(protocol); err != nil {
		return 0, err
	}
	if raw == nil {
		return 0, ErrPacketChannel
	}
	if len(payload) > maxPacketMessageSize-1 {
		return 0, giznet.ErrPacketTooLarge
	}
	msg := make([]byte, 1+len(payload))
	msg[0] = protocol
	copy(msg[1:], payload)
	if _, err := raw.WriteDataChannel(msg, false); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func readPacket(raw datachannel.ReadWriteCloserDeadliner) (directPacket, error) {
	buf := make([]byte, maxPacketMessageSize)
	n, _, err := raw.ReadDataChannel(buf)
	if err != nil {
		return directPacket{}, err
	}
	if n < 1 {
		return directPacket{}, fmt.Errorf("gizwebrtc: empty packet message")
	}
	protocol := buf[0]
	if protocol == giznet.ProtocolOpusPacket ||
		(protocol < 0x40 && protocol != giznet.ProtocolTunnelPacket) {
		return directPacket{}, errPacketProtocolIgnored
	}
	return directPacket{
		protocol: protocol,
		payload:  append([]byte(nil), buf[1:n]...),
	}, nil
}

func validatePacketProtocol(protocol byte) error {
	if protocol == giznet.ProtocolOpusPacket || protocol == giznet.ProtocolTunnelPacket {
		return nil
	}
	if protocol < 0x40 {
		return giznet.ErrPacketProtocol
	}
	return nil
}
