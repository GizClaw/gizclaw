//go:build gizclaw_e2e

package gameplay_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestGameplayAdoptDriveAndPetWorkspace(t *testing.T) {
	env := newIsolatedGameplayHarness(t)
	petName := "e2e-pet-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	adopted, err := env.peer.AdoptPet(env.ctx, "gameplay.pet.adopt", rpcapi.RuntimeAdoptRequest{
		DisplayName: "E2E Pet",
		Name:        petName,
	})
	if err != nil {
		t.Fatalf("pet.adopt: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.peer.DeletePet(env.ctx, "gameplay.pet.delete.cleanup", rpcapi.ServerPetDeleteRequest{Name: adopted.Pet.Name})
	})
	assertAdoptedStarterPet(t, adopted.Pet)
	if adopted.Points.Balance != 90 || adopted.Transaction.Delta != -10 {
		t.Fatalf("pet.adopt points/transaction = %#v %#v", adopted.Points, adopted.Transaction)
	}
	replayed, err := env.peer.AdoptPet(env.ctx, "gameplay.pet.adopt.replay", rpcapi.RuntimeAdoptRequest{
		DisplayName: "Ignored Replay Name",
		Name:        petName,
	})
	if err != nil {
		t.Fatalf("pet.adopt replay: %v", err)
	}
	if replayed.Pet.Name != adopted.Pet.Name || replayed.Pet.DisplayName != adopted.Pet.DisplayName || replayed.Transaction.Name != adopted.Transaction.Name || replayed.Points.Balance != adopted.Points.Balance {
		t.Fatalf("pet.adopt replay = %#v, want Pet name %q, display name %q, transaction name %q, and Points balance %d", replayed, adopted.Pet.Name, adopted.Pet.DisplayName, adopted.Transaction.Name, adopted.Points.Balance)
	}
	workspace, err := env.peer.GetWorkspace(env.ctx, "gameplay.pet.workspace.get", rpcapi.WorkspaceGetRequest{Name: adopted.Pet.WorkspaceName})
	if err != nil {
		t.Fatalf("workspace.get pet workspace: %v", err)
	}
	if workspace.Value.Name != adopted.Pet.WorkspaceName || workspace.Value.WorkflowName != "pet" {
		t.Fatalf("pet workspace = %#v", workspace)
	}
	if workspace.Value.Parameters != nil {
		t.Fatalf("pet workspace parameters = %#v, want nil", workspace.Value.Parameters)
	}
	tickKey := "gameplay-empty-tick-1"
	tick, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive.empty", rpcapi.ServerPetDriveRequest{
		PetName: adopted.Pet.Name, IdempotencyKey: &tickKey,
	})
	if err != nil {
		t.Fatalf("pet.drive empty: %v", err)
	}
	if tick.GameResult != nil || len(tick.Badges) != 0 || len(tick.Transactions) != 0 || len(tick.RewardGrants) != 0 {
		t.Fatalf("pet.drive empty response = %#v", tick)
	}
	storedTick, err := env.peer.GetPet(env.ctx, "gameplay.pet.get.after-empty", rpcapi.ServerPetGetRequest{Name: adopted.Pet.Name})
	if err != nil || storedTick.StateSettledAt != tick.Pet.StateSettledAt || storedTick.LastActiveAt != adopted.Pet.LastActiveAt {
		t.Fatalf("pet.get after empty = %#v, %v", storedTick, err)
	}
	tickReplay, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive.empty.replay", rpcapi.ServerPetDriveRequest{
		PetName: adopted.Pet.Name, IdempotencyKey: &tickKey,
	})
	if err != nil || tickReplay.Pet.StateSettledAt != tick.Pet.StateSettledAt {
		t.Fatalf("pet.drive empty replay = %#v, %v", tickReplay, err)
	}
	behavior := rpcapi.PetBehaviorBathe
	careKey := "gameplay-care-1"
	care, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive.care", rpcapi.ServerPetDriveRequest{
		PetName: adopted.Pet.Name, Behavior: &behavior, IdempotencyKey: &careKey,
	})
	if err != nil {
		t.Fatalf("pet.drive care: %v", err)
	}
	if care.Pet.Stats.Hygiene != 100 || care.Pet.Stats.Energy != 90 || care.Pet.Progression.Experience != 2 {
		t.Fatalf("pet.drive care Pet = %#v", care.Pet)
	}
	if care.Points.Balance != 90 || len(care.Transactions) != 0 || len(care.RewardGrants) != 1 {
		t.Fatalf("pet.drive care response = %#v", care)
	}

	score := int64(42)
	maxScore := int64(100)
	durationMs := int64(2345)
	difficulty := "normal"
	idempotencyKey := "gameplay-result-1"
	drive, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive", rpcapi.ServerPetDriveRequest{
		PetName: adopted.Pet.Name,
		GameResult: &rpcapi.PetDriveGameResultInput{
			GameName:       "starter-game",
			Score:          &score,
			MaxScore:       &maxScore,
			Difficulty:     &difficulty,
			Outcome:        testStringPtr("win"),
			DurationMs:     &durationMs,
			IdempotencyKey: &idempotencyKey,
		},
	})
	if err != nil {
		t.Fatalf("pet.drive: %v", err)
	}
	if drive.Pet.Progression.Experience < 2 || drive.Pet.Progression.Experience > 27 ||
		drive.Pet.Stats.Energy < 80 || drive.Pet.Stats.Energy > 80.1 {
		t.Fatalf("pet.drive pet = %#v reward_grants = %#v", drive.Pet, drive.RewardGrants)
	}
	if drive.Points.Balance != 80 {
		t.Fatalf("pet.drive points = %#v", drive.Points)
	}
	if drive.GameResult == nil || drive.GameResult.GameDefName != "starter-game" || drive.GameResult.Score == nil || *drive.GameResult.Score != score {
		t.Fatalf("pet.drive game result = %#v", drive.GameResult)
	}
	if drive.GameResult.MaxScore == nil || *drive.GameResult.MaxScore != maxScore || drive.GameResult.DurationMs == nil || *drive.GameResult.DurationMs != durationMs || drive.GameResult.IdempotencyKey == nil || *drive.GameResult.IdempotencyKey != idempotencyKey {
		t.Fatalf("pet.drive game result details = %#v", drive.GameResult)
	}
	if len(drive.RewardGrants) != 1 || drive.RewardGrants[0].PointsDelta != 0 || drive.RewardGrants[0].PetExpDelta < 0 || drive.RewardGrants[0].PetExpDelta > 25 {
		t.Fatalf("pet.drive reward grants = %#v", drive.RewardGrants)
	}
	if len(drive.Transactions) != 1 || drive.Transactions[0].Delta != -10 {
		t.Fatalf("pet.drive transactions = %#v", drive.Transactions)
	}
	duplicate, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive.duplicate", rpcapi.ServerPetDriveRequest{
		PetName: adopted.Pet.Name,
		GameResult: &rpcapi.PetDriveGameResultInput{
			GameName:       "starter-game",
			IdempotencyKey: &idempotencyKey,
		},
	})
	if err != nil || duplicate.GameResult == nil || duplicate.GameResult.Name != drive.GameResult.Name || duplicate.Points.Balance != drive.Points.Balance {
		t.Fatalf("duplicate game result = %#v, %v", duplicate, err)
	}

	pets, err := env.peer.ListPets(env.ctx, "gameplay.pet.list", rpcapi.ServerPetListRequest{})
	if err != nil {
		t.Fatalf("pet.list: %v", err)
	}
	requirePetName(t, pets.Items, adopted.Pet.Name)

	pointsTransactions, err := env.peer.ListPointsTransactions(env.ctx, "gameplay.points.transactions.list", rpcapi.ServerPointsTransactionListRequest{})
	if err != nil {
		t.Fatalf("points.transactions.list: %v", err)
	}
	requirePointsTransactionName(t, pointsTransactions.Items, adopted.Transaction.Name)

	results, err := env.peer.ListGameResults(env.ctx, "gameplay.game_result.list", rpcapi.ServerGameResultListRequest{})
	if err != nil {
		t.Fatalf("game_result.list: %v", err)
	}
	requireGameResultName(t, results.Items, drive.GameResult.Name)

	grants, err := env.peer.ListRewardGrants(env.ctx, "gameplay.reward_grant.list", rpcapi.ServerRewardGrantListRequest{})
	if err != nil {
		t.Fatalf("reward_grant.list: %v", err)
	}
	requireRewardGrantName(t, grants.Items, drive.RewardGrants[0].Name)
}

func TestGameplayPetWorkspaceAudioHistory(t *testing.T) {
	env := newSetupGameplayHarness(t, "client-gameplay-chat")
	petName := "e2e-chat-pet-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	adopted, err := env.peer.AdoptPet(env.ctx, "gameplay.chat.pet.adopt", rpcapi.RuntimeAdoptRequest{
		DisplayName: "Chat Pet",
		Name:        petName,
	})
	if err != nil {
		t.Fatalf("pet.adopt for chat: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.peer.DeletePet(env.ctx, "gameplay.chat.pet.delete.cleanup", rpcapi.ServerPetDeleteRequest{Name: adopted.Pet.Name})
	})
	assertAdoptedStarterPet(t, adopted.Pet)
	if adopted.Pet.DisplayName != "Chat Pet" {
		t.Fatalf("adopted chat pet = %#v", adopted.Pet)
	}
	workspace, err := env.peer.GetWorkspace(env.ctx, "gameplay.pet.audio.workspace.get", rpcapi.WorkspaceGetRequest{Name: adopted.Pet.WorkspaceName})
	if err != nil {
		t.Fatalf("get pet audio workspace: %v", err)
	}
	if workspace.Value.Parameters != nil {
		t.Fatalf("pet audio workspace parameters = %#v, want nil", workspace.Value.Parameters)
	}

	if err := selectGameplayWorkspace(env.ctx, env.peer, adopted.Pet.WorkspaceName); err != nil {
		t.Fatalf("select pet workspace %q: %v", adopted.Pet.WorkspaceName, err)
	}
	stream, err := env.peer.OpenPeerStream(512)
	if err != nil {
		t.Fatalf("open pet workspace audio stream: %v", err)
	}
	defer stream.Close()

	known := snapshotGameplayHistory(t, env.ctx, env.peer, adopted.Pet.WorkspaceName)
	utterances := []string{"你好小爪，我今天来看看你。", "小爪，我们继续聊下一句话。"}
	entries := make([]rpcapi.PeerRunHistoryEntry, 0, len(utterances))
	for round, utterance := range utterances {
		var responseErr error
		for attempt := 1; attempt <= 3; attempt++ {
			packets := synthesizeGameplayOpus(t, env, "volc-bigtts", "pet", utterance)
			streamID := "gameplay-pet-audio-" + strconv.Itoa(round+1) + "-" + strconv.Itoa(attempt)
			sendGameplayAudioTurn(t, env.ctx, stream, streamID, packets)
			responseErr = waitForGameplayAssistantMediaResponse(env.ctx, stream, streamID)
			retryable := isRetryableGameplayResponseError(responseErr)
			result := "pass"
			if responseErr != nil {
				result = "fail"
			}
			t.Logf("gameplay_audio_round round=%d attempt=%d result=%s retryable=%t error=%v", round+1, attempt, result, retryable, responseErr)
			if responseErr == nil || !retryable {
				break
			}
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}
		if responseErr != nil {
			t.Fatalf("pet audio round %d failed after retry: %v", round+1, responseErr)
		}

		entry := waitForSingleGameplayTranscript(t, env.ctx, env.peer, adopted.Pet.WorkspaceName, known)
		if entry.Name == "" || entry.Text == "" || !entry.ReplayAvailable {
			t.Fatalf("pet audio history round %d = %#v, want combined replayable transcript", round+1, entry)
		}
		if round > 0 && entry.Name == entries[round-1].Name {
			t.Fatalf("pet audio history round %d reused entry %q", round+1, entry.Name)
		}
		assertGameplayHistoryReplayAudio(t, env.ctx, env.peer, stream, entry)
		known[entry.Name] = entry
		entries = append(entries, entry)
	}

	first, err := env.peer.GetWorkspaceHistory(env.ctx, "gameplay.pet.history.first.get", rpcapi.WorkspaceHistoryGetRequest{
		WorkspaceName: adopted.Pet.WorkspaceName,
		HistoryName:   entries[0].Name,
	})
	if err != nil {
		t.Fatalf("get first pet audio history after second turn: %v", err)
	}
	if first.Text != entries[0].Text || !first.ReplayAvailable {
		t.Fatalf("first pet audio history changed after second turn: before=%#v after=%#v", entries[0], first)
	}
}

func TestGameplayWorkspaceConversationReward(t *testing.T) {
	env := newSetupGameplayHarness(t, "client-gameplay-workspace-reward")
	petName := "e2e-workspace-reward-pet-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	adopted, err := env.peer.AdoptPet(env.ctx, "gameplay.workspace.reward.pet.adopt", rpcapi.RuntimeAdoptRequest{
		DisplayName: "Reward Pet",
		Name:        petName,
	})
	if err != nil {
		t.Fatalf("pet.adopt for Workspace reward: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.peer.DeletePet(env.ctx, "gameplay.workspace.reward.pet.delete.cleanup", rpcapi.ServerPetDeleteRequest{Name: adopted.Pet.Name})
	})
	if err := selectGameplayWorkspace(env.ctx, env.peer, adopted.Pet.WorkspaceName); err != nil {
		t.Fatalf("select Workspace reward Pet workspace %q: %v", adopted.Pet.WorkspaceName, err)
	}
	stream, err := env.peer.OpenPeerStream(512)
	if err != nil {
		t.Fatalf("open Workspace reward stream: %v", err)
	}
	defer stream.Close()

	known := snapshotGameplayHistory(t, env.ctx, env.peer, adopted.Pet.WorkspaceName)
	packets := synthesizeGameplayOpus(
		t,
		env,
		"volc-bigtts",
		"pet",
		"今天我学会了先提出假设，再用实验数据验证，并根据证据修正结论。",
	)
	streamID := "gameplay-workspace-reward-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	sendGameplayAudioTurn(t, env.ctx, stream, streamID, packets)
	if err := waitForGameplayAssistantMediaResponse(env.ctx, stream, streamID); err != nil {
		t.Fatalf("wait for Workspace reward conversation response: %v", err)
	}
	entry := waitForSingleGameplayTranscript(t, env.ctx, env.peer, adopted.Pet.WorkspaceName, known)
	if entry.Name == "" || entry.Text == "" {
		t.Fatalf("Workspace reward conversation History = %#v", entry)
	}

	var rewardEvent *eventpb.GameplayRewardUpdated
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	for rewardEvent == nil {
		select {
		case event := <-stream.ResourceEvents():
			if event != nil && event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_GAMEPLAY_REWARD_UPDATED {
				rewardEvent = event.GetGameplayRewardUpdated()
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for debounced Gameplay reward event")
		}
	}
	if rewardEvent.WorkspaceName != adopted.Pet.WorkspaceName || rewardEvent.RewardGrantName == "" {
		t.Fatalf("Gameplay reward event = %#v", rewardEvent)
	}
	reward, err := env.peer.GetRewardGrant(env.ctx, "gameplay.workspace.reward.get", rpcapi.ServerRewardGrantGetRequest{
		Name: rewardEvent.RewardGrantName,
	})
	if err != nil {
		t.Fatalf("get Workspace RewardGrant: %v", err)
	}
	if reward.SourceType != "workspace_history_window" ||
		reward.PetName != nil || reward.GameResultName != nil ||
		reward.PetExpDelta != 0 || reward.PointsDelta <= 0 {
		t.Fatalf("Workspace RewardGrant = %#v", reward)
	}
}

func assertAdoptedStarterPet(t *testing.T, pet rpcapi.Pet) {
	t.Helper()
	if pet.PetDefName != "starter-pet" || pet.DisplayName == "" || pet.WorkspaceName == "" {
		t.Fatalf("adopted pet = %#v", pet)
	}
	if pet.RuntimeProfileName != "default-gameplay" {
		t.Fatalf("adopted pet RuntimeProfile = %q", pet.RuntimeProfileName)
	}
}

func selectGameplayWorkspace(ctx context.Context, client interface {
	SetServerRunWorkspace(context.Context, string, rpcapi.ServerSetRunWorkspaceRequest) (*rpcapi.ServerSetRunWorkspaceResponse, error)
	ReloadServerRunWorkspace(context.Context, string) (*rpcapi.ServerReloadRunWorkspaceResponse, error)
	GetServerRunWorkspace(context.Context, string) (*rpcapi.ServerGetRunWorkspaceResponse, error)
}, workspaceName string) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := client.SetServerRunWorkspace(ctx, "gameplay.workspace.set", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspaceName}); err != nil {
			return err
		}
		if _, err := client.ReloadServerRunWorkspace(ctx, "gameplay.workspace.reload"); err != nil {
			if time.Now().After(deadline) {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		state, err := client.GetServerRunWorkspace(ctx, "gameplay.workspace.get")
		if err != nil {
			return err
		}
		if state.RuntimeState == rpcapi.PeerRunStatusStateRunning && state.WorkspaceName == workspaceName {
			return nil
		}
		if state.RuntimeState == rpcapi.PeerRunStatusStateError {
			message := ""
			if state.Message != nil {
				message = *state.Message
			}
			return &workspaceStartError{workspace: workspaceName, message: message}
		}
		if time.Now().After(deadline) {
			return &workspaceStartError{workspace: workspaceName, message: string(state.RuntimeState)}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

type workspaceStartError struct {
	workspace string
	message   string
}

func (e *workspaceStartError) Error() string {
	return "workspace " + e.workspace + " did not start: " + e.message
}

func requirePetName(t *testing.T, items []rpcapi.Pet, name string) {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			return
		}
	}
	t.Fatalf("pet %q not found in %#v", name, items)
}

func requirePointsTransactionName(t *testing.T, items []rpcapi.PointsTransaction, name string) rpcapi.PointsTransaction {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("points transaction %q not found in %#v", name, items)
	return rpcapi.PointsTransaction{}
}

func requireGameResultName(t *testing.T, items []rpcapi.GameResult, name string) rpcapi.GameResult {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("game result %q not found in %#v", name, items)
	return rpcapi.GameResult{}
}

func requireRewardGrantName(t *testing.T, items []rpcapi.RewardGrant, name string) rpcapi.RewardGrant {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("reward grant %q not found in %#v", name, items)
	return rpcapi.RewardGrant{}
}

func testStringPtr(v string) *string {
	return &v
}
