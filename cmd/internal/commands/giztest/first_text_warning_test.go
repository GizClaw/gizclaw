package giztestcmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func TestFirstTextWarningWaitsForRequiredContent(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantBlob("reply", testAudibleOpus(t), false)
		time.Sleep(20 * time.Millisecond)
		stream.in <- assistantText("reply", "answer", false)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := invokeFakePeerStream(ctx, giztest.PeerStreamOperation{
		Mode: "text", Completion: "first_response", FirstTextTimeout: "5ms",
		FirstTextTimeoutSeverity: "warning", FirstAudioTimeout: "500ms",
	}, stream)
	if err != nil {
		t.Fatal(err)
	}
	warnings, _ := result.evidence["warnings"].([]string)
	if len(warnings) != 1 || result.evidence["first_text_ms"].(int64) < 5 {
		t.Fatalf("late text evidence = %#v", result.evidence)
	}
}

func TestFirstTextWarningDoesNotAcceptMissingContent(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantBlob("reply", testAudibleOpus(t), false)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	result, err := invokeFakePeerStream(ctx, giztest.PeerStreamOperation{
		Mode: "text", Completion: "first_response", FirstTextTimeout: "5ms",
		FirstTextTimeoutSeverity: "warning", FirstAudioTimeout: "500ms",
	}, stream)
	if !errors.Is(err, context.DeadlineExceeded) || result.evidence["deadline"] != "timeout" {
		t.Fatalf("missing text result=%#v error=%v", result, err)
	}
	if warnings, _ := result.evidence["warnings"].([]string); len(warnings) != 1 {
		t.Fatalf("missing text warning = %#v", result.evidence)
	}
	progress := result.evidence["response_progress"].(map[string]any)
	reply := progress["reply"].(map[string]bool)
	if reply["text"] || !reply["audio"] || reply["text_eos"] || reply["audio_eos"] {
		t.Fatalf("incomplete reply progress = %#v", progress)
	}
}

func TestFirstTextWarningKeepsAudioDeadlineFatal(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantText("reply", "answer", false)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := invokeFakePeerStream(ctx, giztest.PeerStreamOperation{
		Mode: "text", Completion: "first_response", FirstTextTimeout: "5ms",
		FirstTextTimeoutSeverity: "warning", FirstAudioTimeout: "20ms",
	}, stream)
	if !errors.Is(err, context.DeadlineExceeded) || result.evidence["deadline"] != "first_audio_timeout" {
		t.Fatalf("missing audio result=%#v error=%v", result, err)
	}
}
