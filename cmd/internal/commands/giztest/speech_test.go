package giztest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSpeechTranscribeRejectsUntypedInputBeforeRPC(t *testing.T) {
	step := Step{ID: "transcribe", Speech: &SpeechOperation{Method: "server.speech.transcribe"}}
	_, err := invokeSpeech(context.Background(), nil, step, map[string]any{}, "not audio", VariableSpec{}, VariableSpec{})
	if err == nil || !strings.Contains(err.Error(), "audio bytes") {
		t.Fatalf("error = %v", err)
	}
}

func TestTranscriptionInputRequiresPCMLabelForDecodedOpus(t *testing.T) {
	spec := VariableSpec{Type: "audio", MediaType: "audio/ogg", Codec: "opus"}
	_, err := transcriptionInput([]byte("not-decoded"), spec, "audio/ogg")
	if err == nil || !strings.Contains(err.Error(), speechPCM16ContentType) {
		t.Fatalf("error = %v", err)
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
