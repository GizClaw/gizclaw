package sfu

import (
	"errors"
	"time"
)

// ErrInvalidOpusPacket reports a packet whose TOC header cannot be parsed.
var ErrInvalidOpusPacket = errors.New("sfu: invalid Opus packet")

// opusMaxPacketDuration is the RFC 6716 upper bound for one Opus packet.
const opusMaxPacketDuration = 120 * time.Millisecond

// OpusPacketDuration returns the playback duration encoded by the packet's
// TOC byte (RFC 6716 section 3.1). The connector forwards raw Opus frames
// without decoding, so the duration drives the LiveKit sample clock.
func OpusPacketDuration(packet []byte) (time.Duration, error) {
	if len(packet) == 0 {
		return 0, ErrInvalidOpusPacket
	}
	toc := packet[0]
	frameDuration := opusFrameDuration(toc >> 3)
	var frames int
	switch toc & 0x03 {
	case 0:
		frames = 1
	case 1, 2:
		frames = 2
	default:
		if len(packet) < 2 {
			return 0, ErrInvalidOpusPacket
		}
		frames = int(packet[1] & 0x3F)
		if frames == 0 {
			return 0, ErrInvalidOpusPacket
		}
	}
	duration := frameDuration * time.Duration(frames)
	if duration > opusMaxPacketDuration {
		return 0, ErrInvalidOpusPacket
	}
	return duration, nil
}

func opusFrameDuration(config byte) time.Duration {
	switch {
	case config < 12:
		// SILK-only: 10, 20, 40, 60 ms.
		switch config & 0x03 {
		case 0:
			return 10 * time.Millisecond
		case 1:
			return 20 * time.Millisecond
		case 2:
			return 40 * time.Millisecond
		default:
			return 60 * time.Millisecond
		}
	case config < 16:
		// Hybrid: 10 or 20 ms.
		if config&0x01 == 0 {
			return 10 * time.Millisecond
		}
		return 20 * time.Millisecond
	default:
		// CELT-only: 2.5, 5, 10, 20 ms.
		switch config & 0x03 {
		case 0:
			return 2500 * time.Microsecond
		case 1:
			return 5 * time.Millisecond
		case 2:
			return 10 * time.Millisecond
		default:
			return 20 * time.Millisecond
		}
	}
}
