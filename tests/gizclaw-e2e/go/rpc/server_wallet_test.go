//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerWalletRPC(t *testing.T) {
	env := newBusinessHarness(t)

	walletBefore, err := env.a.GetWallet(env.ctx, "wallet.get.before", rpcapi.WalletGetRequest{})
	if err != nil {
		t.Fatalf("wallet.get before: %v", err)
	}
	if walletBefore.PointBalance != 0 {
		t.Fatalf("wallet before point balance = %d, want 0", walletBefore.PointBalance)
	}
	reward, err := env.a.ClaimReward(env.ctx, "reward.claim.wallet", rpcapi.RewardClaimRequest{Prompt: "finished the wallet tutorial; grant a small positive point reward"})
	if err != nil {
		t.Fatalf("reward.claim wallet: %v", err)
	}
	if reward.PointAmount <= 0 {
		t.Fatalf("reward point amount = %d, want positive", reward.PointAmount)
	}
	walletAfter, err := env.a.GetWallet(env.ctx, "wallet.get.after", rpcapi.WalletGetRequest{})
	if err != nil {
		t.Fatalf("wallet.get after: %v", err)
	}
	if walletAfter.PointBalance != reward.PointAmount {
		t.Fatalf("wallet after point balance = %d, want %d", walletAfter.PointBalance, reward.PointAmount)
	}
	transactionID := assertWalletTransaction(t, env.ctx, env.a)
	if _, err := env.b.GetWalletTransaction(env.ctx, "wallet.transactions.get.denied", rpcapi.WalletTransactionsGetRequest{Id: transactionID}); err == nil {
		t.Fatalf("wallet.transactions.get from peer-b should fail")
	}
}
