package packet

// OpusDuration48 reports the TOC-derived duration in 48 kHz samples per channel.
// Empty payloads return zero. It does not validate the encoded frame bodies or
// enforce the maximum packet duration. A truncated code-3 packet or zero frame
// count is treated as one frame.
func OpusDuration48(payload []byte) uint32 {
	if len(payload) == 0 {
		return 0
	}
	config := payload[0] >> 3
	var ticks uint32
	switch {
	case config < 12:
		switch config % 4 {
		case 0:
			ticks = 480
		case 1:
			ticks = 960
		case 2:
			ticks = 1920
		default:
			ticks = 2880
		}
	case config < 16:
		if config%2 == 0 {
			ticks = 480
		} else {
			ticks = 960
		}
	default:
		switch config % 4 {
		case 0:
			ticks = 120
		case 1:
			ticks = 240
		case 2:
			ticks = 480
		default:
			ticks = 960
		}
	}
	var frames uint32
	switch payload[0] & 0x03 {
	case 0:
		frames = 1
	case 1, 2:
		frames = 2
	default:
		frames = 1
		if len(payload) >= 2 {
			frames = max(uint32(payload[1]&0x3f), 1)
		}
	}
	return ticks * frames
}
