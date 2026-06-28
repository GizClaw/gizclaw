package gizwebrtc

import (
	"fmt"

	"github.com/pion/datachannel"
)

type directPacket struct {
	protocol byte
	payload  []byte
}

func writePacket(raw datachannel.ReadWriteCloserDeadliner, protocol byte, payload []byte) (int, error) {
	if raw == nil {
		return 0, ErrPacketChannel
	}
	if 1+len(payload) > maxPacketMessageSize {
		return 0, ErrPacketTooLarge
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
	return directPacket{
		protocol: buf[0],
		payload:  append([]byte(nil), buf[1:n]...),
	}, nil
}
