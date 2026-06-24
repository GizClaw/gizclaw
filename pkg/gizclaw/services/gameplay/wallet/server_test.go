package wallet

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	_ "modernc.org/sqlite"
)

func TestServerWalletTransactionsAreAuditableAndPaged(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	nextID := 0
	srv := &Server{
		DB: newTestDB(t),
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string {
			nextID++
			return string(rune('a' + nextID - 1))
		},
	}

	empty, err := srv.GetWallet(ctx, "gear-a")
	if err != nil {
		t.Fatalf("GetWallet empty error = %v", err)
	}
	if empty.Id != "wallet-gear-a" || empty.TokenBalance != 0 || empty.PointBalance != 0 {
		t.Fatalf("GetWallet empty = %#v", empty)
	}

	wallet, firstTx, err := srv.AddTransaction(ctx, "gear-a", Mutation{
		PointDelta: 10,
		Reason:     rpcapi.WalletTransactionObjectReasonRewardClaim,
	})
	if err != nil {
		t.Fatalf("AddTransaction first error = %v", err)
	}
	if wallet.PointBalance != 10 || wallet.TokenBalance != 0 {
		t.Fatalf("wallet after first = %#v, want points 10 tokens 0", wallet)
	}
	if firstTx.Reason != rpcapi.WalletTransactionObjectReasonRewardClaim || firstTx.PointDelta != 10 {
		t.Fatalf("first transaction = %#v", firstTx)
	}
	if _, _, err := srv.AddTransaction(ctx, "gear-a", Mutation{
		PointDelta: -3,
		Reason:     rpcapi.WalletTransactionObjectReasonPetFeed,
	}); err != nil {
		t.Fatalf("AddTransaction second error = %v", err)
	}
	finalWallet, err := srv.GetWallet(ctx, "gear-a")
	if err != nil {
		t.Fatalf("GetWallet final error = %v", err)
	}
	if finalWallet.PointBalance != 7 {
		t.Fatalf("wallet points final = %d, want 7", finalWallet.PointBalance)
	}

	page, err := srv.ListTransactions(ctx, "gear-a", rpcapi.WalletTransactionsListRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListTransactions page 1 error = %v", err)
	}
	if got := len(page.Items); got != 1 || !page.HasNext || page.NextCursor == nil {
		t.Fatalf("ListTransactions page 1 = len %d has_next %v cursor %v", got, page.HasNext, page.NextCursor)
	}
	if page.Items[0].Reason != rpcapi.WalletTransactionObjectReasonPetFeed {
		t.Fatalf("ListTransactions page 1 first item = %#v, want newest pet_feed", page.Items[0])
	}
	page, err = srv.ListTransactions(ctx, "gear-a", rpcapi.WalletTransactionsListRequest{Cursor: page.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("ListTransactions page 2 error = %v", err)
	}
	if got := len(page.Items); got != 1 || page.HasNext {
		t.Fatalf("ListTransactions page 2 = len %d has_next %v", got, page.HasNext)
	}

	gotTx, err := srv.GetTransaction(ctx, "gear-a", rpcapi.WalletTransactionsGetRequest{Id: page.Items[0].Id})
	if err != nil {
		t.Fatalf("GetTransaction error = %v", err)
	}
	if gotTx.Id != page.Items[0].Id {
		t.Fatalf("GetTransaction = %#v, want %q", gotTx, page.Items[0].Id)
	}
	if _, err := srv.GetTransaction(ctx, "gear-b", rpcapi.WalletTransactionsGetRequest{Id: page.Items[0].Id}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetTransaction foreign error = %v, want %v", err, sql.ErrNoRows)
	}

	foreign, err := srv.ListTransactions(ctx, "gear-b", rpcapi.WalletTransactionsListRequest{})
	if err != nil {
		t.Fatalf("ListTransactions foreign error = %v", err)
	}
	if got := len(foreign.Items); got != 0 {
		t.Fatalf("ListTransactions foreign len = %d, want 0", got)
	}
}

func TestServerWalletValidationAndDefaults(t *testing.T) {
	ctx := context.Background()
	srv := &Server{DB: newTestDB(t)}
	if _, err := (*Server)(nil).GetWallet(ctx, "gear-a"); err == nil {
		t.Fatalf("nil server GetWallet error = nil, want error")
	}
	if _, err := srv.GetWallet(ctx, " "); err == nil {
		t.Fatalf("blank peer GetWallet error = nil, want error")
	}
	if _, _, err := srv.AddTransaction(ctx, "gear-a", Mutation{Reason: rpcapi.WalletTransactionObjectReasonRewardClaim}); err == nil {
		t.Fatalf("zero delta AddTransaction error = nil, want error")
	}
	if _, _, err := srv.AddTransaction(ctx, "gear-a", Mutation{PointDelta: 1}); err == nil {
		t.Fatalf("blank reason AddTransaction error = nil, want error")
	}
	if _, _, err := srv.AddTransaction(ctx, "gear-a", Mutation{
		PointDelta: 1,
		Reason:     rpcapi.WalletTransactionObjectReasonRewardClaim,
	}); err != nil {
		t.Fatalf("default id AddTransaction error = %v", err)
	}
	if _, _, err := srv.AddTransaction(ctx, "gear-a", Mutation{
		PointDelta: -2,
		Reason:     rpcapi.WalletTransactionObjectReasonPetPlay,
	}); err == nil {
		t.Fatalf("negative final balance error = nil, want error")
	}
	finalWallet, err := srv.GetWallet(ctx, "gear-a")
	if err != nil {
		t.Fatalf("GetWallet final error = %v", err)
	}
	if finalWallet.PointBalance != 1 {
		t.Fatalf("wallet points after rejected mutation = %d, want 1", finalWallet.PointBalance)
	}
	txs, err := srv.ListTransactions(ctx, "gear-a", rpcapi.WalletTransactionsListRequest{Limit: -1})
	if err != nil {
		t.Fatalf("ListTransactions default limit error = %v", err)
	}
	if got := len(txs.Items); got != 1 {
		t.Fatalf("ListTransactions len = %d, want 1", got)
	}
}

func TestServerWalletTransactionsSortByCreatedAtNotID(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	srv := &Server{
		DB:    newTestDB(t),
		Now:   func() time.Time { return now },
		NewID: func() string { return "aaa-newer" },
	}
	if _, _, err := srv.AddTransaction(ctx, "gear-a", Mutation{
		PointDelta: 1,
		Reason:     rpcapi.WalletTransactionObjectReasonRewardClaim,
	}); err != nil {
		t.Fatalf("AddTransaction error = %v", err)
	}
	if _, err := srv.DB.ExecContext(ctx, `
INSERT INTO wallet_transactions (peer_id, id, token_delta, point_delta, reason, created_at)
VALUES (?, ?, 0, 1, ?, ?)`,
		"gear-a",
		"zzz-older",
		string(rpcapi.WalletTransactionObjectReasonRewardClaim),
		now.Add(-time.Hour).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert older transaction: %v", err)
	}

	page, err := srv.ListTransactions(ctx, "gear-a", rpcapi.WalletTransactionsListRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListTransactions page 1 error = %v", err)
	}
	if got := page.Items[0].Id; got != "20260611T100000.000000000Z-aaa-newer" {
		t.Fatalf("ListTransactions first id = %q, want newer created_at", got)
	}
	page, err = srv.ListTransactions(ctx, "gear-a", rpcapi.WalletTransactionsListRequest{Cursor: page.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("ListTransactions page 2 error = %v", err)
	}
	if got := page.Items[0].Id; got != "zzz-older" {
		t.Fatalf("ListTransactions second id = %q, want older transaction", got)
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Fatalf("close sqlite: %v", err)
		}
	})
	return db
}
