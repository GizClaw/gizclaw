package dashscoperealtime

import (
	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/timestretch"
)

const (
	minSpeechRatePercent    = 50
	normalSpeechRatePercent = 100
	maxSpeechRatePercent    = 200

	// pcm16 output is 24 kHz mono (see getOutputAudioMIMEType).
	outputSampleRate = 24000
)

func speechRateStretches(percent int) bool {
	return percent != 0 && percent != normalSpeechRatePercent
}

func outputFormatStretchable(format string) bool {
	return format == "" || format == dashscope.AudioFormatPCM16
}

// withSpeechRatePercent time-stretches the PCM16 audio output to the given
// speaking rate.
func withSpeechRatePercent(percent int) option {
	return func(t *Transformer) {
		t.speechRatePercent = percent
	}
}

// dashScopeSpeechRate stretches one response audio route at a time. Responses
// are produced sequentially, so audio for a new response discards the state of
// an earlier response that was interrupted before its audio.done event.
type dashScopeSpeechRate struct {
	speed     float64
	streamID  string
	stretcher *timestretch.Stretcher
}

func newDashScopeSpeechRate(percent int) *dashScopeSpeechRate {
	if !speechRateStretches(percent) {
		return nil
	}
	return &dashScopeSpeechRate{speed: float64(percent) / normalSpeechRatePercent}
}

// audio returns the stretched audio that is final for streamID so far.
func (r *dashScopeSpeechRate) audio(streamID string, pcm []byte) ([]byte, error) {
	if r.stretcher == nil || r.streamID != streamID {
		stretcher, err := timestretch.New(outputSampleRate, 1, r.speed)
		if err != nil {
			return nil, err
		}
		r.streamID, r.stretcher = streamID, stretcher
	}
	return r.stretcher.Write(pcm), nil
}

// flush returns the remaining stretched audio of streamID and releases it.
func (r *dashScopeSpeechRate) flush(streamID string) []byte {
	if r.stretcher == nil || r.streamID != streamID {
		return nil
	}
	tail := r.stretcher.Flush()
	r.streamID, r.stretcher = "", nil
	return tail
}
