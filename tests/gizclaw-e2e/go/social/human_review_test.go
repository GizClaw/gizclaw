//go:build gizclaw_e2e

package social_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/portaudio"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

const (
	socialHumanReviewOutputUnderflowRetries = 3
	socialHumanReviewSampleRate             = 48000
	socialHumanReviewFrameSize              = socialHumanReviewSampleRate / 50
	socialHumanReviewRuntimeProfile         = "social-human-review"
	socialHumanReviewTTSModel               = "volc-bigtts"
	socialHumanReviewVoiceResource          = "volc-tenant:volc-main:zh_female_vv_mars_bigtts"
)

func TestServerSocialRPCHumanReview(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("opus runtime is unavailable for social human review")
	}
	if !portaudio.NativeRuntimeSupported() {
		t.Skipf("portaudio backend %q is unavailable for social human review", portaudio.BackendName())
	}
	requireSocialHumanReviewProviderEnv(t)

	playback := newSocialHumanReviewPlayback(t)
	defer playback.Close()

	h := newSocialHumanReviewHarness(t)
	peerB := h.ContextPublicKey("peer-b")
	peerC := h.ContextPublicKey("peer-c")

	requestAB := createFriendByInviteToken(t, h, "peer-a", "peer-b", peerB)
	if stringValue(requestAB.WorkspaceName) == "" {
		t.Fatalf("accepted friend workspace is empty: %#v", requestAB)
	}
	t.Run("friend direct chat", func(t *testing.T) {
		runSocialHumanReviewAudioStory(t, h, playback, "peer-a", "peer-b", stringValue(requestAB.WorkspaceName), []string{
			"你好，这是好友语音留言测试第一轮。",
			"请确认第二轮好友留言也可以保存和回放。",
			"第三轮测试好友历史播放是否仍然有声音。",
		})
	})

	group := mustCreateFriendGroup(t, h, "peer-a", "human-review", "")
	if stringValue(group.WorkspaceName) == "" {
		t.Fatalf("friend_group.create workspace_name is empty: %#v", group)
	}
	mustAddFriendGroupMember(t, h, "peer-a", stringValue(group.Id), peerB, rpcapi.FriendGroupMemberMutableRoleMember)
	mustAddFriendGroupMember(t, h, "peer-a", stringValue(group.Id), peerC, rpcapi.FriendGroupMemberMutableRoleMember)
	t.Run("group chat", func(t *testing.T) {
		runSocialHumanReviewAudioStory(t, h, playback, "peer-b", "peer-c", stringValue(group.WorkspaceName), []string{
			"你好，这是群聊语音留言测试第一轮。",
			"请确认第二轮群聊留言也可以保存和回放。",
			"第三轮测试群聊历史播放是否仍然有声音。",
		})
	})
}

func newSocialHumanReviewHarness(t *testing.T) *sharedSocialClients {
	t.Helper()

	h := clitest.NewSetupHarness(t, "client-social-human-review")
	configureSocialAdminContext(t, h)
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create social human-review admin client: %v", err)
	}
	configureSocialPeerContext(t, h, "peer-a", "GIZCLAW_E2E_SOCIAL_PERSON_A_IDENTITY", "social-a", "client-social-human-review-peer-a-sn")
	configureSocialPeerContext(t, h, "peer-b", "GIZCLAW_E2E_SOCIAL_PERSON_B_IDENTITY", "social-b", "client-social-human-review-peer-b-sn")
	for _, peer := range []string{"peer-c"} {
		h.CreateContext(peer).MustSucceed(t)
		h.RegisterContext(peer, "--sn", "client-social-human-review-"+peer+"-sn").MustSucceed(t)
	}
	shared := newSharedSocialClients(t, h)
	registerSocialHumanReviewProfile(t, api, shared, "peer-a", "peer-b", "peer-c")
	return shared
}

func requireSocialHumanReviewProviderEnv(t *testing.T) {
	t.Helper()

	setEnvFallback(t, "GIZCLAW_E2E_DOUBAO_APP_ID", "DOUBAO_SPEECH_APP_ID")
	setEnvFallback(t, "GIZCLAW_E2E_DOUBAO_API_KEY", "DOUBAO_SPEECH_API_KEY")
	setEnvFallback(t, "GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY_ID", "VOLC_ACCESS_KEY_ID", "VOLC_ACCESS_KEY")
	setEnvFallback(t, "GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY", "VOLC_SECRET_ACCESS_KEY", "VOLC_SECRET_KEY")

	required := []string{
		"GIZCLAW_E2E_DOUBAO_APP_ID",
		"GIZCLAW_E2E_DOUBAO_API_KEY",
		"GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY_ID",
		"GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY",
	}
	var missing []string
	for _, key := range required {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Skipf("set %s provider env to run social human-review audio", strings.Join(missing, ", "))
	}
}

func setEnvFallback(t *testing.T, key string, fallbacks ...string) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return
	}
	for _, fallback := range fallbacks {
		if value := strings.TrimSpace(os.Getenv(fallback)); value != "" {
			t.Setenv(key, value)
			return
		}
	}
}

func registerSocialHumanReviewProfile(t *testing.T, api *adminhttp.ClientWithResponses, clients *sharedSocialClients, peers ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	workflows := map[string]string{
		"direct": "chatroom-direct",
		"group":  "chatroom",
	}
	models := map[string]string{
		"asr": "volc-asr",
		"tts": socialHumanReviewTTSModel,
	}
	voices := map[string]string{"tts": socialHumanReviewVoiceResource}
	profileResp, err := api.PutRuntimeProfileWithResponse(ctx, socialHumanReviewRuntimeProfile, adminhttp.RuntimeProfileUpsert{
		Name: socialHumanReviewRuntimeProfile,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{
			Workflows: &workflows,
			Models:    &models,
			Voices:    &voices,
		}},
	})
	if err != nil {
		t.Fatalf("put social human-review RuntimeProfile: %v", err)
	}
	if profileResp.JSON200 == nil {
		t.Fatalf("put social human-review RuntimeProfile status %d: %s", profileResp.StatusCode(), strings.TrimSpace(string(profileResp.Body)))
	}
	tokenName := "e2e-social-human-review"
	_, _ = api.DeleteRegistrationTokenWithResponse(ctx, tokenName)
	tokenResp, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Name:               tokenName,
		FirmwareName:       "devkit-firmware-main",
		RuntimeProfileName: socialHumanReviewRuntimeProfile,
	})
	if err != nil {
		t.Fatalf("create social human-review RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == "" {
		t.Fatalf("create social human-review RegistrationToken status %d: %s", tokenResp.StatusCode(), strings.TrimSpace(string(tokenResp.Body)))
	}
	for _, peerName := range peers {
		registered, err := clients.Client(peerName).Register(ctx, "server.register.social-human-review", tokenResp.JSON200.Token)
		if err != nil {
			t.Fatalf("register %s for social human review: %v", peerName, err)
		}
		if registered.RuntimeProfileName != socialHumanReviewRuntimeProfile {
			t.Fatalf("register %s for social human review = %#v", peerName, registered)
		}
	}
}

func runSocialHumanReviewAudioStory(t *testing.T, h socialHarness, playback *socialHumanReviewPlayback, writerContext, readerContext, workspaceName string, texts []string) {
	t.Helper()
	if len(texts) < 3 {
		t.Fatalf("social human-review audio story needs at least 3 rounds, got %d", len(texts))
	}

	writer := h.Client(writerContext)
	reader := h.Client(readerContext)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ensureSocialHumanReviewWorkspace(t, ctx, reader, readerContext, workspaceName)
	readerInput := newBlockingStream()
	readerOut, err := reader.Transform(ctx, readerInput)
	if err != nil {
		t.Fatalf("%s open human-review reader stream: %v", readerContext, err)
	}
	defer readerOut.Close()
	defer readerInput.CloseWithError(io.EOF)

	ensureSocialHumanReviewWorkspace(t, ctx, writer, writerContext, workspaceName)
	writerStream, err := writer.OpenPeerStream(64)
	if err != nil {
		t.Fatalf("%s open human-review writer stream: %v", writerContext, err)
	}
	defer writerStream.Close()
	seenHistoryIDs := make(map[string]struct{}, len(texts))
	for i, text := range texts {
		round := i + 1
		updatedCh := waitForWorkspaceHistoryUpdated(readerOut)
		_, inputPackets := synthesizeSocialHumanReviewSpeech(t, ctx, writer, text)
		fmt.Printf("social_human_review input_ready workspace=%s round=%d text=%q packets=%d\n", workspaceName, round, text, len(inputPackets))
		if err := playback.PlayOpusPackets(fmt.Sprintf("live %s round %d", workspaceName, round), inputPackets); err != nil {
			t.Fatalf("play live social input round %d: %v", round, err)
		}
		if err := pushSocialHumanReviewAudioTurn(ctx, writerStream, inputPackets); err != nil {
			t.Fatalf("%s send human-review audio round %d: %v", writerContext, round, err)
		}

		select {
		case err := <-updatedCh:
			if err != nil {
				t.Fatalf("%s did not observe human-review history update round %d: %v", readerContext, round, err)
			}
		case <-ctx.Done():
			t.Fatalf("%s did not observe human-review history update round %d before timeout: %v", readerContext, round, ctx.Err())
		}

		entry := waitForWorkspaceHistoryReplayableGear(t, ctx, reader, workspaceName, h.ContextPublicKey(writerContext), seenHistoryIDs)
		seenHistoryIDs[entry.Id] = struct{}{}
		got, err := reader.GetWorkspaceHistory(ctx, "social.human_review.history.get", rpcapi.WorkspaceHistoryGetRequest{
			WorkspaceName: workspaceName,
			HistoryId:     entry.Id,
		})
		if err != nil {
			t.Fatalf("%s workspace history get %q round %d: %v", readerContext, entry.Id, round, err)
		}
		if got.Type != rpcapi.PeerRunHistoryEntryTypeGear || got.GearId == nil || *got.GearId != h.ContextPublicKey(writerContext) || !got.ReplayAvailable {
			t.Fatalf("human-review history get round %d = %#v, want replayable gear entry from %s", round, got, writerContext)
		}
		if strings.TrimSpace(got.Text) == "" {
			t.Fatalf("human-review history get text is empty for %q round %d; ASR transcript was not persisted", entry.Id, round)
		}
		historyAudio, historyPackets := readSocialHumanReviewHistoryAudio(t, ctx, reader, workspaceName, entry.Id)
		fmt.Printf("social_human_review history_audio workspace=%s round=%d history_id=%s bytes=%d packets=%d\n", workspaceName, round, entry.Id, len(historyAudio), len(historyPackets))
		play, err := reader.PlayServerRunWorkspaceHistory(ctx, "social.human_review.history.play", rpcapi.ServerPlayRunWorkspaceHistoryRequest{HistoryId: entry.Id})
		if err != nil {
			t.Fatalf("%s workspace history play %q round %d: %v", readerContext, entry.Id, round, err)
		}
		if play == nil || !play.Accepted {
			t.Fatalf("workspace history play round %d = %#v, want accepted", round, play)
		}
		fmt.Printf("social_human_review replay_start workspace=%s round=%d history_id=%s state=%s\n", workspaceName, round, entry.Id, play.State)
		packets, err := playback.PlayReplay(ctx, readerOut)
		if err != nil {
			t.Fatalf("play replayed social history %q round %d: %v", entry.Id, round, err)
		}
		if packets == 0 {
			t.Fatalf("history replay %q round %d produced no audio packets", entry.Id, round)
		}
		fmt.Printf("social_human_review replay_done workspace=%s round=%d history_id=%s packets=%d\n", workspaceName, round, entry.Id, packets)
	}
	_ = writerStream.CloseWithError(io.EOF)
}

func readSocialHumanReviewHistoryAudio(t *testing.T, ctx context.Context, client *gizcli.Client, workspaceName, historyID string) ([]byte, [][]byte) {
	t.Helper()

	var out bytes.Buffer
	result, err := client.GetWorkspaceHistoryAudio(ctx, "social.human_review.history.audio.get", rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: workspaceName,
		HistoryId:     historyID,
	}, &out)
	if err != nil {
		t.Fatalf("workspace history audio get %q: %v", historyID, err)
	}
	if result.Bytes <= 0 || out.Len() == 0 {
		t.Fatalf("workspace history audio get %q returned empty audio: %+v", historyID, result)
	}
	return out.Bytes(), socialHumanReviewOggOpusPackets(t, out.Bytes())
}

func synthesizeSocialHumanReviewSpeech(t *testing.T, ctx context.Context, client *gizcli.Client, text string) ([]byte, [][]byte) {
	t.Helper()

	httpClient := client.HTTPClient(gizcli.ServicePeerOpenAI)
	httpClient.Timeout = 90 * time.Second
	body, err := json.Marshal(map[string]string{
		"input":           text,
		"model":           socialHumanReviewTTSModel,
		"voice":           socialHumanReviewVoiceResource,
		"response_format": "opus",
	})
	if err != nil {
		t.Fatalf("marshal social human-review speech request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gizclaw/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create social human-review speech request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer gizclaw-peer")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("synthesize social human-review speech: %v", err)
	}
	defer resp.Body.Close()
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read social human-review speech: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("synthesize social human-review speech status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(audio)))
	}
	if len(audio) == 0 {
		t.Fatal("social human-review speech returned empty audio")
	}
	packets := socialHumanReviewOggOpusPackets(t, audio)
	return audio, packets
}

func socialHumanReviewOggOpusPackets(t *testing.T, audio []byte) [][]byte {
	t.Helper()

	if !bytes.HasPrefix(audio, []byte("OggS")) {
		t.Fatalf("social human-review speech returned unsupported audio container; got prefix %q", string(audio[:min(len(audio), 4)]))
	}
	var packets [][]byte
	for packet, err := range codecconv.OggOpusPackets(bytes.NewReader(audio)) {
		if err != nil {
			t.Fatalf("read social human-review ogg opus packets: %v", err)
		}
		packets = append(packets, packet)
	}
	if len(packets) == 0 {
		t.Fatal("social human-review speech returned no opus packets")
	}
	return packets
}

func ensureSocialHumanReviewWorkspace(t *testing.T, ctx context.Context, client interface {
	SetServerRunWorkspace(context.Context, string, rpcapi.ServerSetRunWorkspaceRequest) (*rpcapi.ServerSetRunWorkspaceResponse, error)
	ReloadServerRunWorkspace(context.Context, string) (*rpcapi.ServerReloadRunWorkspaceResponse, error)
}, contextName, workspaceName string) {
	t.Helper()

	if _, err := client.SetServerRunWorkspace(ctx, "social.human_review.workspace.set", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspaceName}); err != nil {
		t.Fatalf("%s set run workspace %q: %v", contextName, workspaceName, err)
	}
	state, err := client.ReloadServerRunWorkspace(ctx, "social.human_review.workspace.reload")
	if err != nil {
		t.Fatalf("%s reload run workspace %q: %v", contextName, workspaceName, err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning {
		t.Fatalf("%s reload workspace state = %#v", contextName, state)
	}
}

func waitForWorkspaceHistoryReplayableGear(t *testing.T, ctx context.Context, client interface {
	ListWorkspaceHistory(context.Context, string, rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error)
}, workspaceName, gearID string, seen map[string]struct{}) rpcapi.PeerRunHistoryEntry {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	limit := 20
	desc := rpcapi.WorkspaceHistoryListRequestOrderDesc
	var lastErr error
	for {
		list, err := client.ListWorkspaceHistory(ctx, "social.human_review.history.list", rpcapi.WorkspaceHistoryListRequest{
			WorkspaceName: workspaceName,
			Order:         &desc,
			Limit:         &limit,
		})
		if err == nil {
			for _, item := range list.Items {
				if _, ok := seen[item.Id]; ok {
					continue
				}
				if item.Type == rpcapi.PeerRunHistoryEntryTypeGear && item.GearId != nil && *item.GearId == gearID && item.ReplayAvailable {
					return item
				}
			}
			lastErr = nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			t.Fatalf("replayable gear history for %s not found in workspace %q, last error: %v", gearID, workspaceName, lastErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type socialHumanReviewChunkPusher interface {
	Push(context.Context, *genx.MessageChunk) error
}

func pushSocialHumanReviewAudioTurn(ctx context.Context, stream socialHumanReviewChunkPusher, packets [][]byte) error {
	const streamID = "audio"
	const label = "input"
	if stream == nil {
		return io.ErrClosedPipe
	}
	if err := stream.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: label,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, BeginOfStream: true},
	}); err != nil {
		return err
	}
	timestamp := time.Now().UnixMilli()
	for i, packet := range packets {
		packet = append([]byte(nil), packet...)
		if err := stream.Push(ctx, &genx.MessageChunk{
			Role: genx.RoleUser,
			Name: label,
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, Timestamp: timestamp},
		}); err != nil {
			return err
		}
		timestamp += 20
		if i+1 < len(packets) {
			if err := socialHumanReviewSleep(ctx, 20*time.Millisecond); err != nil {
				return err
			}
		}
	}
	if err := stream.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: label,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "input", Timestamp: timestamp, EndOfStream: true},
	}); err != nil {
		return err
	}
	if err := socialHumanReviewSleep(ctx, 750*time.Millisecond); err != nil {
		return err
	}
	return nil
}

func socialHumanReviewSleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return nil
}

type socialHumanReviewPlayback struct {
	stream            *portaudio.PlaybackStream
	frameSize         int
	underflowWarnings int
}

func newSocialHumanReviewPlayback(t *testing.T) *socialHumanReviewPlayback {
	t.Helper()

	stream, err := portaudio.OpenPlaybackConfig(portaudio.StreamConfig{
		DeviceID:        portaudio.DefaultDeviceID,
		SampleRate:      socialHumanReviewSampleRate,
		Channels:        2,
		FramesPerBuffer: socialHumanReviewFrameSize,
	})
	if err != nil {
		t.Fatalf("open social human-review playback: %v", err)
	}
	return &socialHumanReviewPlayback{
		stream:    stream,
		frameSize: socialHumanReviewFrameSize,
	}
}

func (p *socialHumanReviewPlayback) Close() error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.stream != nil {
		errs = append(errs, p.stream.Close())
	}
	return errors.Join(errs...)
}

func (p *socialHumanReviewPlayback) PlayOpusPackets(label string, packets [][]byte) error {
	if p == nil {
		return nil
	}
	decoder, err := opus.NewDecoder(socialHumanReviewSampleRate, 1)
	if err != nil {
		return fmt.Errorf("create social human-review opus decoder: %w", err)
	}
	defer decoder.Close()
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		samples, err := decoder.Decode(packet, p.frameSize, false)
		if err != nil {
			return err
		}
		if err := p.writePCM(label, socialHumanReviewMonoToStereoPCM16LE(samples)); err != nil {
			return err
		}
	}
	return nil
}

func (p *socialHumanReviewPlayback) PlayReplay(ctx context.Context, stream genx.Stream) (int, error) {
	if p == nil {
		return 0, nil
	}
	decoder, err := opus.NewDecoder(socialHumanReviewSampleRate, 1)
	if err != nil {
		return 0, fmt.Errorf("create social human-review opus decoder: %w", err)
	}
	defer decoder.Close()
	boundStreamID := ""
	packets := 0
	for {
		chunk, err := nextSocialHumanReviewChunk(ctx, stream)
		if err != nil {
			return packets, err
		}
		if socialHumanReviewReplayStreamChunk(chunk, &boundStreamID) {
			if blob, ok := chunk.Part.(*genx.Blob); ok && strings.EqualFold(strings.TrimSpace(blob.MIMEType), "audio/opus") {
				if len(blob.Data) > 0 {
					samples, err := decoder.Decode(blob.Data, p.frameSize, false)
					if err != nil {
						return packets, err
					}
					if err := p.writePCM("history replay", socialHumanReviewMonoToStereoPCM16LE(samples)); err != nil {
						return packets, err
					}
					packets++
					continue
				}
				if chunk.IsEndOfStream() && packets > 0 {
					return packets, nil
				}
			}
			continue
		}
	}
}

func (p *socialHumanReviewPlayback) writePCM(label string, data []byte) error {
	for attempt := 0; ; attempt++ {
		n, err := p.stream.Write(data)
		if err != nil {
			if isSocialHumanReviewPortAudioUnderflow(err) && attempt < socialHumanReviewOutputUnderflowRetries {
				p.reportUnderflow(err, true)
				time.Sleep(20 * time.Millisecond)
				continue
			}
			if isSocialHumanReviewPortAudioUnderflow(err) {
				p.reportUnderflow(err, false)
				return nil
			}
			return fmt.Errorf("%s: %w", label, err)
		}
		if n != len(data) {
			return fmt.Errorf("%s: short playback write %d/%d", label, n, len(data))
		}
		return nil
	}
}

func (p *socialHumanReviewPlayback) reportUnderflow(err error, retry bool) {
	p.underflowWarnings++
	if p.underflowWarnings <= 3 {
		fmt.Printf("social_human_review warning=%q retry=%t\n", err.Error(), retry)
	} else if p.underflowWarnings == 4 {
		fmt.Printf("social_human_review warning=%q retry=%t suppressed=true\n", err.Error(), retry)
	}
}

func isSocialHumanReviewPortAudioUnderflow(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Output underflowed")
}

func nextSocialHumanReviewChunk(ctx context.Context, stream genx.Stream) (*genx.MessageChunk, error) {
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
		if got.err != nil {
			return nil, got.err
		}
		return got.chunk, nil
	case <-ctx.Done():
		_ = stream.CloseWithError(ctx.Err())
		return nil, ctx.Err()
	}
}

func socialHumanReviewReplayStreamChunk(chunk *genx.MessageChunk, boundStreamID *string) bool {
	if chunk == nil || chunk.Ctrl == nil {
		return false
	}
	streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
	if boundStreamID != nil && strings.TrimSpace(*boundStreamID) != "" {
		return streamID == *boundStreamID
	}
	if !strings.HasPrefix(streamID, "history-replay-") {
		return false
	}
	if boundStreamID != nil {
		*boundStreamID = streamID
	}
	return true
}

func socialHumanReviewMonoToStereoPCM16LE(samples []int16) []byte {
	data := make([]byte, len(samples)*4)
	for i, sample := range samples {
		v := uint16(sample)
		binary.LittleEndian.PutUint16(data[i*4:], v)
		binary.LittleEndian.PutUint16(data[i*4+2:], v)
	}
	return data
}
