package giztest

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestSpeechTranscribeRejectsUntypedInputBeforeRPC(t *testing.T) {
	step := Step{ID: "transcribe", Speech: &SpeechOperation{Method: "server.speech.transcribe"}}
	_, err := invokeSpeech(context.Background(), nil, step, map[string]any{}, "not audio", VariableSpec{}, VariableSpec{})
	if err == nil || !strings.Contains(err.Error(), "audio bytes") {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareTranscriptionRequestDerivesWireContentType(t *testing.T) {
	t.Run("decoded Ogg Opus", func(t *testing.T) {
		var encoded bytes.Buffer
		encoder, err := codecconv.NewPCMToOggOpusEncoder(&encoded, opus.SampleRate16K.Int(), 1, opus.ApplicationVoIP)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := encoder.Write(make([]byte, 640)); err != nil {
			t.Fatal(err)
		}
		if err := encoder.Close(); err != nil {
			t.Fatal(err)
		}

		audio, request, err := prepareTranscriptionRequest(encoded.Bytes(), VariableSpec{
			Type: "audio", MediaType: "audio/ogg", Codec: "opus", MaxBytes: 4096,
		}, rpcapi.SpeechTranscribeRequest{ModelName: "asr"})
		if err != nil {
			t.Fatalf("prepareTranscriptionRequest() error = %v", err)
		}
		if len(audio) == 0 {
			t.Fatal("prepareTranscriptionRequest() returned empty PCM")
		}
		if request.ContentType != speechPCM16ContentType {
			t.Fatalf("ContentType = %q, want %q", request.ContentType, speechPCM16ContentType)
		}
	})

	t.Run("pass through", func(t *testing.T) {
		input := []byte("pcm")
		audio, request, err := prepareTranscriptionRequest(input, VariableSpec{
			Type: "audio", MediaType: speechPCM16ContentType, Codec: "pcm_s16le",
		}, rpcapi.SpeechTranscribeRequest{ModelName: "asr"})
		if err != nil {
			t.Fatalf("prepareTranscriptionRequest() error = %v", err)
		}
		if !bytes.Equal(audio, input) {
			t.Fatalf("audio = %q, want %q", audio, input)
		}
		if request.ContentType != speechPCM16ContentType {
			t.Fatalf("ContentType = %q, want %q", request.ContentType, speechPCM16ContentType)
		}
	})
}

func TestPrepareTranscriptionRequestPropagatesConversionError(t *testing.T) {
	_, _, err := prepareTranscriptionRequest([]byte("not-ogg"), VariableSpec{
		Type: "audio", MediaType: "audio/ogg", Codec: "opus", MaxBytes: 1024,
	}, rpcapi.SpeechTranscribeRequest{ModelName: "asr"})
	if err == nil || !strings.Contains(err.Error(), "decode synthesized Ogg Opus for speech input") {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareTranscriptionRequestRejectsUnsupportedAudioBeforeRPC(t *testing.T) {
	tests := []struct {
		name string
		spec VariableSpec
		want string
	}{
		{
			name: "Opus without Ogg container",
			spec: VariableSpec{Type: "audio", MediaType: "audio/opus", Codec: "opus", MaxBytes: 1024},
			want: "Opus input media_type must be audio/ogg",
		},
		{
			name: "unsupported codec",
			spec: VariableSpec{Type: "audio", MediaType: "audio/mpeg", Codec: "mp3", MaxBytes: 1024},
			want: "input must be Ogg/Opus or 16 kHz mono PCM",
		},
		{
			name: "PCM with unsupported metadata",
			spec: VariableSpec{Type: "audio", MediaType: "audio/L16;rate=48000;channels=2", Codec: "pcm_s16le", MaxBytes: 1024},
			want: "input must be Ogg/Opus or 16 kHz mono PCM",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := prepareTranscriptionRequest([]byte("audio"), test.spec, rpcapi.SpeechTranscribeRequest{ModelName: "asr"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestSpeechFixtureCacheRunsOnceAndReturnsOwnedAudio(t *testing.T) {
	cache := newSpeechFixtureCache()
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	invoke := func() (operationResult, error) {
		calls.Add(1)
		once.Do(func() { close(started) })
		<-release
		return operationResult{saved: []byte("audio"), evidence: map[string]any{"method": "synthesize"}}, nil
	}

	const workers = 20
	results := make(chan operationResult, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			result, _, err := cache.Do(context.Background(), "fixture", invoke)
			if err != nil {
				t.Errorf("Do() error = %v", err)
				return
			}
			results <- result
		})
	}
	<-started
	close(release)
	wg.Wait()
	close(results)
	if got := calls.Load(); got != 1 {
		t.Fatalf("invoke calls = %d, want 1", got)
	}
	var audio [][]byte
	for result := range results {
		audio = append(audio, result.saved.([]byte))
	}
	if len(audio) != workers {
		t.Fatalf("results = %d, want %d", len(audio), workers)
	}
	audio[0][0] = 'A'
	if string(audio[1]) != "audio" {
		t.Fatalf("cached audio shares mutable storage: %q", audio[1])
	}
}

func TestSpeechFixtureCacheDoesNotRetainFailure(t *testing.T) {
	cache := newSpeechFixtureCache()
	var calls int
	invoke := func() (operationResult, error) {
		calls++
		if calls == 1 {
			return operationResult{}, errors.New("transient")
		}
		return operationResult{saved: []byte("audio")}, nil
	}
	if _, _, err := cache.Do(context.Background(), "fixture", invoke); err == nil {
		t.Fatal("first Do() unexpectedly passed")
	}
	if _, hit, err := cache.Do(context.Background(), "fixture", invoke); err != nil || hit {
		t.Fatalf("second Do() hit/error = %v/%v, want false/nil", hit, err)
	}
}
