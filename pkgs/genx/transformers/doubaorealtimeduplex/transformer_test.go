package doubaorealtimeduplex

import (
	"context"
	"sync"
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/agentkit"
)

func TestNew(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New(Config{}) succeeded without a client")
	}
	transformer, err := New(Config{Client: doubaospeech.NewClient("")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if transformer == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNewCopiesConfigAndBuildsConfiguredDelegate(t *testing.T) {
	transcode := false
	speed := 1
	loudness := 2
	extension := &doubaospeech.RealtimeDuplexExtension{}
	transformer, err := New(Config{
		Client:          doubaospeech.NewClient(""),
		Speaker:         "speaker",
		Format:          "ogg_opus",
		SampleRate:      24000,
		InputFormat:     "speech_opus",
		InputSampleRate: 16000,
		InputChannels:   1,
		InputTranscode:  &transcode,
		Model:           "model",
		SessionID:       "session",
		Instructions:    "instructions",
		OutputSpeed:     &speed,
		OutputLoudness:  &loudness,
		Extension:       extension,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	transcode = true
	speed = 9
	if transformer.config.InputTranscode == nil || *transformer.config.InputTranscode {
		t.Fatal("New() retained caller-owned InputTranscode pointer")
	}
	if transformer.config.OutputSpeed == nil || *transformer.config.OutputSpeed != 1 {
		t.Fatal("New() retained caller-owned OutputSpeed pointer")
	}
	if transformer.config.Extension == extension {
		t.Fatal("New() retained caller-owned Extension pointer")
	}
	if transformer.delegate() == nil {
		t.Fatal("delegate() returned nil")
	}
}

func TestTransformerConcurrentInvocationsUseIndependentResponses(t *testing.T) {
	transformer := &Transformer{newDelegate: func() genx.Transformer { return concurrentDelegate{} }}
	assertConcurrentResponses(t, transformer)
}

type concurrentDelegate struct{}

func (concurrentDelegate) Transform(context.Context, genx.Stream) (genx.Stream, error) {
	output := agentkit.NewOutput(agentkit.OutputConfig{})
	_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("response"), Ctrl: &genx.StreamCtrl{StreamID: "provider-response"}})
	_ = output.Close()
	return output, nil
}

func assertConcurrentResponses(t *testing.T, transformer *Transformer) {
	t.Helper()
	const count = 8
	ids := make(chan string, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			output, err := transformer.Transform(context.Background(), nil)
			if err != nil {
				t.Errorf("Transform() error = %v", err)
				return
			}
			chunk, err := output.Next()
			if err != nil {
				t.Errorf("Next() error = %v", err)
				return
			}
			ids <- chunk.Ctrl.StreamID
		})
	}
	wg.Wait()
	close(ids)
	seen := make(map[string]bool, count)
	for id := range ids {
		if id == "" || seen[id] {
			t.Fatalf("response StreamID %q is empty or reused", id)
		}
		seen[id] = true
	}
}
