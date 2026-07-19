package dashscoperealtime

import (
	"context"
	"sync"
	"testing"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/agentkit"
)

func TestNew(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New(Config{}) succeeded without a client")
	}
	transformer, err := New(Config{Client: dashscope.NewClient("")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if transformer == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNewCopiesConfigAndBuildsConfiguredDelegate(t *testing.T) {
	temperature := 0.5
	maxTokens := 10
	enableASR := false
	modalities := []string{"text", "audio"}
	turnDetection := &dashscope.TurnDetection{Type: "server_vad"}
	transformer, err := New(Config{
		Client:            dashscope.NewClient(""),
		Model:             "model",
		Voice:             "voice",
		Instructions:      "instructions",
		Modalities:        modalities,
		VAD:               "server_vad",
		Temperature:       &temperature,
		MaxOutputTokens:   &maxTokens,
		EnableASR:         &enableASR,
		ASRModel:          "asr-model",
		TurnDetection:     turnDetection,
		InputAudioFormat:  "pcm16",
		OutputAudioFormat: "pcm16",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	modalities[0] = "changed"
	temperature = 1
	turnDetection.Type = "changed"
	if transformer.config.Modalities[0] != "text" {
		t.Fatal("New() retained caller-owned Modalities slice")
	}
	if transformer.config.Temperature == nil || *transformer.config.Temperature != 0.5 {
		t.Fatal("New() retained caller-owned Temperature pointer")
	}
	if transformer.config.TurnDetection == nil || transformer.config.TurnDetection.Type != "server_vad" {
		t.Fatal("New() retained caller-owned TurnDetection pointer")
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
