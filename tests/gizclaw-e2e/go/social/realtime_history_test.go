//go:build gizclaw_e2e

package social_test

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const socialRealtimeTailSilence = 4 * time.Second

func TestSocialRealtimeHistoryRPC(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Fatal("opus runtime is unavailable for social realtime history")
	}
	requireSocialHumanReviewProviderEnv(t)

	h := newSocialHumanReviewHarness(t)
	peerB := h.ContextPublicKey("peer-b")
	peerC := h.ContextPublicKey("peer-c")
	realtime := apitypes.WorkspaceInputModeRealtime

	if existing, ok := findFriendByPeer(t, h, "peer-a", peerB); ok {
		mustDeleteFriend(t, h, "peer-a", stringValue(existing.Id))
	}
	requestAB := createFriendByInviteToken(t, h, "peer-a", "peer-b", peerB)
	setSocialChatWorkspaceInputMode(t, h, stringValue(requestAB.WorkspaceName), realtime)
	t.Run("friend direct chat", func(t *testing.T) {
		runSocialRealtimeAudioHistory(t, h, "peer-a", "peer-b", stringValue(requestAB.WorkspaceName), []string{
			"你好，这是实时好友留言第一段。",
			"第二段实时好友留言应该自动切分。",
			"第三段实时好友留言用于验证历史播放。",
		})
	})

	group := mustCreateFriendGroup(t, h, "peer-a", "realtime", "")
	mustAddFriendGroupMember(t, h, "peer-a", stringValue(group.Id), peerB, rpcapi.FriendGroupMemberMutableRoleMember)
	mustAddFriendGroupMember(t, h, "peer-a", stringValue(group.Id), peerC, rpcapi.FriendGroupMemberMutableRoleMember)
	setSocialChatWorkspaceInputMode(t, h, stringValue(group.WorkspaceName), realtime)
	t.Run("group chat", func(t *testing.T) {
		runSocialRealtimeAudioHistory(t, h, "peer-b", "peer-c", stringValue(group.WorkspaceName), []string{
			"你好，这是实时群聊留言第一段。",
			"第二段实时群聊留言应该自动切分。",
			"第三段实时群聊留言用于验证历史播放。",
		})
	})
}

func runSocialRealtimeAudioHistory(t *testing.T, h socialHarness, writerContext, readerContext, workspaceName string, texts []string) {
	t.Helper()
	if len(texts) < 3 {
		t.Fatalf("social realtime history test needs at least 3 utterances, got %d", len(texts))
	}

	writer := h.Client(writerContext)
	reader := h.Client(readerContext)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ensureSocialHumanReviewWorkspace(t, ctx, reader, readerContext, workspaceName)
	readerState, err := reader.GetServerRunWorkspace(ctx, "social.realtime.reader.workspace.initial")
	if err != nil {
		t.Fatalf("%s get initial realtime workspace: %v", readerContext, err)
	}
	if readerState.RuntimeState != rpcapi.PeerRunStatusStateRunning || readerState.StartedAt == nil {
		t.Fatalf("%s initial realtime workspace state = %#v, want running with StartedAt", readerContext, readerState)
	}
	readerStartedAt := *readerState.StartedAt
	readerStream, err := reader.OpenPeerStream(64)
	if err != nil {
		t.Fatalf("%s open realtime reader stream: %v", readerContext, err)
	}
	defer readerStream.Close()
	readerOut := genx.Stream(readerStream)
	readerHistoryIDs := make(map[string]struct{}, 2)
	readerTimestamp := time.Now().UnixMilli()
	readerTimestamp, firstReaderEntry := runSocialRealtimeLifecycleProbe(
		t, ctx, reader, readerStream, workspaceName, h.ContextPublicKey(readerContext),
		"reader-segment-first", "这是第一段实时生命周期验证。", readerTimestamp, readerHistoryIDs,
	)
	assertSocialRealtimeWorkspaceUnchanged(t, ctx, reader, readerContext, workspaceName, readerStartedAt, "first provider-VAD route")

	ensureSocialHumanReviewWorkspace(t, ctx, writer, writerContext, workspaceName)
	writerStream, err := writer.OpenPeerStream(64)
	if err != nil {
		t.Fatalf("%s open realtime writer stream: %v", writerContext, err)
	}
	defer writerStream.Close()
	timestamp := time.Now().UnixMilli()
	if err := pushSocialRealtimeAudioBOS(ctx, writerStream, "writer-segment", timestamp); err != nil {
		t.Fatalf("%s start realtime audio stream: %v", writerContext, err)
	}

	seenHistoryIDs := make(map[string]struct{}, len(texts))
	entries := make([]rpcapi.PeerRunHistoryEntry, 0, len(texts)+2)
	entries = append(entries, firstReaderEntry)
	for i, text := range texts {
		round := i + 1
		timestamp = max(timestamp, time.Now().UnixMilli())
		updatedCh := waitForWorkspaceHistoryUpdated(readerOut)
		_, inputPackets := synthesizeSocialHumanReviewSpeech(t, ctx, writer, text)
		inputPackets = socialRealtimePacketsWithTailSilence(t, inputPackets)
		t.Logf("social realtime input ready workspace=%s round=%d text=%q packets=%d", workspaceName, round, text, len(inputPackets))
		var err error
		timestamp, err = pushSocialRealtimeAudioPackets(ctx, writerStream, "writer-segment", inputPackets, timestamp)
		if err != nil {
			t.Fatalf("%s send realtime audio round %d: %v", writerContext, round, err)
		}

		waitForSocialRealtimeHistoryUpdate(t, ctx, reader, updatedCh, readerContext, "writer round "+strconv.Itoa(round))

		entry := waitForWorkspaceHistoryReplayableGear(t, ctx, reader, workspaceName, h.ContextPublicKey(writerContext), seenHistoryIDs)
		seenHistoryIDs[entry.Id] = struct{}{}
		entries = append(entries, entry)
		got := getSocialRealtimeHistoryEntry(t, ctx, reader, workspaceName, entry.Id, h.ContextPublicKey(writerContext), round)
		_, historyPackets := readSocialHumanReviewHistoryAudio(t, ctx, reader, workspaceName, entry.Id)
		if len(historyPackets) == 0 {
			t.Fatalf("realtime history %q round %d has no audio packets", entry.Id, round)
		}
		play, err := reader.PlayServerRunWorkspaceHistory(ctx, "social.realtime.history.play", rpcapi.ServerPlayRunWorkspaceHistoryRequest{HistoryId: entry.Id})
		if err != nil {
			t.Fatalf("%s realtime history play %q round %d: %v", readerContext, entry.Id, round, err)
		}
		if play == nil || !play.Accepted {
			t.Fatalf("realtime history play round %d = %#v, want accepted", round, play)
		}
		replayPackets := waitForSocialRealtimeHistoryReplay(t, ctx, readerOut, entry.Id, got.Text)
		if replayPackets == 0 {
			t.Fatalf("realtime history replay %q round %d produced no audio packets", entry.Id, round)
		}
		assertSocialRealtimeWorkspaceUnchanged(t, ctx, reader, readerContext, workspaceName, readerStartedAt, "history replay")
		if round == 1 {
			repeat, err := reader.PlayServerRunWorkspaceHistory(ctx, "social.realtime.history.play.repeat", rpcapi.ServerPlayRunWorkspaceHistoryRequest{HistoryId: entry.Id})
			if err != nil {
				t.Fatalf("%s repeat realtime history play %q: %v", readerContext, entry.Id, err)
			}
			if repeat == nil || !repeat.Accepted {
				t.Fatalf("repeat realtime history play = %#v, want accepted", repeat)
			}
			if packets := waitForSocialRealtimeHistoryReplay(t, ctx, readerOut, entry.Id, got.Text); packets == 0 {
				t.Fatalf("repeat realtime history replay %q produced no audio packets", entry.Id)
			}
			if err := pushSocialRealtimeAudioEOS(ctx, readerStream, "reader-segment-first", readerTimestamp); err != nil {
				t.Fatalf("%s close first realtime reader segment: %v", readerContext, err)
			}
			assertSocialRealtimeWorkspaceUnchanged(t, ctx, reader, readerContext, workspaceName, readerStartedAt, "local reader EOS")
			var secondReaderEntry rpcapi.PeerRunHistoryEntry
			readerTimestamp, secondReaderEntry = runSocialRealtimeLifecycleProbe(
				t, ctx, reader, readerStream, workspaceName, h.ContextPublicKey(readerContext),
				"reader-segment-second", "这是第二段实时生命周期验证。", readerTimestamp+20, readerHistoryIDs,
			)
			entries = append(entries, secondReaderEntry)
			assertSocialRealtimeWorkspaceUnchanged(t, ctx, reader, readerContext, workspaceName, readerStartedAt, "second provider-VAD route")
		}
	}
	assertWorkspaceHistoryResumeOrder(t, ctx, reader, workspaceName, entries)
	if err := pushSocialRealtimeAudioEOS(ctx, writerStream, "writer-segment", timestamp); err != nil {
		t.Fatalf("%s close realtime writer segment: %v", writerContext, err)
	}
	if err := pushSocialRealtimeAudioEOS(ctx, readerStream, "reader-segment-second", readerTimestamp); err != nil {
		t.Fatalf("%s close second realtime reader segment: %v", readerContext, err)
	}
	assertSocialRealtimeWorkspaceUnchanged(t, ctx, reader, readerContext, workspaceName, readerStartedAt, "second local reader EOS")
	_ = writerStream.CloseWithError(io.EOF)
}

func getSocialRealtimeHistoryEntry(t *testing.T, ctx context.Context, client *gizcli.Client, workspaceName, historyID, gearID string, round int) *rpcapi.WorkspaceHistoryGetResponse {
	t.Helper()
	got, err := client.GetWorkspaceHistory(ctx, "social.realtime.history.get", rpcapi.WorkspaceHistoryGetRequest{
		WorkspaceName: workspaceName,
		HistoryId:     historyID,
	})
	if err != nil {
		t.Fatalf("workspace history get %q round %d: %v", historyID, round, err)
	}
	if got.Type != rpcapi.PeerRunHistoryEntryTypeGear || got.GearId == nil || *got.GearId != gearID || !got.ReplayAvailable {
		t.Fatalf("realtime history get round %d = %#v, want replayable gear entry from %s", round, got, gearID)
	}
	if strings.TrimSpace(got.Text) == "" {
		t.Fatalf("realtime history get round %d text is empty for %q", round, historyID)
	}
	return got
}

func runSocialRealtimeLifecycleProbe(
	t *testing.T,
	ctx context.Context,
	client *gizcli.Client,
	stream *gizcli.PeerStream,
	workspaceName string,
	gearID string,
	streamID string,
	text string,
	timestamp int64,
	seenHistoryIDs map[string]struct{},
) (int64, rpcapi.PeerRunHistoryEntry) {
	t.Helper()
	timestamp = max(timestamp, time.Now().UnixMilli())
	updatedCh := waitForWorkspaceHistoryUpdated(stream)
	if err := pushSocialRealtimeAudioBOS(ctx, stream, streamID, timestamp); err != nil {
		t.Fatalf("start realtime lifecycle probe %s: %v", streamID, err)
	}
	_, packets := synthesizeSocialHumanReviewSpeech(t, ctx, client, text)
	packets = socialRealtimePacketsWithTailSilence(t, packets)
	nextTimestamp, err := pushSocialRealtimeAudioPackets(ctx, stream, streamID, packets, timestamp)
	if err != nil {
		t.Fatalf("send realtime lifecycle probe %s: %v", streamID, err)
	}
	waitForSocialRealtimeHistoryUpdate(t, ctx, client, updatedCh, gearID, "lifecycle probe "+streamID)
	entry := waitForWorkspaceHistoryReplayableGear(t, ctx, client, workspaceName, gearID, seenHistoryIDs)
	seenHistoryIDs[entry.Id] = struct{}{}
	return nextTimestamp, entry
}

func waitForSocialRealtimeHistoryUpdate(
	t *testing.T,
	ctx context.Context,
	client *gizcli.Client,
	updatedCh <-chan error,
	contextName string,
	phase string,
) {
	t.Helper()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case err := <-updatedCh:
		if err != nil {
			state, stateErr := client.GetServerRunWorkspace(ctx, "social.realtime.history.output_error")
			t.Fatalf("%s %s history output failed: %v; runtime=%#v runtime_error=%v", contextName, phase, err, state, stateErr)
		}
	case <-timer.C:
		state, stateErr := client.GetServerRunWorkspace(ctx, "social.realtime.history.timeout")
		t.Fatalf("%s %s history update timed out; runtime=%#v runtime_error=%v", contextName, phase, state, stateErr)
	case <-ctx.Done():
		state, stateErr := client.GetServerRunWorkspace(context.Background(), "social.realtime.history.context_done")
		t.Fatalf("%s %s context ended before history update: %v; runtime=%#v runtime_error=%v", contextName, phase, ctx.Err(), state, stateErr)
	}
}

func assertSocialRealtimeWorkspaceUnchanged(
	t *testing.T,
	ctx context.Context,
	client *gizcli.Client,
	contextName string,
	workspaceName string,
	startedAt time.Time,
	phase string,
) {
	t.Helper()
	state, err := client.GetServerRunWorkspace(ctx, "social.realtime.reader.workspace."+strings.ReplaceAll(phase, " ", "_"))
	if err != nil {
		t.Fatalf("%s get realtime workspace after %s: %v", contextName, phase, err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning ||
		state.WorkspaceName != workspaceName ||
		state.StartedAt == nil ||
		!state.StartedAt.Equal(startedAt) {
		t.Fatalf("%s workspace after %s = %#v, want same running %q started at %s", contextName, phase, state, workspaceName, startedAt)
	}
}

func pushSocialRealtimeAudioBOS(ctx context.Context, stream socialHumanReviewChunkPusher, streamID string, timestamp int64) error {
	return stream.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: "input",
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "input", Timestamp: timestamp, BeginOfStream: true},
	})
}

func pushSocialRealtimeAudioPackets(ctx context.Context, stream socialHumanReviewChunkPusher, streamID string, packets [][]byte, timestamp int64) (int64, error) {
	if stream == nil {
		return timestamp, io.ErrClosedPipe
	}
	for _, packet := range packets {
		packet = append([]byte(nil), packet...)
		if err := stream.Push(ctx, &genx.MessageChunk{
			Role: genx.RoleUser,
			Name: "input",
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "input", Timestamp: timestamp},
		}); err != nil {
			return timestamp, err
		}
		timestamp += 20
		if err := socialHumanReviewSleep(ctx, 20*time.Millisecond); err != nil {
			return timestamp, err
		}
	}
	return timestamp, nil
}

func socialRealtimePacketsWithTailSilence(t *testing.T, packets [][]byte) [][]byte {
	t.Helper()
	decoder, err := opus.NewDecoder(socialHumanReviewSampleRate, 1)
	if err != nil {
		t.Fatalf("create realtime Opus decoder: %v", err)
	}
	defer decoder.Close()

	pcm := make([]int16, 0, (len(packets)+int(socialRealtimeTailSilence/(20*time.Millisecond)))*socialHumanReviewFrameSize)
	for _, packet := range packets {
		frame, err := decoder.Decode(packet, socialHumanReviewFrameSize, false)
		if err != nil {
			t.Fatalf("decode realtime Opus packet: %v", err)
		}
		pcm = append(pcm, frame...)
	}
	pcm = append(pcm, make([]int16, int(socialRealtimeTailSilence*socialHumanReviewSampleRate/time.Second))...)

	encoder, err := opus.NewEncoder(socialHumanReviewSampleRate, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("create realtime Opus encoder: %v", err)
	}
	defer encoder.Close()

	if len(pcm)%socialHumanReviewFrameSize != 0 {
		pcm = append(pcm, make([]int16, socialHumanReviewFrameSize-len(pcm)%socialHumanReviewFrameSize)...)
	}
	out := make([][]byte, 0, len(pcm)/socialHumanReviewFrameSize)
	for offset := 0; offset < len(pcm); offset += socialHumanReviewFrameSize {
		packet, err := encoder.Encode(pcm[offset:offset+socialHumanReviewFrameSize], socialHumanReviewFrameSize)
		if err != nil {
			t.Fatalf("encode realtime Opus frame %d: %v", len(out), err)
		}
		if len(packet) == 0 {
			t.Fatalf("encode realtime Opus frame %d returned no data", len(out))
		}
		out = append(out, packet)
	}
	if len(out) == 0 {
		t.Fatal("realtime Opus input produced no packets")
	}
	return out
}

func pushSocialRealtimeAudioEOS(ctx context.Context, stream socialHumanReviewChunkPusher, streamID string, timestamp int64) error {
	if stream == nil {
		return io.ErrClosedPipe
	}
	return stream.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: "input",
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "input", Timestamp: timestamp, EndOfStream: true},
	})
}

func waitForSocialRealtimeHistoryReplay(t *testing.T, ctx context.Context, stream genx.Stream, historyID string, wantText string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	boundStreamID := ""
	var gotText strings.Builder
	textEOS := false
	audioEOS := false
	audioPackets := 0
	for {
		chunk, err := nextWorkspaceHistoryReplayChunk(ctx, stream)
		if err != nil {
			t.Fatalf("realtime history replay %q stream read: %v", historyID, err)
		}
		if !socialChatReplayStreamChunk(chunk, &boundStreamID) {
			continue
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Error) != "" {
			t.Fatalf("realtime history replay %q stream %q returned error %q", historyID, boundStreamID, chunk.Ctrl.Error)
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			gotText.WriteString(string(part))
			if chunk.IsEndOfStream() {
				textEOS = true
			}
		case *genx.Blob:
			if part != nil && strings.EqualFold(strings.TrimSpace(part.MIMEType), "audio/opus") && len(part.Data) > 0 {
				audioPackets++
			}
			if chunk.IsEndOfStream() {
				audioEOS = true
			}
		}
		if textEOS && audioEOS {
			if gotText.String() != wantText {
				t.Fatalf("realtime history replay %q text = %q, want %q", historyID, gotText.String(), wantText)
			}
			return audioPackets
		}
	}
}
