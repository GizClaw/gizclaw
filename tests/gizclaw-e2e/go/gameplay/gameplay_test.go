//go:build gizclaw_e2e

package gameplay_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestGameplayAdoptDriveAndPetWorkspace(t *testing.T) {
	env := newIsolatedGameplayHarness(t)

	adopted, err := env.peer.AdoptPet(env.ctx, "gameplay.pet.adopt", rpcapi.RuntimeAdoptRequest{
		DisplayName: testStringPtr("E2E Pet"),
	})
	if err != nil {
		t.Fatalf("pet.adopt: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.peer.DeletePet(env.ctx, "gameplay.pet.delete.cleanup", rpcapi.ServerPetDeleteRequest{Id: adopted.Pet.Id})
	})
	assertAdoptedStarterPet(t, adopted.Pet)
	if adopted.Points.Balance != 90 || adopted.Transaction.Delta != -10 {
		t.Fatalf("pet.adopt points/transaction = %#v %#v", adopted.Points, adopted.Transaction)
	}
	workspace, err := env.peer.GetWorkspace(env.ctx, "gameplay.pet.workspace.get", rpcapi.WorkspaceGetRequest{Name: adopted.Pet.WorkspaceName})
	if err != nil {
		t.Fatalf("workspace.get pet workspace: %v", err)
	}
	if workspace.Value.Name != adopted.Pet.WorkspaceName || workspace.Value.WorkflowAlias != "pet-care" {
		t.Fatalf("pet workspace = %#v", workspace)
	}
	if workspace.Value.Parameters == nil {
		t.Fatalf("pet workspace parameters = nil")
	}
	petParameters, err := workspace.Value.Parameters.AsPetWorkspaceParameters()
	if err != nil {
		t.Fatalf("pet workspace parameters: %v", err)
	}
	if petParameters.AgentType != rpcapi.PetWorkspaceParametersAgentTypePet || petParameters.Voice.VoiceId != "pet" {
		t.Fatalf("pet workspace parameters = %#v", petParameters)
	}
	tickKey := "gameplay-empty-tick-1"
	tick, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive.empty", rpcapi.ServerPetDriveRequest{
		PetId: adopted.Pet.Id, IdempotencyKey: &tickKey,
	})
	if err != nil {
		t.Fatalf("pet.drive empty: %v", err)
	}
	if tick.GameResult != nil || len(tick.Badges) != 0 || len(tick.Transactions) != 0 || len(tick.RewardGrants) != 0 {
		t.Fatalf("pet.drive empty response = %#v", tick)
	}
	storedTick, err := env.peer.GetPet(env.ctx, "gameplay.pet.get.after-empty", rpcapi.ServerPetGetRequest{Id: adopted.Pet.Id})
	if err != nil || storedTick.StateSettledAt != tick.Pet.StateSettledAt || storedTick.LastActiveAt != adopted.Pet.LastActiveAt {
		t.Fatalf("pet.get after empty = %#v, %v", storedTick, err)
	}
	tickReplay, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive.empty.replay", rpcapi.ServerPetDriveRequest{
		PetId: adopted.Pet.Id, IdempotencyKey: &tickKey,
	})
	if err != nil || tickReplay.Pet.StateSettledAt != tick.Pet.StateSettledAt {
		t.Fatalf("pet.drive empty replay = %#v, %v", tickReplay, err)
	}
	behavior := rpcapi.PetBehaviorBathe
	careKey := "gameplay-care-1"
	care, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive.care", rpcapi.ServerPetDriveRequest{
		PetId: adopted.Pet.Id, Behavior: &behavior, IdempotencyKey: &careKey,
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
		PetId: adopted.Pet.Id,
		GameResult: &rpcapi.PetDriveGameResultInput{
			GameDefId:      "game-starter",
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
	if drive.Pet.Progression.Experience < 2 || drive.Pet.Progression.Experience > 27 || drive.Pet.Stats.Energy != 80 {
		t.Fatalf("pet.drive pet = %#v reward_grants = %#v", drive.Pet, drive.RewardGrants)
	}
	if drive.Points.Balance != 80 {
		t.Fatalf("pet.drive points = %#v", drive.Points)
	}
	if drive.GameResult == nil || drive.GameResult.GameDefId != "game-starter" || drive.GameResult.Score == nil || *drive.GameResult.Score != score {
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
		PetId: adopted.Pet.Id,
		GameResult: &rpcapi.PetDriveGameResultInput{
			GameDefId:      "game-starter",
			IdempotencyKey: &idempotencyKey,
		},
	})
	if err != nil || duplicate.GameResult == nil || duplicate.GameResult.Id != drive.GameResult.Id || duplicate.Points.Balance != drive.Points.Balance {
		t.Fatalf("duplicate game result = %#v, %v", duplicate, err)
	}

	pets, err := env.peer.ListPets(env.ctx, "gameplay.pet.list", rpcapi.ServerPetListRequest{})
	if err != nil {
		t.Fatalf("pet.list: %v", err)
	}
	requirePetID(t, pets.Items, adopted.Pet.Id)

	pointsTransactions, err := env.peer.ListPointsTransactions(env.ctx, "gameplay.points.transactions.list", rpcapi.ServerPointsTransactionListRequest{})
	if err != nil {
		t.Fatalf("points.transactions.list: %v", err)
	}
	requirePointsTransactionID(t, pointsTransactions.Items, adopted.Transaction.Id)

	results, err := env.peer.ListGameResults(env.ctx, "gameplay.game_result.list", rpcapi.ServerGameResultListRequest{})
	if err != nil {
		t.Fatalf("game_result.list: %v", err)
	}
	requireGameResultID(t, results.Items, drive.GameResult.Id)

	grants, err := env.peer.ListRewardGrants(env.ctx, "gameplay.reward_grant.list", rpcapi.ServerRewardGrantListRequest{})
	if err != nil {
		t.Fatalf("reward_grant.list: %v", err)
	}
	requireRewardGrantID(t, grants.Items, drive.RewardGrants[0].Id)
}

func TestGameplayPetWorkspaceAudioHistory(t *testing.T) {
	env := newSetupGameplayHarness(t, "client-gameplay-chat")

	adopted, err := env.peer.AdoptPet(env.ctx, "gameplay.chat.pet.adopt", rpcapi.RuntimeAdoptRequest{
		DisplayName: testStringPtr("Chat Pet"),
	})
	if err != nil {
		t.Fatalf("pet.adopt for chat: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.peer.DeletePet(env.ctx, "gameplay.chat.pet.delete.cleanup", rpcapi.ServerPetDeleteRequest{Id: adopted.Pet.Id})
	})
	assertAdoptedStarterPet(t, adopted.Pet)
	if adopted.Pet.DisplayName != "Chat Pet" {
		t.Fatalf("adopted chat pet = %#v", adopted.Pet)
	}
	workspace, err := env.peer.GetWorkspace(env.ctx, "gameplay.pet.audio.workspace.get", rpcapi.WorkspaceGetRequest{Name: adopted.Pet.WorkspaceName})
	if err != nil {
		t.Fatalf("get pet audio workspace: %v", err)
	}
	if workspace.Value.Parameters == nil {
		t.Fatal("pet audio workspace parameters are missing")
	}
	petParameters, err := workspace.Value.Parameters.AsPetWorkspaceParameters()
	if err != nil {
		t.Fatalf("decode pet audio workspace parameters: %v", err)
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
			packets := synthesizeGameplayOpus(t, env, "volc-bigtts", petParameters.Voice.VoiceId, utterance)
			streamID := "gameplay-pet-audio-" + strconv.Itoa(round+1) + "-" + strconv.Itoa(attempt)
			sendGameplayAudioTurn(t, env.ctx, stream, streamID, packets)
			responseErr = waitForGameplayAssistantResponse(env.ctx, stream, streamID)
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
		if entry.Id == "" || entry.Text == "" || !entry.ReplayAvailable {
			t.Fatalf("pet audio history round %d = %#v, want combined replayable transcript", round+1, entry)
		}
		if round > 0 && entry.Id == entries[round-1].Id {
			t.Fatalf("pet audio history round %d reused entry %q", round+1, entry.Id)
		}
		assertGameplayHistoryReplayAudio(t, env.ctx, env.peer, stream, entry)
		known[entry.Id] = entry
		entries = append(entries, entry)
	}

	first, err := env.peer.GetWorkspaceHistory(env.ctx, "gameplay.pet.history.first.get", rpcapi.WorkspaceHistoryGetRequest{
		WorkspaceName: adopted.Pet.WorkspaceName,
		HistoryId:     entries[0].Id,
	})
	if err != nil {
		t.Fatalf("get first pet audio history after second turn: %v", err)
	}
	if first.Text != entries[0].Text || !first.ReplayAvailable {
		t.Fatalf("first pet audio history changed after second turn: before=%#v after=%#v", entries[0], first)
	}
}

func assertAdoptedStarterPet(t *testing.T, pet rpcapi.Pet) {
	t.Helper()
	if pet.PetdefId != "petdef-starter" || pet.DisplayName == "" || pet.WorkspaceName == "" {
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

func requirePetID(t *testing.T, items []rpcapi.Pet, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("pet %q not found in %#v", id, items)
}

func requirePointsTransactionID(t *testing.T, items []rpcapi.PointsTransaction, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("points transaction %q not found in %#v", id, items)
}

func requireGameResultID(t *testing.T, items []rpcapi.GameResult, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("game result %q not found in %#v", id, items)
}

func requireRewardGrantID(t *testing.T, items []rpcapi.RewardGrant, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("reward grant %q not found in %#v", id, items)
}

func testStringPtr(v string) *string {
	return &v
}
