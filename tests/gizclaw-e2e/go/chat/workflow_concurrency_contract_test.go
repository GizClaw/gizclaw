//go:build gizclaw_e2e

package chat

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestWorkflowConcurrencyContract(t *testing.T) {
	t.Run("Eino concurrency fixture loads", func(t *testing.T) {
		serverKey, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair(server): %v", err)
		}
		clientKey, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair(client): %v", err)
		}
		contextConfigPath := filepath.Join(t.TempDir(), "config.yaml")
		writeSetupContextConfig(t, contextConfigPath, serverKey, clientKey, "")

		fixture := filepath.Join("..", "..", "testdata", "workspaces", "eino-concurrency.json")
		cfg, err := loadConfig(fixture, contextConfigPath)
		if err != nil {
			t.Fatalf("loadConfig(%s): %v", fixture, err)
		}
		if cfg.Workflow.Name != "eino-concurrency-assistant" || cfg.Workflow.Eino == nil {
			t.Fatalf("unexpected Eino workflow: name=%q eino=%v", cfg.Workflow.Name, cfg.Workflow.Eino)
		}
	})

	t.Run("barrier waits for every lane", func(t *testing.T) {
		barrier := newWorkflowConcurrencyBarrier(3)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done := make(chan error, 3)
		for lane := 1; lane <= 3; lane++ {
			lane := lane
			go func() { done <- barrier.arriveAndWait(ctx, lane) }()
		}
		ready, err := barrier.waitReady(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ready) != 3 {
			t.Fatalf("ready=%v, want three lanes", ready)
		}
		select {
		case <-done:
			t.Fatal("lane crossed the barrier before release")
		default:
		}
		barrier.releaseAll()
		for range 3 {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("events remain turn scoped", func(t *testing.T) {
		observations := []workflowConcurrencyTurnObservation{
			{result: workflowConcurrencyTurnResult{InputStreamID: "lane-1-turn-1", AssistantStreamID: "lane-1-turn-1:assistant"}},
			{result: workflowConcurrencyTurnResult{InputStreamID: "lane-1-turn-2"}},
		}
		streamID := "lane-1-turn-1:assistant"
		label := "assistant"
		if got := workflowConcurrencyEventTurn(observations, 1, peerStreamEvent{StreamId: &streamID, Label: &label}); got != 0 {
			t.Fatalf("old event attributed to turn %d, want 0", got)
		}
		streamID = "lane-1-turn-2:assistant"
		if got := workflowConcurrencyEventTurn(observations, 1, peerStreamEvent{StreamId: &streamID, Label: &label}); got != 1 {
			t.Fatalf("current event attributed to turn %d, want 1", got)
		}
	})

	t.Run("unbound interrupted routes stay with the cut over turn", func(t *testing.T) {
		observations := []workflowConcurrencyTurnObservation{
			{cutoverSent: true, result: workflowConcurrencyTurnResult{InputStreamID: "turn-1", AssistantStreamID: "turn-1:text"}},
			{result: workflowConcurrencyTurnResult{InputStreamID: "turn-2", AssistantStreamID: "turn-2:text"}},
		}
		label := "assistant"
		interrupted := "interrupted"
		oldAudioStreamID := "old-response:audio"
		event := peerStreamEvent{
			Type: peerStreamEventTypeEos, Kind: eventpb.StreamKind_STREAM_KIND_AUDIO,
			StreamId: &oldAudioStreamID, Label: &label, Error: &interrupted,
		}
		if got := workflowConcurrencyEventTurn(observations, 1, event); got != 0 {
			t.Fatalf("old interrupted audio route attributed to turn %d, want 0", got)
		}
	})

	t.Run("late output after interruption fails", func(t *testing.T) {
		observation := workflowConcurrencyTurnObservation{textInterruptedAt: time.Now()}
		text := "late"
		label := "assistant"
		streamID := "turn-1:assistant"
		err := observeWorkflowConcurrencyEvent(&observation, peerStreamEvent{
			Type: peerStreamEventTypeTextDelta, Kind: eventpb.StreamKind_STREAM_KIND_TEXT, StreamId: &streamID, Label: &label, Text: &text,
		}, time.Now())
		if err == nil || !strings.Contains(err.Error(), "continued after interrupted terminal") {
			t.Fatalf("late output error=%v", err)
		}
	})

	t.Run("queued audio uses receive time across channels", func(t *testing.T) {
		interruptedAt := time.Now()
		observation := workflowConcurrencyTurnObservation{
			audioEpoch:         interruptedAt.Add(-time.Second),
			audioInterruptedAt: interruptedAt,
		}
		if workflowConcurrencyAudioAfterInterruption(&observation, interruptedAt.Add(-time.Millisecond)) {
			t.Fatal("pre-interrupt packet was classified as late")
		}
		if !workflowConcurrencyAudioAfterInterruption(&observation, interruptedAt.Add(time.Millisecond)) {
			t.Fatal("post-interrupt packet was classified as queued")
		}
	})

	t.Run("text and audio interruption terminals are independent", func(t *testing.T) {
		observation := workflowConcurrencyTurnObservation{}
		label := "assistant"
		streamID := "turn-1:assistant"
		interrupted := "interrupted"
		for _, kind := range []eventpb.StreamKind{eventpb.StreamKind_STREAM_KIND_AUDIO, eventpb.StreamKind_STREAM_KIND_TEXT} {
			if err := observeWorkflowConcurrencyEvent(&observation, peerStreamEvent{
				Type: peerStreamEventTypeEos, Kind: kind, StreamId: &streamID, Label: &label, Error: &interrupted,
			}, time.Now()); err != nil {
				t.Fatal(err)
			}
		}
		if observation.result.InterruptedTerminals != 1 || observation.result.InterruptedText != 1 || observation.result.InterruptedAudio != 1 {
			t.Fatalf("interrupted terminals=%+v", observation.result)
		}
	})

	t.Run("route interruption terminal does not replace typed terminals", func(t *testing.T) {
		observation := workflowConcurrencyTurnObservation{}
		label := "assistant"
		streamID := "turn-1:assistant"
		interrupted := "interrupted"
		if err := observeWorkflowConcurrencyEvent(&observation, peerStreamEvent{
			Type: peerStreamEventTypeEos, Kind: eventpb.StreamKind_STREAM_KIND_UNSPECIFIED,
			StreamId: &streamID, Label: &label, Error: &interrupted,
		}, time.Now()); err != nil {
			t.Fatal(err)
		}
		if observation.result.InterruptedText != 0 || observation.result.InterruptedAudio != 0 {
			t.Fatalf("route terminal counted as typed terminal: %+v", observation.result)
		}
	})

	t.Run("open realtime interrupt waits for packets but not EOS", func(t *testing.T) {
		observation := workflowConcurrencyTurnObservation{}
		observation.assistant.WriteString("reply")
		observation.audioEpoch = time.Now()
		if workflowConcurrencyInterruptGateReady(&observation, realtimeWorkflowRealtimeConcurrencySpec) {
			t.Fatal("open realtime barge-in gate accepted interleaved old input packets")
		}
		observation.sendDone = true
		if workflowConcurrencyInterruptGateReady(&observation, realtimeWorkflowRealtimeConcurrencySpec) {
			t.Fatal("open realtime barge-in gate accepted audio BOS without a packet")
		}
		observation.result.AudioPackets = 1
		if !workflowConcurrencyInterruptGateReady(&observation, realtimeWorkflowRealtimeConcurrencySpec) {
			t.Fatal("open realtime barge-in gate required client audio EOS")
		}
	})

	t.Run("open realtime contract rejects client EOS", func(t *testing.T) {
		observation := workflowConcurrencyTurnObservation{sendDone: true, assistantTextDone: true, assistantAudioDone: true}
		observation.assistant.WriteString("reply")
		observation.transcript = "input"
		observation.audioEpoch = time.Now()
		observation.result = workflowConcurrencyTurnResult{
			InputEOSSent: true, AudioPackets: 1, TranscriptDone: true,
		}
		if err := validateWorkflowConcurrencyTurns(
			[]workflowConcurrencyTurnObservation{observation},
			realtimeWorkflowRealtimeConcurrencySpec,
			workflowConcurrencyConversation,
		); err == nil || !strings.Contains(err.Error(), "sent client EOS") {
			t.Fatalf("open realtime EOS validation error = %v", err)
		}
	})

	t.Run("empty assistant terminal fails immediately", func(t *testing.T) {
		observation := workflowConcurrencyTurnObservation{}
		label := "assistant"
		streamID := "turn-1:assistant"
		empty := ""
		err := observeWorkflowConcurrencyEvent(&observation, peerStreamEvent{
			Type: peerStreamEventTypeTextDone, StreamId: &streamID, Label: &label, Text: &empty,
		}, time.Now())
		if err == nil || !strings.Contains(err.Error(), "text response is empty") {
			t.Fatalf("empty terminal error=%v", err)
		}
	})

	t.Run("secret values are redacted", func(t *testing.T) {
		got := redactWorkflowConcurrencyText("authorization: Bearer-value api_key=secret registration_token='token-value'")
		for _, secret := range []string{"Bearer-value", "secret", "token-value"} {
			if strings.Contains(got, secret) {
				t.Fatalf("redaction retained %q in %q", secret, got)
			}
		}
	})

	t.Run("packet clones do not share storage", func(t *testing.T) {
		original := [][]byte{{1, 2, 3}}
		clone := cloneOpusPackets(original)
		clone[0][0] = 9
		if original[0][0] != 1 {
			t.Fatalf("clone mutated original: %v", original)
		}
	})

	t.Run("three turn contract requires two interruptions", func(t *testing.T) {
		observations := make([]workflowConcurrencyTurnObservation, 3)
		for index := range observations {
			observations[index].assistant.WriteString("ok")
			observations[index].assistantTextDone = true
			observations[index].sendDone = true
			observations[index].result.AssistantTextDone = true
			if index < 2 {
				observations[index].cutoverSent = true
				observations[index].result.InterruptedTerminals = 1
				observations[index].result.InterruptedText = 1
			}
		}
		if err := validateWorkflowConcurrencyTurns(observations, einoWorkflowConcurrencySpec, workflowConcurrencyInterrupt); err != nil {
			t.Fatal(err)
		}
		observations[1].result.InterruptedTerminals = 0
		if err := validateWorkflowConcurrencyTurns(observations, einoWorkflowConcurrencySpec, workflowConcurrencyInterrupt); err == nil {
			t.Fatal("missing interrupted terminal passed")
		}
	})
}
