package audiostream

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestDurationMeterPCMPartialSamples(t *testing.T) {
	for _, rate := range []int{16000, 24000, 44100} {
		meter := NewDurationMeter("audio/pcm", rate, 2)
		var duration time.Duration
		for range rate * 4 {
			duration += meter.Add([]byte{1})
		}
		if duration != time.Second || meter.pcmPending != 0 {
			t.Fatalf("%d Hz bytewise PCM duration = %s, pending=%d", rate, duration, meter.pcmPending)
		}
	}
}

func TestDurationMeterMP3Frames(t *testing.T) {
	for _, tc := range []struct {
		header  []byte
		samples int
		rate    int
	}{
		{[]byte{0xff, 0xfb, 0x90, 0}, 1152, 44100},
		{[]byte{0xff, 0xf3, 0x80, 0}, 576, 22050},
		{[]byte{0xff, 0xe3, 0x80, 0}, 576, 11025},
	} {
		size, samples, rate := mp3FrameTiming(tc.header)
		if samples != tc.samples || rate != tc.rate || size < 4 {
			t.Fatalf("frame timing = %d bytes, %d samples, %d Hz", size, samples, rate)
		}
		frame := make([]byte, size)
		copy(frame, tc.header)
		data := bytes.Repeat(frame, 3)
		for _, width := range []int{1, 3, 200, len(data)} {
			meter := NewDurationMeter("audio/mpeg", 0, 0)
			var duration time.Duration
			for start := 0; start < len(data); start += width {
				duration += meter.Add(data[start:min(start+width, len(data))])
			}
			if want := 3 * (time.Duration(samples) * time.Second / time.Duration(rate)); duration != want {
				t.Fatalf("%d Hz MP3 width=%d duration=%s, want %s", rate, width, duration, want)
			}
		}
	}
}

func TestDurationMeterOggOpusHeadersTrimAndChaining(t *testing.T) {
	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8], head[9] = 1, 1
	binary.LittleEndian.PutUint16(head[10:12], 312)
	data := durationOggPage(1, 2, 0, head)
	data = append(data, durationOggPage(1, 0, 0, []byte("OpusTags"))...)
	data = append(data, durationOggPage(1, 0, 960, []byte{0xf8})...)
	data = append(data, durationOggPage(1, 4, 1800, []byte{0xf8})...)
	binary.LittleEndian.PutUint16(head[10:12], 0)
	data = append(data, durationOggPage(2, 2, 0, head)...)
	data = append(data, durationOggPage(2, 4, 960, []byte{0xf8})...)
	for _, width := range []int{1, 3, 17, 64, len(data)} {
		meter := NewDurationMeter("audio/ogg", 0, 0)
		var duration time.Duration
		for start := 0; start < len(data); start += width {
			duration += meter.Add(data[start:min(start+width, len(data))])
		}
		if duration != 51*time.Millisecond {
			t.Fatalf("Ogg width=%d duration=%s, want 51ms", width, duration)
		}
	}
}

func TestDurationMeterOggOpusContinuationAndInvalidGranule(t *testing.T) {
	head := make([]byte, 19)
	copy(head, "OpusHead")
	packet := append([]byte{0xf8}, make([]byte, 255)...)
	data := durationOggPage(1, 2, 0, head)
	data = append(data, durationOggPage(1, 0, ^uint64(0), packet[:255])...)
	data = append(data, durationOggPage(1, 1, 960, packet[255:])...)
	data = append(data, durationOggPage(1, 4, 1<<60, []byte{0xf8})...)
	meter := NewDurationMeter("audio/ogg", 0, 0)
	if duration := meter.Add(data); duration != 40*time.Millisecond {
		t.Fatalf("continued packets or untrusted clock invented duration: %s", duration)
	}
}

func TestDurationMeterBoundsMalformedData(t *testing.T) {
	for _, mime := range []string{"audio/ogg", "audio/mpeg", "audio/flac", "audio/pcm"} {
		meter := NewDurationMeter(mime, 0, 0)
		if duration := meter.Add(bytes.Repeat([]byte{0xff}, 2<<20)); duration != 0 || len(meter.pending) > durationPendingLimit {
			t.Fatalf("%s malformed duration=%s, retained=%d", mime, duration, len(meter.pending))
		}
	}
}

// Page envelopes exercise timing only; decoder-owned checksum validation is
// covered by codec tests and the MiniMax protocol integration test.
func durationOggPage(serial uint32, flags byte, granule uint64, packet []byte) []byte {
	laces := (len(packet) + 254) / 255
	header := make([]byte, 27+laces)
	copy(header, "OggS")
	header[5], header[26] = flags, byte(laces)
	binary.LittleEndian.PutUint64(header[6:14], granule)
	binary.LittleEndian.PutUint32(header[14:18], serial)
	for i := range laces {
		header[27+i] = byte(min(255, len(packet)-i*255))
	}
	return append(header, packet...)
}
