package agenthost

import (
	"io"
	"os"
	"path/filepath"
	"testing"

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
