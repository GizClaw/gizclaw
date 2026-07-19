//go:build gizclaw_e2e

package social_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestSocialRealtimeHistoryRPC(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("opus runtime is unavailable for social realtime history")
	}
	requireSocialHumanReviewProviderEnv(t)

	h := newSocialHumanReviewHarness(t)
	peerB := h.ContextPublicKey("peer-b")
	peerC := h.ContextPublicKey("peer-c")
	realtime := apitypes.WorkspaceInputModeRealtime

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
	readerInput := newBlockingStream()
	readerOut, err := reader.Transform(ctx, readerInput)
	if err != nil {
		t.Fatalf("%s open realtime reader stream: %v", readerContext, err)
	}
	defer readerOut.Close()
	defer readerInput.CloseWithError(io.EOF)

	ensureSocialHumanReviewWorkspace(t, ctx, writer, writerContext, workspaceName)
	writerStream, err := writer.OpenPeerStream(64)
	if err != nil {
		t.Fatalf("%s open realtime writer stream: %v", writerContext, err)
	}
	defer writerStream.Close()
	if err := pushSocialRealtimeAudioBOS(ctx, writerStream); err != nil {
		t.Fatalf("%s start realtime audio stream: %v", writerContext, err)
	}

	seenHistoryIDs := make(map[string]struct{}, len(texts))
	entries := make([]rpcapi.PeerRunHistoryEntry, 0, len(texts))
	timestamp := time.Now().UnixMilli()
	for i, text := range texts {
		round := i + 1
		updatedCh := waitForWorkspaceHistoryUpdated(readerOut)
		_, inputPackets := synthesizeSocialHumanReviewSpeech(t, ctx, writer, text)
		t.Logf("social realtime input ready workspace=%s round=%d text=%q packets=%d", workspaceName, round, text, len(inputPackets))
		var err error
		timestamp, err = pushSocialRealtimeAudioPackets(ctx, writerStream, inputPackets, timestamp)
		if err != nil {
			t.Fatalf("%s send realtime audio round %d: %v", writerContext, round, err)
		}
		if err := socialHumanReviewSleep(ctx, 1100*time.Millisecond); err != nil {
			t.Fatalf("wait for realtime ASR boundary round %d: %v", round, err)
		}

		select {
		case err := <-updatedCh:
			if err != nil {
				t.Fatalf("%s did not observe realtime history update round %d: %v", readerContext, round, err)
			}
		case <-ctx.Done():
			t.Fatalf("%s did not observe realtime history update round %d before timeout: %v", readerContext, round, ctx.Err())
		}

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
	}
	assertWorkspaceHistoryResumeOrder(t, ctx, reader, workspaceName, entries)
	_ = pushSocialRealtimeAudioEOS(ctx, writerStream, timestamp)
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

func pushSocialRealtimeAudioBOS(ctx context.Context, stream socialHumanReviewChunkPusher) error {
	return stream.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: "input",
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "input", BeginOfStream: true},
	})
}

func pushSocialRealtimeAudioPackets(ctx context.Context, stream socialHumanReviewChunkPusher, packets [][]byte, timestamp int64) (int64, error) {
	if stream == nil {
		return timestamp, io.ErrClosedPipe
	}
	for _, packet := range packets {
		packet = append([]byte(nil), packet...)
		if err := stream.Push(ctx, &genx.MessageChunk{
			Role: genx.RoleUser,
			Name: "input",
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "input", Timestamp: timestamp},
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

func pushSocialRealtimeAudioEOS(ctx context.Context, stream socialHumanReviewChunkPusher, timestamp int64) error {
	if stream == nil {
		return io.ErrClosedPipe
	}
	return stream.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: "input",
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "input", Timestamp: timestamp, EndOfStream: true},
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
