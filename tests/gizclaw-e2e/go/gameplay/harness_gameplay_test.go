//go:build gizclaw_e2e

package gameplay_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type isolatedGameplayHarness struct {
	ctx  context.Context
	h    *clitest.Harness
	peer *gizcli.Client
}

func newIsolatedGameplayHarness(t *testing.T) *isolatedGameplayHarness {
	t.Helper()

	h := clitest.NewSetupHarness(t, "client-gameplay")
	h.InstallFixedAdminContext("admin-a").MustSucceed(t)
	h.RequireAdminContextEndpoint("admin-a")
	h.CreateContext("peer-a").MustSucceed(t)
	h.RequireClientContextEndpoint("peer-a")
	h.RegisterContext("peer-a", "--sn", "client-gameplay-peer-a-sn").MustSucceed(t)
	applyGameplayCatalog(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { peer.Close() })
	registerGameplayProfile(t, h, peer, "isolated")
	return &isolatedGameplayHarness{ctx: ctx, h: h, peer: peer}
}

func applyGameplayCatalog(t *testing.T, h *clitest.Harness) {
	t.Helper()

	for _, fixture := range []string{
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "04-workflows", "23-pet-care.yaml"),
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "07-gameplay", "00-starter-gameplay.yaml"),
	} {
		result := h.RunCLI("admin", "apply", "--context", "admin-a", "-f", fixture)
		result.MustSucceed(t)
	}
}

func registerGameplayProfile(t *testing.T, h *clitest.Harness, peer *gizcli.Client, tokenSuffix string) {
	t.Helper()

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tokenName := "e2e-gameplay-" + tokenSuffix
	_, _ = api.DeleteRegistrationTokenWithResponse(ctx, tokenName)
	tokenResp, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Name:               tokenName,
		Token:              tokenName,
		RuntimeProfileName: "default-gameplay",
	})
	if err != nil {
		t.Fatalf("create gameplay RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == "" {
		t.Fatalf("create gameplay RegistrationToken status %d: %s", tokenResp.StatusCode(), strings.TrimSpace(string(tokenResp.Body)))
	}
	registered, err := peer.Register(ctx, "server.register.gameplay", tokenResp.JSON200.Token)
	if err != nil {
		t.Fatalf("register gameplay connection: %v", err)
	}
	if registered.RuntimeProfileName != "default-gameplay" {
		t.Fatalf("register gameplay connection = %#v", registered)
	}
}

type setupGameplayHarness struct {
	ctx  context.Context
	h    *clitest.Harness
	peer *gizcli.Client
}

func newSetupGameplayHarness(t *testing.T, clientName string) *setupGameplayHarness {
	t.Helper()

	h := clitest.NewSetupHarness(t, clientName)
	identitiesHome := getenvDefault("GIZCLAW_E2E_IDENTITIES_HOME", filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities"))
	contextName := getenvDefault("GIZCLAW_E2E_PEER_IDENTITY", "peer")
	h.SetContextDirAlias("gear1", filepath.Join(identitiesHome, contextName))
	adminContextName := getenvDefault("GIZCLAW_E2E_ADMIN_IDENTITY", "admin")
	h.SetContextDirAlias("admin-a", filepath.Join(identitiesHome, adminContextName))
	h.RequireClientContextEndpoint("gear1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("gear1")
	t.Cleanup(func() { peer.Close() })
	registerGameplayProfile(t, h, peer, clientName)
	return &setupGameplayHarness{ctx: ctx, h: h, peer: peer}
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func synthesizeGameplayOpus(t *testing.T, env *setupGameplayHarness, model, voice, text string) [][]byte {
	t.Helper()
	httpClient := env.peer.HTTPClient(gizcli.ServicePeerOpenAI)
	httpClient.Timeout = 60 * time.Second
	client := openai.NewClient(
		option.WithAPIKey("gizclaw-peer"),
		option.WithBaseURL("http://gizclaw/v1"),
		option.WithHTTPClient(httpClient),
	)

	started := time.Now()
	speech, err := client.Audio.Speech.New(env.ctx, openai.AudioSpeechNewParams{
		Input:          text,
		Model:          openai.SpeechModel(model),
		Voice:          openai.AudioSpeechNewParamsVoice(voice),
		ResponseFormat: openai.AudioSpeechNewParamsResponseFormatOpus,
	})
	if err != nil {
		t.Fatalf("synthesize pet audio: %v", err)
	}
	audio, err := io.ReadAll(speech.Body)
	closeErr := speech.Body.Close()
	if err != nil {
		t.Fatalf("read synthesized pet audio: %v", err)
	}
	if closeErr != nil {
		t.Fatalf("close synthesized pet audio: %v", closeErr)
	}
	packets, err := gameplayOpusPackets(audio)
	if err != nil {
		t.Fatalf("decode synthesized pet audio: %v", err)
	}
	fmt.Printf("gameplay_audio_synthesis result=pass elapsed=%s packets=%d\n", time.Since(started).Truncate(time.Millisecond), len(packets))
	return packets
}

func gameplayOpusPackets(audio []byte) ([][]byte, error) {
	var packets [][]byte
	for packet, err := range ogg.Packets(bytes.NewReader(audio)) {
		if err != nil {
			return nil, fmt.Errorf("read Ogg Opus packets: %w", err)
		}
		if len(packet.Data) == 0 || bytes.HasPrefix(packet.Data, []byte("OpusHead")) || bytes.HasPrefix(packet.Data, []byte("OpusTags")) {
			continue
		}
		packets = append(packets, append([]byte(nil), packet.Data...))
	}
	if len(packets) == 0 {
		return nil, fmt.Errorf("speech returned no Opus payload packets")
	}
	return packets, nil
}

func sendGameplayAudioTurn(t *testing.T, ctx context.Context, stream *gizcli.PeerStream, streamID string, packets [][]byte) {
	t.Helper()
	const inputLabel = "workspacetest"
	if len(packets) == 0 {
		t.Fatal("pet audio turn has no packets")
	}
	if err := stream.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: inputLabel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: inputLabel, BeginOfStream: true},
	}); err != nil {
		t.Fatalf("push pet audio BOS: %v", err)
	}
	timestamp := time.Now().UnixMilli()
	for i, packet := range packets {
		if err := stream.Push(ctx, &genx.MessageChunk{
			Role: genx.RoleUser,
			Name: inputLabel,
			Part: &genx.Blob{MIMEType: "audio/opus", Data: append([]byte(nil), packet...)},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: inputLabel, Timestamp: timestamp},
		}); err != nil {
			t.Fatalf("push pet audio packet %d: %v", i+1, err)
		}
		timestamp += 20
		if i+1 < len(packets) {
			timer := time.NewTimer(20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				t.Fatalf("pace pet audio packets: %v", ctx.Err())
			case <-timer.C:
			}
		}
	}
	if err := stream.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: inputLabel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: inputLabel, Timestamp: timestamp, EndOfStream: true},
	}); err != nil {
		t.Fatalf("push pet audio EOS: %v", err)
	}
}

func waitForGameplayAssistantResponse(parent context.Context, stream genx.Stream, inputStreamID string) error {
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	var assistantText strings.Builder
	textDone := false
	audioDone := false
	audioPackets := 0
	var trace []string
	for !textDone || !audioDone || audioPackets == 0 {
		chunk, err := nextGameplayStreamChunk(ctx, stream)
		if err != nil {
			return fmt.Errorf("read pet audio response for %s: %w; text_done=%t audio_done=%t audio_packets=%d chunks=%s", inputStreamID, err, textDone, audioDone, audioPackets, strings.Join(trace, " | "))
		}
		trace = appendGameplayTrace(trace, gameplayChunkSummary(chunk))
		label := gameplayChunkLabel(chunk)
		streamID := gameplayChunkStreamID(chunk)
		if !gameplayResponseStreamIDMatches(streamID, inputStreamID) {
			continue
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Error) != "" {
			return fmt.Errorf("pet audio response for %s returned error %q", inputStreamID, chunk.Ctrl.Error)
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			if chunk.Role != genx.RoleModel && label != "assistant" {
				continue
			}
			assistantText.WriteString(string(part))
			if chunk.IsEndOfStream() {
				textDone = true
			}
		case *genx.Blob:
			if part == nil || !strings.EqualFold(strings.TrimSpace(part.MIMEType), "audio/opus") {
				continue
			}
			if gameplayChunkIsOpusPacket(chunk) {
				audioPackets++
			}
			if chunk.IsEndOfStream() {
				audioDone = true
			}
		}
	}
	if strings.TrimSpace(assistantText.String()) == "" {
		return fmt.Errorf("pet audio response for %s has no assistant text", inputStreamID)
	}
	return nil
}

func isRetryableGameplayResponseError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "doubaospeech: [Server processing timeout] node execution timeout") ||
		strings.Contains(text, "doubaospeech: [Server-side generic error]") && strings.Contains(text, "big asr recv err")
}

func TestRetryableGameplayResponseError(t *testing.T) {
	retryable := []error{
		fmt.Errorf("pet audio response returned error %q", "doubaospeech: [Server processing timeout] node execution timeout"),
		fmt.Errorf("pet audio response returned error %q", "doubaospeech: [Server-side generic error] OperatorWrapper Process failed: big asr recv err. rpc timeout"),
	}
	for _, err := range retryable {
		if !isRetryableGameplayResponseError(err) {
			t.Fatalf("isRetryableGameplayResponseError(%q) = false", err)
		}
	}
	if isRetryableGameplayResponseError(fmt.Errorf("pet audio response returned error %q", "permission denied")) {
		t.Fatal("permission error should not be retryable")
	}
}

func gameplayChunkIsOpusPacket(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return false
	}
	part, ok := chunk.Part.(*genx.Blob)
	return ok && part != nil && strings.EqualFold(strings.TrimSpace(part.MIMEType), "audio/opus") && len(part.Data) > 0
}

func TestGameplayChunkIsOpusPacketDoesNotRequireEventLabel(t *testing.T) {
	chunk := &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x01}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio"},
	}
	if !gameplayChunkIsOpusPacket(chunk) {
		t.Fatalf("unlabeled PeerStream Opus packet was not recognized: %#v", chunk)
	}
}

func appendGameplayTrace(trace []string, item string) []string {
	const maxItems = 40
	if len(trace) < maxItems {
		return append(trace, item)
	}
	copy(trace, trace[1:])
	trace[len(trace)-1] = item
	return trace
}

func gameplayChunkSummary(chunk *genx.MessageChunk) string {
	if chunk == nil {
		return "nil"
	}
	kind := fmt.Sprintf("%T", chunk.Part)
	size := 0
	switch part := chunk.Part.(type) {
	case genx.Text:
		size = len(part)
	case *genx.Blob:
		if part != nil {
			size = len(part.Data)
		}
	}
	errText := ""
	if chunk.Ctrl != nil {
		errText = chunk.Ctrl.Error
	}
	return fmt.Sprintf("stream=%s label=%s role=%s part=%s size=%d eos=%t error=%q", gameplayChunkStreamID(chunk), gameplayChunkLabel(chunk), chunk.Role, kind, size, chunk.IsEndOfStream(), errText)
}

func snapshotGameplayHistory(t *testing.T, ctx context.Context, client interface {
	ListWorkspaceHistory(context.Context, string, rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error)
}, workspaceName string) map[string]rpcapi.PeerRunHistoryEntry {
	t.Helper()
	limit := 100
	list, err := client.ListWorkspaceHistory(ctx, "gameplay.pet.history.baseline", rpcapi.WorkspaceHistoryListRequest{
		WorkspaceName: workspaceName,
		Limit:         &limit,
	})
	if err != nil {
		t.Fatalf("list pet workspace history baseline: %v", err)
	}
	out := make(map[string]rpcapi.PeerRunHistoryEntry, len(list.Items))
	for _, item := range list.Items {
		out[item.Id] = item
	}
	return out
}

func waitForSingleGameplayTranscript(t *testing.T, ctx context.Context, client interface {
	ListWorkspaceHistory(context.Context, string, rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error)
}, workspaceName string, known map[string]rpcapi.PeerRunHistoryEntry) rpcapi.PeerRunHistoryEntry {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	limit := 100
	var candidate rpcapi.PeerRunHistoryEntry
	var stableSince time.Time
	var lastErr error
	for {
		list, err := client.ListWorkspaceHistory(ctx, "gameplay.pet.history.wait", rpcapi.WorkspaceHistoryListRequest{
			WorkspaceName: workspaceName,
			Limit:         &limit,
		})
		if err == nil {
			items := newGameplayTranscriptItems(list.Items, known)
			if len(items) > 1 {
				t.Fatalf("one pet audio turn created %d new gear/transcript entries: %#v", len(items), items)
			}
			if len(items) == 1 && strings.TrimSpace(items[0].Text) != "" && items[0].ReplayAvailable {
				if candidate.Id != items[0].Id {
					candidate = items[0]
					stableSince = time.Now()
				} else if time.Since(stableSince) >= 500*time.Millisecond {
					return items[0]
				}
			}
			lastErr = nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			t.Fatalf("one combined replayable pet gear/transcript entry did not stabilize: candidate=%#v last_error=%v", candidate, lastErr)
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("wait for pet history: %v", ctx.Err())
		case <-timer.C:
		}
	}
}

func newGameplayTranscriptItems(items []rpcapi.PeerRunHistoryEntry, known map[string]rpcapi.PeerRunHistoryEntry) []rpcapi.PeerRunHistoryEntry {
	var out []rpcapi.PeerRunHistoryEntry
	for _, item := range items {
		if _, ok := known[item.Id]; ok {
			continue
		}
		if item.Type == rpcapi.PeerRunHistoryEntryTypeGear && item.Name == "transcript" {
			out = append(out, item)
		}
	}
	return out
}

func TestNewGameplayTranscriptItemsSelectsOnlyNewGearTranscripts(t *testing.T) {
	known := map[string]rpcapi.PeerRunHistoryEntry{"known": {Id: "known"}}
	items := []rpcapi.PeerRunHistoryEntry{
		{Id: "known", Name: "transcript", Type: rpcapi.PeerRunHistoryEntryTypeGear},
		{Id: "new-transcript", Name: "transcript", Text: "你好", ReplayAvailable: true, Type: rpcapi.PeerRunHistoryEntryTypeGear},
		{Id: "new-audio", Name: "audio", ReplayAvailable: true, Type: rpcapi.PeerRunHistoryEntryTypeGear},
		{Id: "new-agent", Name: "assistant", Type: rpcapi.PeerRunHistoryEntryTypeAgent},
	}
	got := newGameplayTranscriptItems(items, known)
	if len(got) != 1 || got[0].Id != "new-transcript" {
		t.Fatalf("newGameplayTranscriptItems() = %#v", got)
	}
}

func assertGameplayHistoryReplayAudio(t *testing.T, parent context.Context, client interface {
	PlayServerRunWorkspaceHistory(context.Context, string, rpcapi.ServerPlayRunWorkspaceHistoryRequest) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error)
}, stream *gizcli.PeerStream, entry rpcapi.PeerRunHistoryEntry) {
	t.Helper()
	play, err := client.PlayServerRunWorkspaceHistory(parent, "gameplay.pet.history.play", rpcapi.ServerPlayRunWorkspaceHistoryRequest{HistoryId: entry.Id})
	if err != nil {
		t.Fatalf("play pet history %q: %v", entry.Id, err)
	}
	if play == nil || !play.Accepted {
		t.Fatalf("play pet history %q = %#v, want accepted", entry.Id, play)
	}

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	var replayText strings.Builder
	textDone := false
	audioDone := false
	audioPackets := 0
	for !textDone || !audioDone || audioPackets == 0 {
		chunk, err := nextGameplayStreamChunk(ctx, stream)
		if err != nil {
			t.Fatalf("read pet history replay %q: %v", entry.Id, err)
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Error) != "" {
			t.Fatalf("pet history replay %q returned error %q", entry.Id, chunk.Ctrl.Error)
		}
		if !strings.HasPrefix(gameplayChunkStreamID(chunk), "history-replay-") {
			continue
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			replayText.WriteString(string(part))
			if chunk.IsEndOfStream() {
				textDone = true
			}
		case *genx.Blob:
			if part != nil && strings.EqualFold(strings.TrimSpace(part.MIMEType), "audio/opus") && len(part.Data) > 0 {
				audioPackets++
			}
			if chunk.IsEndOfStream() {
				audioDone = true
			}
		}
	}
	if strings.TrimSpace(replayText.String()) != strings.TrimSpace(entry.Text) {
		t.Fatalf("pet history replay %q text = %q, want %q", entry.Id, replayText.String(), entry.Text)
	}
}

func nextGameplayStreamChunk(ctx context.Context, stream genx.Stream) (*genx.MessageChunk, error) {
	type result struct {
		chunk *genx.MessageChunk
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		chunk, err := stream.Next()
		ch <- result{chunk: chunk, err: err}
	}()
	select {
	case got := <-ch:
		return got.chunk, got.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func gameplayChunkLabel(chunk *genx.MessageChunk) string {
	if chunk == nil {
		return ""
	}
	if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Label) != "" {
		return strings.TrimSpace(chunk.Ctrl.Label)
	}
	return strings.TrimSpace(chunk.Name)
}

func gameplayChunkStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return strings.TrimSpace(chunk.Ctrl.StreamID)
}

func gameplayResponseStreamIDMatches(actual, input string) bool {
	actual = strings.TrimSpace(actual)
	input = strings.TrimSpace(input)
	return input != "" && (actual == input || strings.HasPrefix(actual, input+":"))
}

type gameplayResponseTestStream struct {
	chunks []*genx.MessageChunk
}

func (s *gameplayResponseTestStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (*gameplayResponseTestStream) Close() error               { return nil }
func (*gameplayResponseTestStream) CloseWithError(error) error { return nil }

func TestWaitForGameplayAssistantResponseIgnoresPreviousAttempt(t *testing.T) {
	const (
		previous = "gameplay-pet-audio-1-1"
		current  = "gameplay-pet-audio-1-2"
		response = current + ":ast:1"
	)
	stream := &gameplayResponseTestStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: previous, Error: "stale retryable error"}},
		{Role: genx.RoleModel, Part: genx.Text("stale"), Ctrl: &genx.StreamCtrl{StreamID: previous, Label: "assistant", EndOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x01}}, Ctrl: &genx.StreamCtrl{StreamID: "stale-provider-audio"}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: previous, Label: "assistant", EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("current"), Ctrl: &genx.StreamCtrl{StreamID: response, Label: "assistant", EndOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x02}}, Ctrl: &genx.StreamCtrl{StreamID: response, Label: "assistant"}},
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: response, Label: "assistant", EndOfStream: true}},
	}}
	if err := waitForGameplayAssistantResponse(t.Context(), stream, current); err != nil {
		t.Fatalf("waitForGameplayAssistantResponse() error = %v", err)
	}
}

func TestGameplayResponseStreamIDMatchesCurrentAttempt(t *testing.T) {
	tests := []struct {
		name   string
		actual string
		input  string
		want   bool
	}{
		{name: "exact", actual: "gameplay-pet-audio-1-2", input: "gameplay-pet-audio-1-2", want: true},
		{name: "AST response", actual: "gameplay-pet-audio-1-2:ast:1", input: "gameplay-pet-audio-1-2", want: true},
		{name: "previous attempt", actual: "gameplay-pet-audio-1-1", input: "gameplay-pet-audio-1-2"},
		{name: "similar attempt prefix", actual: "gameplay-pet-audio-1-20", input: "gameplay-pet-audio-1-2"},
		{name: "missing response stream", input: "gameplay-pet-audio-1-2"},
		{name: "missing input stream", actual: "gameplay-pet-audio-1-2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gameplayResponseStreamIDMatches(test.actual, test.input); got != test.want {
				t.Fatalf("gameplayResponseStreamIDMatches(%q, %q) = %t, want %t", test.actual, test.input, got, test.want)
			}
		})
	}
}
