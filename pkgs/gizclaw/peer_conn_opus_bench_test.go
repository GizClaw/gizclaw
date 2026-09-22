//go:build cgo && (darwin || linux)

package gizclaw

import (
	"encoding/binary"
	"os"
	"strconv"
	"syscall"
	"testing"
)

// BenchmarkPeerConnOpusComplexity measures the production encoder using an
// external speech corpus: signed little-endian PCM16, 16 kHz mono, whole 20 ms
// frames. GIZCLAW_OPUS_BENCH_PCM must point to that corpus. Load and convert it
// before timing; keep the same continuous encoder state as the mixed downlink.
func BenchmarkPeerConnOpusComplexity(b *testing.B) {
	path := os.Getenv("GIZCLAW_OPUS_BENCH_PCM")
	if path == "" {
		b.Skip("set GIZCLAW_OPUS_BENCH_PCM to a 16 kHz mono PCM16 speech corpus")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	frameSize := int(peerConnMixerFormat.SamplesInDuration(peerConnOpusFrameDuration))
	if len(raw) == 0 || len(raw)%(2*frameSize) != 0 {
		b.Fatal("speech corpus must contain whole 20 ms PCM16 frames")
	}
	samples := make([]int16, len(raw)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	for _, complexity := range []int{0, 1, 2, 3, 4, 5, 6, 8, 10} {
		b.Run(strconv.Itoa(complexity), func(b *testing.B) {
			enc, err := newPeerConnOpusEncoder()
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = enc.Close() }()
			if err := enc.SetComplexity(complexity); err != nil {
				b.Fatal(err)
			}
			var before, after syscall.Rusage
			if err := syscall.Getrusage(syscall.RUSAGE_SELF, &before); err != nil {
				b.Fatal(err)
			}
			offset, bytes := 0, int64(0)
			for b.Loop() {
				packet, err := enc.Encode(samples[offset:offset+frameSize], frameSize)
				if err != nil {
					b.Fatal(err)
				}
				bytes += int64(len(packet))
				offset += frameSize
				if offset == len(samples) {
					offset = 0
				}
			}
			if err := syscall.Getrusage(syscall.RUSAGE_SELF, &after); err != nil {
				b.Fatal(err)
			}
			cpuNS := after.Utime.Nano() + after.Stime.Nano() - before.Utime.Nano() - before.Stime.Nano()
			audioSeconds := float64(b.N) * peerConnOpusFrameDuration.Seconds()
			b.ReportMetric(float64(cpuNS)/float64(b.N), "cpu-ns/frame")
			b.ReportMetric(float64(cpuNS)/1e6/audioSeconds, "cpu-ms/audio-s")
			b.ReportMetric(float64(bytes)/audioSeconds, "payload-B/audio-s")
		})
	}
}
