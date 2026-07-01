//go:build gizclaw_e2e

package rpc_test

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerRewardRPC(t *testing.T) {
	env := newBusinessHarness(t)

	reward, err := env.a.ClaimReward(env.ctx, "reward.claim", rpcapi.RewardClaimRequest{Prompt: "finished the tutorial; grant a small positive point reward"})
	if err != nil {
		t.Fatalf("reward.claim: %v", err)
	}
	if reward.Id == "" || reward.Prompt == "" || (reward.BadgeId == "" && reward.PointAmount <= 0) {
		t.Fatalf("reward = %#v", reward)
	}
	gotReward, err := env.a.GetReward(env.ctx, "reward.get", rpcapi.RewardGetRequest{Id: reward.Id})
	if err != nil {
		t.Fatalf("reward.get: %v", err)
	}
	if gotReward.Id != reward.Id {
		t.Fatalf("reward.get id = %q, want %q", gotReward.Id, reward.Id)
	}
	time.Sleep(2 * time.Millisecond)
	secondReward, err := env.a.ClaimReward(env.ctx, "reward.claim.second", rpcapi.RewardClaimRequest{Prompt: "helped a friend; grant a small positive point reward"})
	if err != nil {
		t.Fatalf("reward.claim second: %v", err)
	}
	if secondReward.Id == "" || secondReward.Prompt == "" || (secondReward.BadgeId == "" && secondReward.PointAmount <= 0) {
		t.Fatalf("second reward = %#v", secondReward)
	}
	assertRewardPagination(t, env.ctx, env.a, []string{reward.Id, secondReward.Id})
	if _, err := env.b.GetReward(env.ctx, "reward.get.denied", rpcapi.RewardGetRequest{Id: reward.Id}); err == nil {
		t.Fatalf("reward.get from peer-b should fail")
	}
}
