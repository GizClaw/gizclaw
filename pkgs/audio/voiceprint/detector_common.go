package voiceprint

import (
	"bytes"
	"fmt"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
)

func prepareDetectorAudio(audio []byte, format pcm.Format) ([]byte, error) {
	if len(audio) == 0 {
		return nil, nil
	}
	if len(audio)%2 != 0 {
		return nil, fmt.Errorf("voiceprint: detector expects PCM16 audio bytes, got odd length %d", len(audio))
	}
	if format == pcm.L16Mono16K {
		return audio, nil
	}

	rs, err := resampler.New(
		bytes.NewReader(audio),
		resampler.Format{
			SampleRate: format.SampleRate(),
			Stereo:     format.Channels() == 2,
		},
		resampler.Format{
			SampleRate: pcm.L16Mono16K.SampleRate(),
			Stereo:     false,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("voiceprint: create detector resampler: %w", err)
	}
	defer func() {
		_ = rs.Close()
	}()

	out, err := io.ReadAll(rs)
	if err != nil {
		return nil, fmt.Errorf("voiceprint: resample detector audio: %w", err)
	}
	return out, nil
}

func copyEmbedding(emb []float32) []float32 {
	if len(emb) == 0 {
		return nil
	}
	out := make([]float32, len(emb))
	copy(out, emb)
	return out
}

func cosineSimilarity(a, b []float32) float32 {
	n := min(len(b), len(a))
	if n == 0 {
		return 0
	}
	var sum float32
	for i := range n {
		sum += a[i] * b[i]
	}
	return sum
}
