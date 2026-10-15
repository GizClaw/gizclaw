package audiostream

import (
	"bytes"
	"encoding/binary"
	"time"
)

const durationPendingLimit = 27 + 255 + 255*255 // one maximum-size Ogg page

// DurationMeter measures newly available media in normalized PCM16LE, MP3
// Layer III or Ogg/Opus output without decoding audio. It retains partial frames
// across chunks and ignores container headers. Create one per synthesis request;
// Add calls must be serial. Unsupported or unrecognized data yields zero.
type DurationMeter struct {
	mime       string
	pcmRate    int
	pcmFrame   int
	pcmPending int
	fraction   int64
	pending    []byte
	ogg        oggDurationState
}

// NewDurationMeter configures a request's output format. sampleRate and channels
// describe raw signed 16-bit little-endian PCM; encoded formats carry their own
// timing. Measuring duration does not replace downstream format validation.
func NewDurationMeter(mimeType string, sampleRate, channels int) *DurationMeter {
	return &DurationMeter{mime: normalizeMIME(mimeType), pcmRate: sampleRate, pcmFrame: channels * 2}
}

// Add returns the duration of complete media newly made available by data.
// Incomplete frames and metadata do not create playback budget.
func (m *DurationMeter) Add(data []byte) time.Duration {
	if m.mime == "audio/pcm" || m.mime == "audio/l16" {
		if m.pcmRate <= 0 || m.pcmFrame <= 0 {
			return 0
		}
		size := m.pcmPending + len(data)
		frames := size / m.pcmFrame
		m.pcmPending = size % m.pcmFrame
		numerator := int64(frames)*int64(time.Second) + m.fraction
		m.fraction = numerator % int64(m.pcmRate)
		return time.Duration(numerator / int64(m.pcmRate))
	}
	var drain func() time.Duration
	switch m.mime {
	case "audio/mpeg", "audio/mp3", "audio/x-mpeg":
		drain = m.drainMP3
	case "audio/ogg":
		drain = m.drainOgg
	default:
		return 0
	}
	var duration time.Duration
	for len(data) > 0 {
		count := min(len(data), durationPendingLimit-len(m.pending))
		m.pending = append(m.pending, data[:count]...)
		data = data[count:]
		duration += drain()
		if len(m.pending) == durationPendingLimit {
			// Malformed input cannot retain an unbounded prefix or stop progress.
			m.pending = m.pending[1:]
		}
	}
	return duration
}

func (m *DurationMeter) drainMP3() time.Duration {
	var duration time.Duration
	for len(m.pending) >= 4 {
		size, samples, rate := mp3FrameTiming(m.pending)
		if size == 0 {
			m.pending = m.pending[1:]
			continue
		}
		if len(m.pending) < size {
			break
		}
		duration += time.Duration(samples) * time.Second / time.Duration(rate)
		m.pending = m.pending[size:]
	}
	return duration
}

func mp3FrameTiming(header []byte) (size, samples, rate int) {
	if header[0] != 0xff || header[1]&0xe0 != 0xe0 || header[1]&6 != 2 || header[3]&3 == 2 {
		return 0, 0, 0
	}
	version, index, frequency := header[1]>>3&3, header[2]>>4, header[2]>>2&3
	if version == 1 || index == 0 || index == 15 || frequency == 3 {
		return 0, 0, 0
	}
	rate = [...]int{44100, 48000, 32000}[frequency]
	bitrates := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	samples, coefficient := 1152, 144
	if version != 3 {
		bitrates = [...]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
		rate /= 2
		if version == 0 {
			rate /= 2
		}
		samples, coefficient = 576, 72
	}
	return coefficient*bitrates[index]*1000/rate + int(header[2]>>1&1), samples, rate
}

type oggDurationState struct {
	serial       uint32
	active       bool
	opus         bool
	preSkip      uint64
	decoded      uint64
	measured     uint64
	packetPrefix [19]byte
	packetBytes  int
}

func (m *DurationMeter) drainOgg() time.Duration {
	var duration time.Duration
	for len(m.pending) >= 27 {
		index := bytes.Index(m.pending, []byte("OggS"))
		if index < 0 {
			m.pending = m.pending[len(m.pending)-3:]
			break
		}
		m.pending = m.pending[index:]
		if len(m.pending) < 27 {
			break
		}
		if m.pending[4] != 0 {
			m.pending = m.pending[1:]
			continue
		}
		header := 27 + int(m.pending[26])
		if len(m.pending) < header {
			break
		}
		size := header
		for _, lace := range m.pending[27:header] {
			size += int(lace)
		}
		if len(m.pending) < size {
			break
		}
		duration += m.ogg.page(m.pending[:size], header)
		m.pending = m.pending[size:]
	}
	return duration
}

func (s *oggDurationState) page(page []byte, header int) time.Duration {
	serial := binary.LittleEndian.Uint32(page[14:18])
	if page[5]&2 != 0 {
		*s = oggDurationState{serial: serial, active: true}
	}
	if !s.active || serial != s.serial {
		return 0
	}
	offset := header
	for _, lace := range page[27:header] {
		count := int(lace)
		if s.packetBytes < len(s.packetPrefix) {
			copy(s.packetPrefix[s.packetBytes:], page[offset:offset+count])
		}
		s.packetBytes += count
		offset += count
		if lace < 255 {
			s.packet()
			s.packetBytes = 0
			s.packetPrefix = [19]byte{}
		}
	}
	// RFC 7845: Opus granules use 48 kHz, include pre-skip and may trim the
	// final packet. Clamp granules to parsed packet duration so malformed clock
	// values cannot invent arbitrarily large budgets.
	granule := binary.LittleEndian.Uint64(page[6:14])
	if granule == ^uint64(0) || !s.opus {
		return 0
	}
	end := min(granule, s.decoded)
	if end <= s.preSkip+s.measured {
		return 0
	}
	available := end - s.preSkip
	duration := time.Duration(available-s.measured) * time.Second / 48000
	s.measured = available
	return duration
}

func (s *oggDurationState) packet() {
	if s.packetBytes >= 19 && string(s.packetPrefix[:8]) == "OpusHead" {
		s.opus = true
		s.preSkip = uint64(binary.LittleEndian.Uint16(s.packetPrefix[10:12]))
		return
	}
	if !s.opus || s.packetBytes == 0 || s.packetBytes >= 8 && string(s.packetPrefix[:8]) == "OpusTags" {
		return
	}
	config := s.packetPrefix[0] >> 3
	samples := uint64(480 << (config & 3))
	if config >= 16 {
		samples = 120 << (config & 3)
	} else if config >= 12 {
		samples = 480 << (config & 1)
	} else if config&3 == 3 {
		samples = 2880
	}
	frames := uint64(1)
	switch s.packetPrefix[0] & 3 {
	case 1, 2:
		frames = 2
	case 3:
		if s.packetBytes < 2 {
			return
		}
		frames = uint64(s.packetPrefix[1] & 63)
	}
	if samples*frames <= 5760 {
		s.decoded += samples * frames
	}
}
