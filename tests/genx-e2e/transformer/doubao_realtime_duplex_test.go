//go:build gizclaw_genx_e2e

package transformer

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtimeduplex"
)

const (
	doubaoAppIDEnv  = "GIZCLAW_GENX_E2E_DOUBAO_APP_ID"
	doubaoAPIKeyEnv = "GIZCLAW_GENX_E2E_DOUBAO_API_KEY"

	duplexTranscriptLabel = "transcript"
	duplexAssistantLabel  = "assistant"
	duplexInputMIME       = "audio/opus"
	duplexRounds          = 2
)

//go:embed testdata/doubao_realtime_duplex_prompt.ogg
var doubaoRealtimeDuplexPromptOgg []byte

func TestDoubaoRealtimeDuplexConversation(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv, "GIZCLAW_E2E_DOUBAO_APP_ID")
	apiKey := firstEnv(doubaoAPIKeyEnv, "GIZCLAW_E2E_DOUBAO_API_KEY")
	if appID == "" || apiKey == "" {
		t.Skipf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}

	packets := embeddedPromptOpusPackets(t)
	client := doubaospeech.NewClient(appID, doubaospeech.WithAPIKey(apiKey))
	transcode := false
	config := doubaorealtimeduplex.Config{
		Client:         client,
		Instructions:   "Reply in one short English sentence.",
		InputTranscode: &transcode,
	}

	t.Run("realtime_server_vad_multi_round", func(t *testing.T) {
		tfr, err := doubaorealtimeduplex.New(config)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		results := runDuplexConversation(t, tfr, packets)
		for i, result := range results {
			assertDuplexRound(t, i+1, result)
		}
	})
}

func runDuplexConversation(t *testing.T, tfr genx.Transformer, packets [][]byte) []duplexRoundResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.CloseWithError(context.Canceled)

	output, err := tfr.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	events, errs := collectDuplexOutput(output)
	results := make([]duplexRoundResult, 0, duplexRounds)
	for round := 1; round <= duplexRounds; round++ {
		streamID := fmt.Sprintf("duplex-e2e-round-%d", round)
		feedDone := make(chan error, 1)
		go func() {
			feedDone <- pushDuplexTurn(ctx, input, streamID, packets)
		}()

		result, err := waitDuplexRound(ctx, events, errs, streamID, feedDone)
		if err != nil {
			t.Fatalf("round %d failed: %v", round, err)
		}
		if round < duplexRounds {
			if err := waitDuplexQuiet(ctx, events, errs, 1500*time.Millisecond); err != nil {
				t.Fatalf("round %d quiet wait failed: %v", round, err)
			}
		}
		t.Logf("round=%d transcript=%q assistant=%q assistant_audio_bytes=%d", round, result.transcript.String(), result.assistantText.String(), result.assistantAudioBytes)
		results = append(results, result)
	}
	return results
}

func collectDuplexOutput(output genx.Stream) (<-chan *genx.MessageChunk, <-chan error) {
	events := make(chan *genx.MessageChunk, 512)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		for {
			chunk, err := output.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
					errs <- err
				}
				return
			}
			events <- chunk
		}
	}()
	return events, errs
}

func pushDuplexTurn(ctx context.Context, input *genx.RealtimeStream, streamID string, packets [][]byte) error {
	if err := input.Push(ctx, &genx.MessageChunk{
		Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true},
	}); err != nil {
		return err
	}
	for _, packet := range packets {
		if err := input.Push(ctx, &genx.MessageChunk{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: duplexInputMIME, Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: streamID},
		}); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return nil
}

func waitDuplexRound(ctx context.Context, events <-chan *genx.MessageChunk, errs <-chan error, streamID string, feedDone <-chan error) (duplexRoundResult, error) {
	var result duplexRoundResult
	inputDone := false
	for {
		if inputDone && result.done() {
			return result, nil
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("wait duplex round %s: %w; transcript=%q assistant=%q audio_bytes=%d", streamID, ctx.Err(), result.transcript.String(), result.assistantText.String(), result.assistantAudioBytes)
		case err := <-feedDone:
			feedDone = nil
			inputDone = true
			if err != nil {
				return result, err
			}
		case err := <-errs:
			if err != nil {
				return result, err
			}
		case chunk, ok := <-events:
			if !ok {
				return result, fmt.Errorf("duplex output closed before round %s completed; transcript=%q assistant=%q audio_bytes=%d", streamID, result.transcript.String(), result.assistantText.String(), result.assistantAudioBytes)
			}
			if err := result.observe(streamID, chunk); err != nil {
				return result, err
			}
		}
	}
}

func waitDuplexQuiet(ctx context.Context, events <-chan *genx.MessageChunk, errs <-chan error, quiet time.Duration) error {
	timer := time.NewTimer(quiet)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errs:
			if err != nil {
				return err
			}
		case chunk, ok := <-events:
			if !ok {
				return nil
			}
			if err := duplexChunkError(chunk); err != nil {
				return err
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(quiet)
		case <-timer.C:
			return nil
		}
	}
}

type duplexRoundResult struct {
	transcript          strings.Builder
	assistantText       strings.Builder
	transcriptDone      bool
	assistantTextDone   bool
	assistantAudioDone  bool
	assistantAudioBytes int
}

func (r *duplexRoundResult) observe(streamID string, chunk *genx.MessageChunk) error {
	if chunk == nil {
		return nil
	}
	label := ""
	chunkStreamID := ""
	if chunk.Ctrl != nil {
		label = chunk.Ctrl.Label
		chunkStreamID = chunk.Ctrl.StreamID
	}
	if (label == duplexTranscriptLabel || label == duplexAssistantLabel) &&
		!roundStreamMatches(chunkStreamID, streamID) {
		return nil
	}
	if err := duplexChunkError(chunk); err != nil {
		return err
	}
	if label == duplexTranscriptLabel {
		if text, ok := chunk.Part.(genx.Text); ok && strings.TrimSpace(string(text)) != "" {
			r.transcript.WriteString(string(text))
		}
		if chunk.IsEndOfStream() {
			r.transcriptDone = true
		}
		return nil
	}
	if label != duplexAssistantLabel {
		return nil
	}
	switch part := chunk.Part.(type) {
	case genx.Text:
		if strings.TrimSpace(string(part)) != "" {
			r.assistantText.WriteString(string(part))
		}
		if chunk.IsEndOfStream() {
			r.assistantTextDone = true
		}
	case *genx.Blob:
		if len(part.Data) > 0 {
			r.assistantAudioBytes += len(part.Data)
		}
		if chunk.IsEndOfStream() {
			r.assistantAudioDone = true
		}
	}
	return nil
}

func TestDuplexRoundResultIgnoresOtherStreamTerminalError(t *testing.T) {
	var result duplexRoundResult
	oldStreamError := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{
			StreamID:    "round-1:rt:1",
			Label:       duplexAssistantLabel,
			EndOfStream: true,
			Error:       "interrupted",
		},
	}
	if err := result.observe("round-2", oldStreamError); err != nil {
		t.Fatalf("observe() unrelated terminal error = %v", err)
	}

	currentStreamError := oldStreamError.Clone()
	currentStreamError.Ctrl.StreamID = "round-2:rt:1"
	if err := result.observe("round-2", currentStreamError); err == nil {
		t.Fatal("observe() current terminal error = nil")
	}
}

func (r *duplexRoundResult) done() bool {
	return strings.TrimSpace(r.transcript.String()) != "" &&
		r.transcriptDone &&
		strings.TrimSpace(r.assistantText.String()) != "" &&
		r.assistantAudioBytes > 0 &&
		r.assistantTextDone &&
		r.assistantAudioDone
}

func assertDuplexRound(t *testing.T, round int, result duplexRoundResult) {
	t.Helper()
	if strings.TrimSpace(result.transcript.String()) == "" {
		t.Fatalf("round %d missing transcript", round)
	}
	if strings.TrimSpace(result.assistantText.String()) == "" {
		t.Fatalf("round %d missing assistant text", round)
	}
	if result.assistantAudioBytes == 0 {
		t.Fatalf("round %d missing assistant audio", round)
	}
}

func roundStreamMatches(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	return got == want || strings.HasPrefix(got, want+":")
}

func duplexChunkError(chunk *genx.MessageChunk) error {
	if chunk == nil || chunk.Ctrl == nil || strings.TrimSpace(chunk.Ctrl.Error) == "" {
		return nil
	}
	return fmt.Errorf("duplex stream %q label=%q returned error: %s", chunk.Ctrl.StreamID, chunk.Ctrl.Label, chunk.Ctrl.Error)
}

func embeddedPromptOpusPackets(t *testing.T) [][]byte {
	t.Helper()
	var packets [][]byte
	for packet, err := range codecconv.OggOpusPackets(bytes.NewReader(doubaoRealtimeDuplexPromptOgg)) {
		if err != nil {
			t.Fatalf("read embedded ogg opus packets: %v", err)
		}
		if len(packet) == 0 {
			continue
		}
		packets = append(packets, packet)
	}
	if len(packets) == 0 {
		t.Fatal("embedded Ogg/Opus fixture has no audio packets")
	}
	return packets
}

func loadGenXE2EEnv(t *testing.T) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return
	}
	path := filepath.Join(filepath.Dir(filename), "..", ".env")
	file, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("open %s: %v", path, err)
		}
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func TestDoubaoRealtimeDuplexCommitDuringDownlinkProbe(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv, "GIZCLAW_E2E_DOUBAO_APP_ID")
	apiKey := firstEnv(doubaoAPIKeyEnv, "GIZCLAW_E2E_DOUBAO_API_KEY")
	if appID == "" || apiKey == "" {
		t.Skipf("set %s and %s in tests/genx-e2e/.env to run this provider probe", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	session := openProbeDuplexSession(t, ctx, appID, apiKey)
	defer session.Close()

	if err := session.CommitSpeechText(ctx, doubaospeech.RealtimeDuplexSpeechTextRequest{
		SpeechID: "probe-long-story",
		Text:     longStoryProbeText(),
	}); err != nil {
		t.Fatalf("commit long story speech text: %v", err)
	}

	packets := embeddedPromptOpusPackets(t)
	secondCommitDone := make(chan probeCommitResult, 1)
	secondCommitStarted := false
	secondCommitAt := time.Time{}
	firstResponseID := ""
	firstAudioDeltasBeforeCommit := 0
	firstAudioDeltasAfterCommit := 0
	audioDeltaTotal := 0
	firstAudioDoneAfterCommit := false
	responseCanceledAfterCommit := false
	responseDoneAfterCommit := false
	receivedAnyAudio := false
	observedAfterCommit := false
	longResponseActive := false

	for {
		evt, err := session.RecvEvent(ctx)
		if err != nil {
			t.Fatalf("recv event: %v", err)
		}
		if evt.Type != doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta {
			t.Logf("event type=%s response=%s question=%s status=%s audio=%d delta=%q text=%q transcript=%q",
				evt.Type,
				evt.ResponseID,
				evt.QuestionID,
				evt.StatusCode,
				len(evt.Audio),
				trimLog(evt.Delta),
				trimLog(evt.Text),
				trimLog(evt.Transcript),
			)
		}
		if evt.Type == doubaospeech.RealtimeDuplexEventError {
			if evt.Error != nil {
				t.Fatalf("server error after commitStarted=%t commitDone=%t: %v", secondCommitStarted, !secondCommitAt.IsZero(), evt.Error)
			}
			t.Fatalf("server returned generic error event after commitStarted=%t commitDone=%t", secondCommitStarted, !secondCommitAt.IsZero())
		}
		if evt.ResponseID != "" && firstResponseID == "" {
			firstResponseID = evt.ResponseID
		}
		switch evt.Type {
		case doubaospeech.RealtimeDuplexEventResponseOutputAudioStarted:
			longResponseActive = true
		case doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta:
			audioDeltaTotal++
			if audioDeltaTotal%20 == 0 {
				t.Logf("audio delta count=%d commitDone=%t bytes=%d", audioDeltaTotal, !secondCommitAt.IsZero(), len(evt.Audio))
			}
			if longResponseActive {
				receivedAnyAudio = true
				if secondCommitAt.IsZero() {
					firstAudioDeltasBeforeCommit++
				} else {
					firstAudioDeltasAfterCommit++
				}
			}
			if !secondCommitStarted {
				secondCommitStarted = true
				go func() {
					start := time.Now()
					err := commitProbeAudio(ctx, session, packets)
					secondCommitDone <- probeCommitResult{at: time.Now(), elapsed: time.Since(start), err: err}
				}()
			}
		case doubaospeech.RealtimeDuplexEventResponseOutputAudioDone:
			if longResponseActive && !secondCommitAt.IsZero() {
				firstAudioDoneAfterCommit = true
				observedAfterCommit = true
			}
			longResponseActive = false
		case doubaospeech.RealtimeDuplexEventResponseCanceled:
			if !secondCommitAt.IsZero() {
				responseCanceledAfterCommit = true
				observedAfterCommit = true
			}
		case doubaospeech.RealtimeDuplexEventResponseDone:
			if !secondCommitAt.IsZero() {
				responseDoneAfterCommit = true
				observedAfterCommit = true
			}
		}

		select {
		case result := <-secondCommitDone:
			if result.err != nil {
				t.Fatalf("second input_audio_buffer.commit failed: %v", result.err)
			}
			secondCommitAt = result.at
			t.Logf("second input_audio_buffer.commit sent after %s", result.elapsed)
		default:
		}

		if !receivedAnyAudio {
			continue
		}
		if secondCommitStarted && !secondCommitAt.IsZero() && observedAfterCommit &&
			(firstAudioDoneAfterCommit || responseCanceledAfterCommit || responseDoneAfterCommit) {
			t.Logf("probe result: first_response=%s audio_deltas_before_commit=%d audio_deltas_after_commit=%d audio_done_after_commit=%t response_canceled_after_commit=%t response_done_after_commit=%t",
				firstResponseID,
				firstAudioDeltasBeforeCommit,
				firstAudioDeltasAfterCommit,
				firstAudioDoneAfterCommit,
				responseCanceledAfterCommit,
				responseDoneAfterCommit,
			)
			return
		}
	}
}

func TestDoubaoRealtimeDuplexDownlinkWithoutUplinkProbe(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv, "GIZCLAW_E2E_DOUBAO_APP_ID")
	apiKey := firstEnv(doubaoAPIKeyEnv, "GIZCLAW_E2E_DOUBAO_API_KEY")
	if appID == "" || apiKey == "" {
		t.Skipf("set %s and %s in tests/genx-e2e/.env to run this provider probe", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session := openProbeDuplexSession(t, ctx, appID, apiKey)
	defer session.Close()

	start := time.Now()
	if err := session.CommitSpeechText(ctx, doubaospeech.RealtimeDuplexSpeechTextRequest{
		SpeechID: "probe-long-downlink-no-uplink",
		Text:     strings.Repeat(longStoryProbeText(), 3),
	}); err != nil {
		t.Fatalf("commit long downlink speech text: %v", err)
	}

	audioDeltas := 0
	audioBytes := 0
	responseID := ""
	for {
		evt, err := session.RecvEvent(ctx)
		if err != nil {
			t.Fatalf("recv event after no-uplink commit: %v", err)
		}
		if evt.Type != doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta {
			t.Logf("event type=%s response=%s question=%s status=%s audio=%d elapsed=%s",
				evt.Type,
				evt.ResponseID,
				evt.QuestionID,
				evt.StatusCode,
				len(evt.Audio),
				time.Since(start).Round(time.Millisecond),
			)
		}
		if evt.Type == doubaospeech.RealtimeDuplexEventError {
			if evt.Error != nil {
				t.Fatalf("server error while downlink continued without uplink: %v", evt.Error)
			}
			t.Fatal("server returned generic error while downlink continued without uplink")
		}
		if evt.ResponseID != "" && responseID == "" {
			responseID = evt.ResponseID
		}
		switch evt.Type {
		case doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta:
			audioDeltas++
			audioBytes += len(evt.Audio)
			if audioDeltas%50 == 0 {
				t.Logf("downlink delta count=%d bytes=%d elapsed=%s", audioDeltas, audioBytes, time.Since(start).Round(time.Millisecond))
			}
		case doubaospeech.RealtimeDuplexEventResponseOutputAudioDone:
			t.Logf("downlink no-uplink result: response=%s audio_deltas=%d audio_bytes=%d elapsed=%s",
				responseID,
				audioDeltas,
				audioBytes,
				time.Since(start).Round(time.Millisecond),
			)
			if audioDeltas == 0 || audioBytes == 0 {
				t.Fatal("downlink completed without audio deltas")
			}
			return
		case doubaospeech.RealtimeDuplexEventResponseCanceled:
			t.Fatalf("response canceled while downlink continued without uplink: response=%s audio_deltas=%d audio_bytes=%d elapsed=%s",
				responseID,
				audioDeltas,
				audioBytes,
				time.Since(start).Round(time.Millisecond),
			)
		}
	}
}

func TestDoubaoRealtimeDuplexIdleAfterResponseProbe(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv, "GIZCLAW_E2E_DOUBAO_APP_ID")
	apiKey := firstEnv(doubaoAPIKeyEnv, "GIZCLAW_E2E_DOUBAO_API_KEY")
	if appID == "" || apiKey == "" {
		t.Skipf("set %s and %s in tests/genx-e2e/.env to run this provider probe", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	session := openProbeDuplexSession(t, ctx, appID, apiKey)
	defer session.Close()

	if err := session.CommitSpeechText(ctx, doubaospeech.RealtimeDuplexSpeechTextRequest{
		SpeechID: "probe-short-response-before-idle",
		Text:     "请用一句中文简短回答：好的。",
	}); err != nil {
		t.Fatalf("commit short speech text: %v", err)
	}

	start := time.Now()
	idleStart := time.Time{}
	audioDeltas := 0
	for {
		evt, err := session.RecvEvent(ctx)
		if err != nil {
			if idleStart.IsZero() {
				t.Fatalf("recv before idle: %v", err)
			}
			t.Fatalf("recv after idle %s: %v", time.Since(idleStart).Round(time.Millisecond), err)
		}
		if evt.Type != doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta {
			t.Logf("event type=%s response=%s audio=%d since_start=%s idle_for=%s",
				evt.Type,
				evt.ResponseID,
				len(evt.Audio),
				time.Since(start).Round(time.Millisecond),
				idleDuration(idleStart),
			)
		}
		if evt.Type == doubaospeech.RealtimeDuplexEventError {
			if evt.Error != nil {
				t.Fatalf("server error idle_for=%s: %v", idleDuration(idleStart), evt.Error)
			}
			t.Fatalf("server generic error idle_for=%s", idleDuration(idleStart))
		}
		switch evt.Type {
		case doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta:
			audioDeltas++
		case doubaospeech.RealtimeDuplexEventResponseOutputAudioDone:
			if audioDeltas == 0 {
				t.Fatal("short response completed without audio")
			}
			idleStart = time.Now()
			t.Logf("idle probe started after response audio done; no more uplink will be sent")
		case doubaospeech.RealtimeDuplexEventSessionClosed:
			if idleStart.IsZero() {
				t.Fatal("session closed before idle started")
			}
			t.Fatalf("session closed by server after idle %s", time.Since(idleStart).Round(time.Millisecond))
		}
		if !idleStart.IsZero() && time.Since(idleStart) >= 3*time.Minute {
			t.Logf("idle probe result: no provider timeout observed after idle %s", time.Since(idleStart).Round(time.Millisecond))
			return
		}
	}
}

func openProbeDuplexSession(t *testing.T, ctx context.Context, appID, apiKey string) *doubaospeech.RealtimeDuplexSession {
	t.Helper()
	client := doubaospeech.NewClient(appID, doubaospeech.WithAPIKey(apiKey))
	cfg := doubaospeech.DefaultRealtimeDuplexConfig()
	cfg.Session.Model = doubaospeech.RealtimeDuplexModelDefault
	cfg.Session.Instructions = "You are a realtime duplex probe assistant. Keep the audio flowing naturally."
	cfg.Session.Audio.Input.Format.Type = doubaospeech.RealtimeDuplexAudioOpus
	cfg.Session.Audio.Input.Format.Rate = 16000
	cfg.Session.Audio.Output.Format.Type = doubaospeech.RealtimeDuplexAudioOggOpus
	cfg.Session.Audio.Output.Format.Rate = 24000
	cfg.Session.Audio.Output.Voice = "zh_female_vv_jupiter_bigtts"
	session, err := client.RealtimeDuplex.OpenSession(ctx, &cfg)
	if err != nil {
		t.Fatalf("open realtime duplex session: %v", err)
	}
	return session
}

func idleDuration(start time.Time) string {
	if start.IsZero() {
		return "not-started"
	}
	return time.Since(start).Round(time.Millisecond).String()
}

type probeCommitResult struct {
	at      time.Time
	elapsed time.Duration
	err     error
}

func commitProbeAudio(ctx context.Context, session interface {
	SendAudio(context.Context, []byte) error
	CommitAudio(context.Context) error
}, packets [][]byte) error {
	for _, packet := range packets {
		if err := session.SendAudio(ctx, packet); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return session.CommitAudio(ctx)
}

func longStoryProbeText() string {
	paragraphs := []string{
		"从前有一座靠海的小城，城里有一间很旧的钟表铺。铺子里的老钟匠每天清晨都会打开木窗，让海风吹过满墙滴答作响的钟。",
		"有一天，一个孩子带来一只不会走的怀表。老钟匠打开表壳，发现里面没有坏掉的齿轮，只有一粒细小的蓝色沙子，像夜空里落下来的星光。",
		"孩子说，这只表属于他的母亲。每当表针走动，母亲就会想起一段被遗忘的旅程。可是表停了以后，家里的人也渐渐不再提起那些故事。",
		"老钟匠没有马上修理它。他把蓝色沙子放进一只玻璃瓶，又让孩子坐在门口听海浪。海浪一层一层拍过石阶，好像在替怀表数着时间。",
		"到了傍晚，瓶子里的沙子忽然亮了起来。墙上所有钟表都同时停住，只有那只怀表轻轻响了一声。孩子看见表盘里映出一条长长的路，路的尽头是一盏温暖的灯。",
		"老钟匠说，时间不是为了把人带走，而是为了让人知道还有什么值得回去寻找。孩子握着重新走动的怀表，沿着海边慢慢回家。",
	}
	return strings.Join(paragraphs, "")
}

func trimLog(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 60 {
		return value
	}
	return fmt.Sprintf("%s...", value[:60])
}
