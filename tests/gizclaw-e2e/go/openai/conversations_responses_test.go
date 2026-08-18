//go:build gizclaw_e2e

package openai_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/openai/openai-go"
)

type e2eConversation struct {
	ID string `json:"id"`
}

type e2eResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output []struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

type e2eItemList struct {
	Data []struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	} `json:"data"`
}

type responseTiming struct {
	FirstEventMS int64 `json:"first_event_ms"`
	TTFTMS       int64 `json:"ttft_ms"`
	TotalMS      int64 `json:"total_ms"`
}

type speechTiming struct {
	HeaderMS    int64 `json:"response_header_ms"`
	FirstByteMS int64 `json:"first_byte_ms"`
	TotalMS     int64 `json:"total_ms"`
}

func TestOpenAIConversationsResponsesTextAndAudioComposition(t *testing.T) {
	env := newOpenAIHarness(t)
	peerRunBefore, err := env.peer.GetServerRunWorkspace(env.ctx, "openai.e2e.peerrun.before")
	if err != nil {
		t.Fatalf("get initial PeerRun Workspace: %v", err)
	}
	started := time.Now()
	createStarted := time.Now()
	var conversation e2eConversation
	if err := env.client.Post(env.ctx, "conversations", map[string]any{"metadata": map[string]string{"collection": "assistants", "workflow_name": "shared"}}, &conversation); err != nil {
		t.Fatalf("create Conversation: %v", err)
	}
	conversationCreateMS := elapsedMS(createStarted)
	getStarted := time.Now()
	var retrieved e2eConversation
	if err := env.client.Get(env.ctx, "conversations/"+conversation.ID, nil, &retrieved); err != nil || retrieved.ID != conversation.ID {
		t.Fatalf("get Conversation: %#v err=%v", retrieved, err)
	}
	conversationGetMS := elapsedMS(getStarted)
	var distinct e2eConversation
	if err := env.client.Post(env.ctx, "conversations", map[string]any{"metadata": map[string]string{"collection": "assistants", "workflow_name": "shared"}}, &distinct); err != nil {
		t.Fatalf("create distinct Conversation: %v", err)
	}
	if distinct.ID == conversation.ID {
		t.Fatalf("identical Conversation creates reused %q", conversation.ID)
	}
	warmup, warmupTiming, err := runStreamingResponse(env.ctx, env.http, distinct.ID, "compatibility warm-up")
	if err != nil || warmup.Status != "completed" {
		t.Fatalf("warm-up Response: %#v timing=%#v err=%v", warmup, warmupTiming, err)
	}
	var foreign e2eConversation
	if err := env.other.Get(env.ctx, "conversations/"+conversation.ID, nil, &foreign); err == nil {
		t.Fatalf("foreign Peer read Conversation %q", conversation.ID)
	}
	workspaceName := conversation.ID[len("conv_"):]
	distinctWorkspaceName := distinct.ID[len("conv_"):]
	t.Cleanup(func() {
		_, _ = env.peer.DeleteWorkspace(env.ctx, "openai.e2e.cleanup", rpcapi.WorkspaceDeleteRequest{Name: workspaceName})
		_, _ = env.peer.DeleteWorkspace(env.ctx, "openai.e2e.cleanup.distinct", rpcapi.WorkspaceDeleteRequest{Name: distinctWorkspaceName})
	})

	audio, inputSpeechTiming := synthesizeMeasuredSpeech(t, env, "second audio turn")
	pcm, sampleRate, channels, err := mp3.DecodeFull(bytes.NewReader(audio))
	if err != nil || sampleRate <= 0 || channels <= 0 {
		t.Fatalf("decode bounded audio fixture: rate=%d channels=%d err=%v", sampleRate, channels, err)
	}
	fixtureDurationMS := int64(len(pcm)) * 1000 / int64(sampleRate*channels*2)
	transcriptionStart := time.Now()
	composedStarted := transcriptionStart
	transcript, err := env.client.Audio.Transcriptions.New(env.ctx, openai.AudioTranscriptionNewParams{File: bytes.NewReader(audio), Model: openai.AudioModel("asr")})
	if err != nil || transcript.Text == "" {
		t.Fatalf("transcribe bounded audio: text=%t err=%v", transcript.Text != "", err)
	}
	transcriptionMS := elapsedMS(transcriptionStart)

	inputs := []string{"first text turn marker alpha", transcript.Text, "third text turn: recall alpha and the prior audio turn"}
	turnTimings := make([]responseTiming, 0, len(inputs))
	var last e2eResponse
	for _, input := range inputs {
		var timing responseTiming
		last, timing, err = runStreamingResponse(env.ctx, env.http, conversation.ID, input)
		if err != nil {
			t.Fatalf("create Response: %v", err)
		}
		if last.Status != "completed" {
			t.Fatalf("Response = %#v", last)
		}
		turnTimings = append(turnTimings, timing)
	}
	var items e2eItemList
	if err := env.client.Get(env.ctx, "conversations/"+conversation.ID+"/items?limit=20&order=asc", nil, &items); err != nil {
		t.Fatalf("list Conversation items: %v", err)
	}
	peerRunAfterReads, err := env.peer.GetServerRunWorkspace(env.ctx, "openai.e2e.peerrun.after")
	if err != nil {
		t.Fatalf("get final PeerRun Workspace: %v", err)
	}
	if !samePeerRunSelection(peerRunBefore, peerRunAfterReads) {
		t.Fatalf("OpenAI attachment changed PeerRun selection: before=%#v after=%#v", peerRunBefore, peerRunAfterReads)
	}
	if len(items.Data) != 6 {
		t.Fatalf("Conversation items = %d, want 6", len(items.Data))
	}
	for index, item := range items.Data {
		wantRole := "user"
		if index%2 == 1 {
			wantRole = "assistant"
		}
		if item.ID == "" || item.Role != wantRole {
			t.Fatalf("Conversation item %d = %#v, want role %q", index, item, wantRole)
		}
	}
	limit := 20
	order := rpcapi.WorkspaceHistoryListRequestOrderAsc
	history, err := env.peer.ListWorkspaceHistory(env.ctx, "openai.e2e.history", rpcapi.WorkspaceHistoryListRequest{WorkspaceName: workspaceName, Limit: &limit, Order: &order})
	if err != nil {
		t.Fatalf("list Workspace History: %v", err)
	}
	if len(history.Items) != 6 {
		t.Fatalf("Workspace History entries = %d, want 6", len(history.Items))
	}
	responseText := e2eResponseText(last)
	if responseText == "" {
		t.Fatalf("final Response has no textual output: %#v", last)
	}
	composedBeforeSpeechMS := elapsedMS(composedStarted)
	outputAudio, outputSpeechTiming := synthesizeMeasuredSpeech(t, env, responseText)
	composedFirstAudioMS := composedBeforeSpeechMS + outputSpeechTiming.FirstByteMS
	composedTotalMS := elapsedMS(composedStarted)

	var background e2eResponse
	if err := env.client.Post(env.ctx, "responses", map[string]any{"conversation": conversation.ID, "input": "cancel this turn", "background": true}, &background); err != nil || background.Status != "in_progress" {
		t.Fatalf("create background Response: %#v err=%v", background, err)
	}
	cancelStarted := time.Now()
	var cancelled e2eResponse
	if err := env.client.Post(env.ctx, "responses/"+background.ID+"/cancel", nil, &cancelled); err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("cancel background Response: %#v err=%v", cancelled, err)
	}
	cancelTerminalMS := elapsedMS(cancelStarted)

	streamBody, err := json.Marshal(map[string]any{"conversation": conversation.ID, "input": "abort this stream", "stream": true})
	if err != nil {
		t.Fatal(err)
	}
	streamCtx, streamCancel := context.WithCancel(env.ctx)
	request, err := http.NewRequestWithContext(streamCtx, http.MethodPost, "http://gizclaw/v1/responses", bytes.NewReader(streamBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer gizclaw-peer")
	request.Header.Set("Content-Type", "application/json")
	streamResponse, err := env.http.Do(request)
	if err != nil {
		t.Fatalf("start streaming Response: %v", err)
	}
	scanner := bufio.NewScanner(streamResponse.Body)
	sawDelta := false
	for scanner.Scan() {
		if bytes.Contains(scanner.Bytes(), []byte("response.output_text.delta")) {
			sawDelta = true
			break
		}
	}
	abortStarted := time.Now()
	streamCancel()
	if err := streamResponse.Body.Close(); err != nil {
		t.Fatalf("abort streaming Response: %v", err)
	}
	if !sawDelta {
		t.Fatalf("stream ended before output delta: %v", scanner.Err())
	}
	abortCloseMS := elapsedMS(abortStarted)
	slotReleaseStarted := time.Now()
	deadline := time.Now().Add(10 * time.Second)
	var recoveryTiming responseTiming
	for {
		last, recoveryTiming, err = runStreamingResponse(env.ctx, env.http, conversation.ID, "recover after interruption")
		if err == nil && last.Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("same-Conversation recovery: %#v err=%v", last, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	recoveryElapsedMS := elapsedMS(slotReleaseStarted)
	slotReleaseMS := max(recoveryElapsedMS-recoveryTiming.TotalMS+recoveryTiming.FirstEventMS, 1)
	if _, err := env.peer.DeleteWorkspace(env.ctx, "openai.e2e.cleanup", rpcapi.WorkspaceDeleteRequest{Name: workspaceName}); err != nil {
		t.Fatalf("delete Conversation Workspace: %v", err)
	}
	if _, err := env.peer.DeleteWorkspace(env.ctx, "openai.e2e.cleanup.distinct", rpcapi.WorkspaceDeleteRequest{Name: distinctWorkspaceName}); err != nil {
		t.Fatalf("delete distinct Conversation Workspace: %v", err)
	}
	var deleted e2eConversation
	if err := env.client.Get(env.ctx, "conversations/"+conversation.ID, nil, &deleted); err == nil {
		t.Fatalf("deleted Conversation %q remained readable", conversation.ID)
	}

	recordTimingArtifact(t, env.h.RepoRoot, map[string]any{
		"schema_version": 1, "target": "gizclaw-e2e", "case": "conversations-responses-text-audio",
		"status": "pass", "workflow_alias": "shared", "model_alias": "shared",
		"conversation_create_ms": conversationCreateMS, "conversation_get_ms": conversationGetMS,
		"conversation_create_to_cleanup_ms": elapsedMS(started), "warmup_response": warmupTiming,
		"response_samples": turnTimings, "response_aggregates": responseTimingAggregates(turnTimings),
		"transcription_first_event_ms": transcriptionMS, "transcription_total_ms": transcriptionMS,
		"audio_input_speech": inputSpeechTiming, "audio_output_speech": outputSpeechTiming,
		"audio_input_bytes": len(audio), "audio_output_bytes": len(outputAudio), "audio_fixture_duration_ms": fixtureDurationMS,
		"composed_audio_to_first_assistant_audio_ms": composedFirstAudioMS, "composed_audio_total_ms": composedTotalMS,
		"background_cancel_ack_to_terminal_ms": cancelTerminalMS, "stream_abort_to_local_close_ms": abortCloseMS,
		"workspace_slot_release_ms": slotReleaseMS, "recovery_response": recoveryTiming,
	})
}

func samePeerRunSelection(left, right *rpcapi.ServerGetRunWorkspaceResponse) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.WorkspaceName == right.WorkspaceName &&
		left.RuntimeState == right.RuntimeState &&
		optionalStringEqual(left.ActiveWorkspaceName, right.ActiveWorkspaceName) &&
		optionalStringEqual(left.SelectedWorkspaceName, right.SelectedWorkspaceName) &&
		optionalStringEqual(left.PendingWorkspaceName, right.PendingWorkspaceName)
}

func optionalStringEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func e2eResponseText(response e2eResponse) string {
	var text strings.Builder
	for _, output := range response.Output {
		for _, content := range output.Content {
			text.WriteString(content.Text)
		}
	}
	return strings.TrimSpace(text.String())
}

func runStreamingResponse(ctx context.Context, client *http.Client, conversationID, input string) (e2eResponse, responseTiming, error) {
	started := time.Now()
	body, err := json.Marshal(map[string]any{"conversation": conversationID, "input": input, "stream": true})
	if err != nil {
		return e2eResponse{}, responseTiming{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gizclaw/v1/responses", bytes.NewReader(body))
	if err != nil {
		return e2eResponse{}, responseTiming{}, err
	}
	request.Header.Set("Authorization", "Bearer gizclaw-peer")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return e2eResponse{}, responseTiming{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return e2eResponse{}, responseTiming{}, fmt.Errorf("streaming Response status %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	timing := responseTiming{}
	var terminal e2eResponse
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		if timing.FirstEventMS == 0 {
			timing.FirstEventMS = elapsedMS(started)
		}
		var event struct {
			Type     string      `json:"type"`
			Delta    string      `json:"delta"`
			Response e2eResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			return e2eResponse{}, timing, err
		}
		if event.Type == "response.output_text.delta" && event.Delta != "" && timing.TTFTMS == 0 {
			timing.TTFTMS = elapsedMS(started)
		}
		switch event.Type {
		case "response.completed":
			terminal = event.Response
			timing.TotalMS = elapsedMS(started)
		case "response.failed", "response.incomplete":
			return event.Response, timing, fmt.Errorf("streaming Response terminated as %s", event.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return e2eResponse{}, timing, err
	}
	if terminal.ID == "" || terminal.Status != "completed" || timing.FirstEventMS == 0 || timing.TTFTMS == 0 || timing.TotalMS == 0 || timing.FirstEventMS > timing.TTFTMS || timing.TTFTMS > timing.TotalMS {
		return terminal, timing, fmt.Errorf("incomplete streaming timing signals: %#v", timing)
	}
	return terminal, timing, nil
}

func synthesizeMeasuredSpeech(t *testing.T, env *openAIHarness, text string) ([]byte, speechTiming) {
	t.Helper()
	started := time.Now()
	speech, err := env.client.Audio.Speech.New(env.ctx, openai.AudioSpeechNewParams{Input: text, Model: openai.SpeechModelTTS1, Voice: "narrator", ResponseFormat: openai.AudioSpeechNewParamsResponseFormatMP3})
	if err != nil {
		t.Fatalf("synthesize speech: %v", err)
	}
	timing := speechTiming{HeaderMS: elapsedMS(started)}
	first := make([]byte, 1)
	n, readErr := io.ReadFull(speech.Body, first)
	if readErr != nil || n != 1 {
		_ = speech.Body.Close()
		t.Fatalf("read first speech byte: bytes=%d err=%v", n, readErr)
	}
	timing.FirstByteMS = elapsedMS(started)
	rest, readErr := io.ReadAll(io.LimitReader(speech.Body, 4<<20))
	closeErr := speech.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read synthesized speech: read=%v close=%v", readErr, closeErr)
	}
	if len(rest)+n > 4<<20 {
		t.Fatalf("synthesized speech exceeded 4 MiB bound: %d bytes", len(rest)+n)
	}
	timing.TotalMS = elapsedMS(started)
	if timing.HeaderMS > timing.FirstByteMS || timing.FirstByteMS > timing.TotalMS {
		t.Fatalf("invalid speech timing: %#v", timing)
	}
	return append(first[:n], rest...), timing
}

func elapsedMS(started time.Time) int64 {
	return max(time.Since(started).Milliseconds(), 1)
}

func responseTimingAggregates(samples []responseTiming) map[string]map[string]int64 {
	return map[string]map[string]int64{
		"first_event_ms": timingStats(samples, func(sample responseTiming) int64 { return sample.FirstEventMS }),
		"ttft_ms":        timingStats(samples, func(sample responseTiming) int64 { return sample.TTFTMS }),
		"total_ms":       timingStats(samples, func(sample responseTiming) int64 { return sample.TotalMS }),
	}
}

func timingStats(samples []responseTiming, value func(responseTiming) int64) map[string]int64 {
	values := make([]int64, 0, len(samples))
	for _, sample := range samples {
		values = append(values, value(sample))
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return map[string]int64{"min": values[0], "median": values[len(values)/2], "max": values[len(values)-1]}
}

func recordTimingArtifact(t *testing.T, repoRoot string, value map[string]any) {
	t.Helper()
	value["recorded_at"] = time.Now().UTC().Format(time.RFC3339)
	value["sdk_version"] = openAISDKVersion()
	head, err := gitOutput(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	status, err := gitOutput(repoRoot, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	value["repository"] = map[string]any{"head": head, "dirty": status != ""}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repoRoot, "tests", "gizclaw-e2e", "testdata", "openai-compatibility")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fmt.Sprintf("timings-%d.json", time.Now().UnixNano()))
	temporary, err := os.CreateTemp(dir, ".timings-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		t.Fatal(err)
	}
	t.Logf("OpenAI compatibility timing artifact: %s", path)
}

func gitOutput(repoRoot string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", repoRoot}, args...)
	data, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(data)))
	}
	return strings.TrimSpace(string(data)), nil
}

func openAISDKVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dependency := range info.Deps {
		if dependency.Path == "github.com/openai/openai-go" {
			return dependency.Version
		}
	}
	return "unknown"
}
