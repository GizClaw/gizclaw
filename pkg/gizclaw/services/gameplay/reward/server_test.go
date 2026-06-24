package reward

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/wallet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestServerRewardClaimUpdatesWalletAndAuditTrail(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	nextID := 0
	wallets := &recordingWallet{}
	srv := &Server{
		Store:   kv.NewMemory(nil),
		Wallet:  wallets,
		Decider: fixedDecision(rpcapi.RewardDecision{PointAmount: 25}),
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string {
			nextID++
			return string(rune('a' + nextID - 1))
		},
	}

	claimed, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "won a match"})
	if err != nil {
		t.Fatalf("ClaimReward error = %v", err)
	}
	if claimed.Id != "a" || claimed.Prompt != "won a match" || claimed.PointAmount != 25 {
		t.Fatalf("claimed reward = %#v", claimed)
	}
	if len(wallets.mutations) != 1 || wallets.mutations[0].PointDelta != 25 || wallets.mutations[0].Reason != rpcapi.WalletTransactionObjectReasonRewardClaim {
		t.Fatalf("wallet mutations = %#v", wallets.mutations)
	}
	got, err := srv.GetReward(ctx, "gear-a", rpcapi.RewardGetRequest{Id: "a"})
	if err != nil {
		t.Fatalf("GetReward error = %v", err)
	}
	if got.Id != "a" {
		t.Fatalf("GetReward id = %q, want a", got.Id)
	}
	if _, err := srv.GetReward(ctx, "gear-b", rpcapi.RewardGetRequest{Id: "a"}); err == nil {
		t.Fatalf("GetReward foreign error = nil, want error")
	}
}

func TestServerRewardListIsScopedAndPaged(t *testing.T) {
	ctx := context.Background()
	nextID := 0
	srv := &Server{
		Store:    kv.NewMemory(nil),
		Wallet:   &recordingWallet{},
		Decider:  fixedDecision(rpcapi.RewardDecision{PointAmount: 1}),
		Cooldown: -1,
		NewID: func() string {
			nextID++
			return string(rune('a' + nextID - 1))
		},
	}
	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "first"}); err != nil {
		t.Fatalf("ClaimReward a1 error = %v", err)
	}
	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "second"}); err != nil {
		t.Fatalf("ClaimReward a2 error = %v", err)
	}
	if _, err := srv.ClaimReward(ctx, "gear-b", rpcapi.RewardClaimRequest{Prompt: "foreign"}); err != nil {
		t.Fatalf("ClaimReward b error = %v", err)
	}
	page, err := srv.ListRewards(ctx, "gear-a", rpcapi.RewardListRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListRewards page 1 error = %v", err)
	}
	if got := len(page.Items); got != 1 || !page.HasNext || page.NextCursor == nil {
		t.Fatalf("ListRewards page 1 = len %d has_next %v cursor %v", got, page.HasNext, page.NextCursor)
	}
	page, err = srv.ListRewards(ctx, "gear-a", rpcapi.RewardListRequest{Cursor: page.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("ListRewards page 2 error = %v", err)
	}
	if got := len(page.Items); got != 1 || page.HasNext {
		t.Fatalf("ListRewards page 2 = len %d has_next %v", got, page.HasNext)
	}
	foreign, err := srv.ListRewards(ctx, "gear-b", rpcapi.RewardListRequest{})
	if err != nil {
		t.Fatalf("ListRewards foreign error = %v", err)
	}
	if got := len(foreign.Items); got != 1 {
		t.Fatalf("ListRewards foreign len = %d, want 1", got)
	}
}

func TestServerRewardCooldownAndInvalidDecisionDoNotMutate(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	wallets := &recordingWallet{}
	srv := &Server{
		Store:    kv.NewMemory(nil),
		Wallet:   wallets,
		Decider:  fixedDecision(rpcapi.RewardDecision{PointAmount: 1}),
		Cooldown: time.Minute,
		Now: func() time.Time {
			return now
		},
		NewID: func() string { return now.Format("150405") },
	}
	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "first"}); err != nil {
		t.Fatalf("ClaimReward first error = %v", err)
	}
	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "second"}); err == nil {
		t.Fatalf("ClaimReward cooldown error = nil, want error")
	}
	if len(wallets.mutations) != 1 {
		t.Fatalf("wallet mutations after cooldown = %d, want 1", len(wallets.mutations))
	}
	now = now.Add(time.Minute)
	srv.Decider = fixedDecision(rpcapi.RewardDecision{})
	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "invalid"}); err == nil {
		t.Fatalf("ClaimReward invalid decision error = nil, want error")
	}
	if len(wallets.mutations) != 1 {
		t.Fatalf("wallet mutations after invalid decision = %d, want 1", len(wallets.mutations))
	}
	list, err := srv.ListRewards(ctx, "gear-a", rpcapi.RewardListRequest{})
	if err != nil {
		t.Fatalf("ListRewards error = %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("reward history len after invalid decision = %d, want 1", len(list.Items))
	}
}

func TestServerRewardDeciderErrorDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	wallets := &recordingWallet{}
	srv := &Server{
		Store:   kv.NewMemory(nil),
		Wallet:  wallets,
		Decider: errorDecision{err: errors.New("generator rejected output")},
	}
	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "claim"}); err == nil {
		t.Fatalf("ClaimReward decider error = nil, want error")
	}
	if len(wallets.mutations) != 0 {
		t.Fatalf("wallet mutations after decider error = %d, want 0", len(wallets.mutations))
	}
	list, err := srv.ListRewards(ctx, "gear-a", rpcapi.RewardListRequest{})
	if err != nil {
		t.Fatalf("ListRewards after decider error = %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("reward history len after decider error = %d, want 0", len(list.Items))
	}
}

func TestServerRewardCooldownScansBeyondFirstPage(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	wallets := &recordingWallet{}
	srv := &Server{
		Store:    kv.NewMemory(nil),
		Wallet:   wallets,
		Decider:  fixedDecision(rpcapi.RewardDecision{PointAmount: 1}),
		Cooldown: time.Hour,
		Now:      func() time.Time { return now },
		NewID:    func() string { return "new" },
	}
	for i := range maxListLimit + 1 {
		_, err := srv.saveReward(ctx, "gear-a", rpcapi.RewardObject{
			Id:        fmt.Sprintf("old-%03d", i),
			Prompt:    "old",
			CreatedAt: now.Add(-2 * time.Hour),
		})
		if err != nil {
			t.Fatalf("save old reward %d: %v", i, err)
		}
	}
	_, err := srv.saveReward(ctx, "gear-a", rpcapi.RewardObject{
		Id:        "zzz-recent",
		Prompt:    "recent",
		CreatedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("save recent reward: %v", err)
	}

	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "claim"}); err == nil {
		t.Fatalf("ClaimReward cooldown error = nil, want error")
	}
	if len(wallets.mutations) != 0 {
		t.Fatalf("wallet mutations after cooldown = %d, want 0", len(wallets.mutations))
	}
}

func TestServerRewardValidationAndBadgeResolver(t *testing.T) {
	ctx := context.Background()
	srv := &Server{
		Store:   kv.NewMemory(nil),
		Wallet:  &recordingWallet{},
		Decider: fixedDecision(rpcapi.RewardDecision{BadgeId: "badge-a"}),
	}
	if _, err := (*Server)(nil).ListRewards(ctx, "gear-a", rpcapi.RewardListRequest{}); err == nil {
		t.Fatalf("nil server ListRewards error = nil, want error")
	}
	if _, err := srv.GetReward(ctx, " ", rpcapi.RewardGetRequest{Id: "r1"}); err == nil {
		t.Fatalf("blank owner GetReward error = nil, want error")
	}
	if _, err := srv.GetReward(ctx, "gear-a", rpcapi.RewardGetRequest{}); err == nil {
		t.Fatalf("blank id GetReward error = nil, want error")
	}
	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{}); err == nil {
		t.Fatalf("blank prompt ClaimReward error = nil, want error")
	}
	if _, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "badge"}); err == nil {
		t.Fatalf("ClaimReward missing badge resolver error = nil, want error")
	}
	srv.BadgeResolver = allowBadge{}
	claimed, err := srv.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "badge"})
	if err != nil {
		t.Fatalf("ClaimReward badge error = %v", err)
	}
	if claimed.BadgeId != "badge-a" || claimed.PointAmount != 0 {
		t.Fatalf("claimed badge reward = %#v", claimed)
	}
	noWallet := &Server{Store: kv.NewMemory(nil), Decider: fixedDecision(rpcapi.RewardDecision{PointAmount: 1})}
	if _, err := noWallet.ClaimReward(ctx, "gear-a", rpcapi.RewardClaimRequest{Prompt: "points"}); err == nil {
		t.Fatalf("ClaimReward no wallet error = nil, want error")
	}
}

type fixedDecision rpcapi.RewardDecision

func (d fixedDecision) DecideReward(context.Context, string, string) (rpcapi.RewardDecision, error) {
	return rpcapi.RewardDecision(d), nil
}

type errorDecision struct {
	err error
}

func (d errorDecision) DecideReward(context.Context, string, string) (rpcapi.RewardDecision, error) {
	return rpcapi.RewardDecision{}, d.err
}

type allowBadge struct{}

func (allowBadge) CanGrantBadge(context.Context, string, string) error {
	return nil
}

type recordingWallet struct {
	mutations []wallet.Mutation
}

func (w *recordingWallet) AddTransaction(_ context.Context, _ string, mutation wallet.Mutation) (rpcapi.WalletObject, rpcapi.WalletTransactionObject, error) {
	w.mutations = append(w.mutations, mutation)
	return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, nil
}
