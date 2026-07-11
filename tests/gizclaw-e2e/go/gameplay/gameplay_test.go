//go:build gizclaw_e2e

package gameplay_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestGameplayAdoptDriveAndPetWorkspace(t *testing.T) {
	env := newIsolatedGameplayHarness(t)

	ruleset, err := env.peer.GetGameRuleset(env.ctx, "gameplay.game_ruleset.get", rpcapi.ServerGameRulesetGetRequest{Name: testStringPtr("default-gameplay")})
	if err != nil {
		t.Fatalf("game_ruleset.get default-gameplay: %v", err)
	}
	if ruleset.Name != "default-gameplay" || !ruleset.Spec.Enabled || ruleset.Spec.DefaultWorkflowName == nil || *ruleset.Spec.DefaultWorkflowName != "pet-care-flowcraft" {
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
	if workspace.Name != adopted.Pet.WorkspaceName || workspace.WorkflowName != "pet-care-flowcraft" {
		t.Fatalf("pet workspace = %#v", workspace)
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

func TestGameplayPetWorkspaceChat(t *testing.T) {
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

	if err := selectGameplayWorkspace(env.ctx, env.peer, adopted.Pet.WorkspaceName); err != nil {
		t.Fatalf("select pet workspace %q: %v", adopted.Pet.WorkspaceName, err)
	}
	out, err := env.peer.Transform(env.ctx, "gameplay.pet.chat", chatTextStream("你好，我今天来看看你。"))
	if err != nil {
		t.Fatalf("pet workspace chat transform: %v", err)
	}
	defer out.Close()
	reply := readFirstModelText(t, env.ctx, out)
	if strings.TrimSpace(reply) == "" {
		t.Fatalf("pet workspace chat reply is empty")
	}
	waitForWorkspaceHistoryText(t, env.ctx, env.peer, adopted.Pet.WorkspaceName, "你好，我今天来看看你。")
}

func assertAdoptedStarterPet(t *testing.T, pet rpcapi.Pet) {
	t.Helper()
	if pet.PetdefId != "petdef-starter" || pet.DisplayName == "" || pet.WorkspaceName == "" {
		t.Fatalf("adopted pet = %#v", pet)
	}
	if pet.WorkflowName == nil || *pet.WorkflowName != "pet-care-flowcraft" {
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

func readFirstModelText(t *testing.T, ctx context.Context, stream genx.Stream) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var got strings.Builder
	for {
		chunk, err := nextChunk(ctx, stream)
		if err != nil {
			if err == genx.ErrDone || err == io.EOF {
				return got.String()
			}
			t.Fatalf("read pet chat stream: %v", err)
		}
		if chunk == nil {
			continue
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Error) != "" {
			t.Fatalf("pet chat stream returned error: %s", chunk.Ctrl.Error)
		}
		if text, ok := chunk.Part.(genx.Text); ok && chunk.Role == genx.RoleModel {
			got.WriteString(string(text))
			if strings.TrimSpace(got.String()) != "" {
				return got.String()
			}
		}
	}
}

func nextChunk(ctx context.Context, stream genx.Stream) (*genx.MessageChunk, error) {
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
		_ = stream.CloseWithError(ctx.Err())
		return nil, ctx.Err()
	}
}

func waitForWorkspaceHistoryText(t *testing.T, ctx context.Context, client interface {
	ListWorkspaceHistory(context.Context, string, rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error)
}, workspaceName, text string) rpcapi.PeerRunHistoryEntry {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for {
		list, err := client.ListWorkspaceHistory(ctx, "gameplay.workspace.history.list", rpcapi.WorkspaceHistoryListRequest{WorkspaceName: workspaceName})
		if err == nil {
			for _, item := range list.Items {
				if item.Text == text {
					return item
				}
			}
			lastErr = nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			t.Fatalf("history text %q not found in workspace %q, last error: %v", text, workspaceName, lastErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func chatTextStream(text string) genx.Stream {
	return &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "gameplay-chat-text", Label: "transcript"}},
		{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "gameplay-chat-text", Label: "transcript", EndOfStream: true}},
	}}
}

type sliceStream struct {
	chunks []*genx.MessageChunk
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, genx.ErrDone
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *sliceStream) Close() error {
	s.chunks = nil
	return nil
}

func (s *sliceStream) CloseWithError(error) error {
	s.chunks = nil
	return nil
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
