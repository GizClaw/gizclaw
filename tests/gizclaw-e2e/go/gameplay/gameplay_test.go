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

	ruleset, err := env.peer.GetGameRuleset(env.ctx, "gameplay.game_ruleset.get", rpcapi.ServerGameRulesetGetRequest{Name: testStringPtr("default-gameplay")})
	if err != nil {
		t.Fatalf("game_ruleset.get default-gameplay: %v", err)
	}
	if ruleset.Name != "default-gameplay" || !ruleset.Spec.Enabled || ruleset.Spec.DefaultWorkflowName == nil || *ruleset.Spec.DefaultWorkflowName != "pet-care" {
		t.Fatalf("game_ruleset.get = %#v", ruleset)
	}

	adopted, err := env.peer.AdoptPet(env.ctx, "gameplay.pet.adopt", rpcapi.ServerPetAdoptRequest{
		RulesetName: testStringPtr("default-gameplay"),
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
	if workspace.Name != adopted.Pet.WorkspaceName || workspace.WorkflowName != "pet-care" {
		t.Fatalf("pet workspace = %#v", workspace)
	}
	if workspace.Parameters == nil {
		t.Fatalf("pet workspace parameters = nil")
	}
	petParameters, err := workspace.Parameters.AsPetWorkspaceParameters()
	if err != nil {
		t.Fatalf("pet workspace parameters: %v", err)
	}
	if petParameters.AgentType != rpcapi.PetWorkspaceParametersAgentTypePet || petParameters.Voice.VoiceId != "volc-tenant:volc-main:zh_female_shaoergushi_mars_bigtts" {
		t.Fatalf("pet workspace parameters = %#v", petParameters)
	}

	score := int64(42)
	maxScore := int64(100)
	durationMs := int64(2345)
	difficulty := "normal"
	idempotencyKey := "gameplay-result-1"
	drive, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive", rpcapi.ServerPetDriveRequest{
		PetId:  adopted.Pet.Id,
		Action: testStringPtr("bath"),
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
	if drive.Pet.Progression["xp"] != 105 {
		t.Fatalf("pet.drive pet = %#v reward_grants = %#v", drive.Pet, drive.RewardGrants)
	}
	if drive.Points.Balance != 105 {
		t.Fatalf("pet.drive points = %#v", drive.Points)
	}
	if drive.GameResult == nil || drive.GameResult.GameDefId != "game-starter" || drive.GameResult.Score == nil || *drive.GameResult.Score != score {
		t.Fatalf("pet.drive game result = %#v", drive.GameResult)
	}
	if drive.GameResult.MaxScore == nil || *drive.GameResult.MaxScore != maxScore || drive.GameResult.DurationMs == nil || *drive.GameResult.DurationMs != durationMs || drive.GameResult.IdempotencyKey == nil || *drive.GameResult.IdempotencyKey != idempotencyKey {
		t.Fatalf("pet.drive game result details = %#v", drive.GameResult)
	}
	if len(drive.Badges) != 1 || drive.Badges[0].BadgeDefId != "badge-starter" || !drive.Badges[0].Active || drive.Badges[0].Level != 1 {
		t.Fatalf("pet.drive badges = %#v", drive.Badges)
	}
	if len(drive.RewardGrants) != 1 || drive.RewardGrants[0].PointsDelta != 20 || drive.RewardGrants[0].PetExpDelta != 105 {
		t.Fatalf("pet.drive reward grants = %#v", drive.RewardGrants)
	}
	if len(drive.Transactions) != 2 || drive.Transactions[0].Delta != -5 || drive.Transactions[1].Delta != 20 {
		t.Fatalf("pet.drive transactions = %#v", drive.Transactions)
	}
	if _, err := env.peer.DrivePet(env.ctx, "gameplay.pet.drive.duplicate", rpcapi.ServerPetDriveRequest{
		PetId: adopted.Pet.Id,
		GameResult: &rpcapi.PetDriveGameResultInput{
			GameDefId:      "game-starter",
			IdempotencyKey: &idempotencyKey,
		},
	}); err == nil {
		t.Fatal("duplicate game result idempotency key should fail")
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

	adopted, err := env.peer.AdoptPet(env.ctx, "gameplay.chat.pet.adopt", rpcapi.ServerPetAdoptRequest{
		RulesetName: testStringPtr("default-gameplay"),
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
	if workspace.Parameters == nil {
		t.Fatal("pet audio workspace parameters are missing")
	}
	petParameters, err := workspace.Parameters.AsPetWorkspaceParameters()
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
	if pet.WorkflowName == nil || *pet.WorkflowName != "pet-care" {
		t.Fatalf("adopted pet workflow = %#v", pet.WorkflowName)
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
