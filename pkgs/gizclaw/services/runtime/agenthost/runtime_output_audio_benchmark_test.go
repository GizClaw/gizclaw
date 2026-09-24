package agenthost

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
)

type benchmarkAudioTracks struct{ mixer *pcm.Mixer }

func (c benchmarkAudioTracks) CreateAudioTrack(opts ...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	return c.mixer.CreateTrack(opts...)
}

func (c benchmarkAudioTracks) AudioOutputFormat() pcm.Format { return c.mixer.Output() }

// BenchmarkPeerSpeech includes Ogg decode, track buffering, resampling and
// mixing into the peer's 16 kHz mono format. It excludes Opus encoding/network
// pacing. Each fixture contains six seconds of speech, shorter than the track
// buffer, so draining after input does not measure producer backpressure.
func BenchmarkPeerSpeech(b *testing.B) {
	dir := os.Getenv("GIZCLAW_RESAMPLER_SPEECH_DIR")
	if dir == "" {
		b.Skip("set GIZCLAW_RESAMPLER_SPEECH_DIR to six-second speech fixtures")
	}
	for _, tc := range []struct{ name, file, mime string }{
		{"Ogg", "peer.ogg", "audio/ogg"},
		{"PCM16", "16000-1.pcm", "audio/L16; rate=16000; channels=1"},
		{"PCM24", "24000-1.pcm", "audio/L16; rate=24000; channels=1"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(filepath.Join(dir, tc.file))
			if err != nil {
				b.Fatal(err)
			}
			buf := make([]byte, 16000*2*60/1000)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				mixer := pcm.NewMixer(pcm.L16Mono16K)
				tracks := newAudioOutputTracks(benchmarkAudioTracks{mixer})
				for offset := 0; offset < len(data); offset += 1024 {
					end := min(offset+1024, len(data))
					if err := tracks.consume(pcmOutputChunk("speech", tc.mime, data[offset:end], false, "")); err != nil {
						b.Fatal(err)
					}
				}
				if err := tracks.closeWrite(); err != nil {
					b.Fatal(err)
				}
				if err := mixer.CloseWrite(); err != nil {
					b.Fatal(err)
				}
				var total int
				for {
					n, err := mixer.Read(buf)
					total += n
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
				}
				if total < 16000*2*5 {
					b.Fatalf("decoded only %d bytes from six seconds of speech", total)
				}
				if err := mixer.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/6e6, "ms/audio-s")
		})
	}
}

// BenchmarkDoubaoRealtimeDownlink compares 24 kHz Ogg/Opus input with 16 kHz
// PCM input from the same speech through decode, mixing,
// and 20 ms downlink Opus encoding.
// The fixtures must contain the same six seconds of speech.
func BenchmarkDoubaoRealtimeDownlink(b *testing.B) {
	dir := os.Getenv("GIZCLAW_RESAMPLER_SPEECH_DIR")
	if dir == "" {
		b.Skip("set GIZCLAW_RESAMPLER_SPEECH_DIR to six-second speech fixtures")
	}
	pcmData, err := os.ReadFile(filepath.Join(dir, "16000-1.pcm"))
	if err != nil {
		b.Fatal(err)
	}
	oggData, err := os.ReadFile(filepath.Join(dir, "peer.ogg"))
	if err != nil {
		b.Fatal(err)
	}
	var opusPackets [][]byte
	for packet, err := range ogg.Packets(bytes.NewReader(oggData)) {
		if err != nil {
			b.Fatal(err)
		}
		if codecconv.IsOpusHeadPacket(packet.Data) || codecconv.IsOpusTagsPacket(packet.Data) || len(packet.Data) == 0 {
			continue
		}
		opusPackets = append(opusPackets, packet.Data)
	}
	if len(opusPackets) == 0 {
		b.Fatal("Ogg fixture contains no Opus audio packets")
	}
	for _, tc := range []struct {
		name string
		opus bool
	}{
		{name: "Opus24", opus: true},
		{name: "PCM16"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				mixer := pcm.NewMixer(pcm.L16Mono16K)
				tracks := newAudioOutputTracks(benchmarkAudioTracks{mixer})
				if tc.opus {
					for _, packet := range opusPackets {
						if err := tracks.consume(pcmOutputChunk("speech", "audio/opus", packet, false, "")); err != nil {
							b.Fatal(err)
						}
					}
				} else {
					for offset := 0; offset < len(pcmData); offset += 1024 {
						end := min(offset+1024, len(pcmData))
						if err := tracks.consume(pcmOutputChunk("speech", "audio/L16; rate=16000; channels=1", pcmData[offset:end], false, "")); err != nil {
							b.Fatal(err)
						}
					}
				}
				if err := tracks.closeWrite(); err != nil {
					b.Fatal(err)
				}
				if err := mixer.CloseWrite(); err != nil {
					b.Fatal(err)
				}
				encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
				if err != nil {
					b.Fatal(err)
				}
				if err := encoder.SetComplexity(2); err != nil {
					b.Fatal(err)
				}
				frame := make([]byte, 640)
				samples := make([]int16, 320)
				var frames int
				for {
					_, err := io.ReadFull(mixer, frame)
					if err == io.EOF || err == io.ErrUnexpectedEOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
					for index := range samples {
						samples[index] = int16(binary.LittleEndian.Uint16(frame[index*2:]))
					}
					if _, err := encoder.Encode(samples, len(samples)); err != nil {
						b.Fatal(err)
					}
					frames++
				}
				if frames < 250 {
					b.Fatalf("encoded only %d frames from six seconds of speech", frames)
				}
				if err := encoder.Close(); err != nil {
					b.Fatal(err)
				}
				if err := mixer.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/6e6, "ms/audio-s")
		})
	}
}
