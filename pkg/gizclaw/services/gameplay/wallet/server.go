package wallet

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	DB    *sql.DB
	Now   func() time.Time
	NewID func() string
}

type Mutation struct {
	TokenDelta int64
	PointDelta int64
	Reason     rpcapi.WalletTransactionObjectReason
}

func (s *Server) GetWallet(ctx context.Context, peerID string) (rpcapi.WalletObject, error) {
	if err := s.validate(peerID); err != nil {
		return rpcapi.WalletObject{}, err
	}
	if err := s.ensureSchema(ctx); err != nil {
		return rpcapi.WalletObject{}, err
	}
	return s.ensureWallet(ctx, nil, peerID)
}

func (s *Server) ListTransactions(ctx context.Context, peerID string, request rpcapi.WalletTransactionsListRequest) (rpcapi.WalletTransactionsListResponse, error) {
	if err := s.validate(peerID); err != nil {
		return rpcapi.WalletTransactionsListResponse{}, err
	}
	if err := s.ensureSchema(ctx); err != nil {
		return rpcapi.WalletTransactionsListResponse{}, err
	}
	cursorValue := ""
	if request.Cursor != nil {
		cursorValue = *request.Cursor
	}
	cursor, limit := normalizeListParams(cursorValue, request.Limit)
	query := `SELECT id, token_delta, point_delta, reason, created_at FROM wallet_transactions WHERE peer_id = ?`
	args := []any{peerID}
	if cursor != "" {
		query += ` AND (
			created_at < (SELECT created_at FROM wallet_transactions WHERE peer_id = ? AND id = ?)
			OR (
				created_at = (SELECT created_at FROM wallet_transactions WHERE peer_id = ? AND id = ?)
				AND id < ?
			)
		)`
		args = append(args, peerID, cursor, peerID, cursor, cursor)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return rpcapi.WalletTransactionsListResponse{}, err
	}
	defer rows.Close()

	items := make([]rpcapi.WalletTransactionObject, 0, limit)
	for rows.Next() {
		tx, err := scanTransaction(rows)
		if err != nil {
			return rpcapi.WalletTransactionsListResponse{}, err
		}
		items = append(items, tx)
	}
	if err := rows.Err(); err != nil {
		return rpcapi.WalletTransactionsListResponse{}, err
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	var next *string
	if hasNext && len(items) > 0 {
		v := items[len(items)-1].Id
		next = &v
	}
	return rpcapi.WalletTransactionsListResponse{Items: items, HasNext: hasNext, NextCursor: next}, nil
}

func (s *Server) GetTransaction(ctx context.Context, peerID string, request rpcapi.WalletTransactionsGetRequest) (rpcapi.WalletTransactionObject, error) {
	if err := s.validate(peerID); err != nil {
		return rpcapi.WalletTransactionObject{}, err
	}
	id := strings.TrimSpace(request.Id)
	if id == "" {
		return rpcapi.WalletTransactionObject{}, errors.New("wallet transaction id is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return rpcapi.WalletTransactionObject{}, err
	}
	row := s.DB.QueryRowContext(ctx, `SELECT id, token_delta, point_delta, reason, created_at FROM wallet_transactions WHERE peer_id = ? AND id = ?`, peerID, id)
	tx, err := scanTransaction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return rpcapi.WalletTransactionObject{}, fmt.Errorf("wallet transaction %q not found: %w", id, sql.ErrNoRows)
	}
	return tx, err
}

func (s *Server) AddTransaction(ctx context.Context, peerID string, mutation Mutation) (rpcapi.WalletObject, rpcapi.WalletTransactionObject, error) {
	if err := s.validate(peerID); err != nil {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, err
	}
	if mutation.TokenDelta == 0 && mutation.PointDelta == 0 {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, errors.New("wallet transaction delta must be non-zero")
	}
	if mutation.Reason == "" {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, errors.New("wallet transaction reason is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = tx.Rollback()
		}
	}()

	wallet, err := s.ensureWallet(ctx, tx, peerID)
	if err != nil {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, err
	}
	token := wallet.TokenBalance + mutation.TokenDelta
	points := wallet.PointBalance + mutation.PointDelta
	if token < 0 || points < 0 {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, errors.New("wallet balance cannot be negative")
	}
	now := s.now()
	nowText := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE wallets SET token_balance = ?, point_balance = ?, updated_at = ? WHERE peer_id = ?`, token, points, nowText, peerID); err != nil {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, err
	}
	txID := s.transactionID(now)
	if _, err := tx.ExecContext(ctx, `INSERT INTO wallet_transactions (peer_id, id, token_delta, point_delta, reason, created_at) VALUES (?, ?, ?, ?, ?, ?)`, peerID, txID, mutation.TokenDelta, mutation.PointDelta, string(mutation.Reason), nowText); err != nil {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, err
	}
	if err := tx.Commit(); err != nil {
		return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, err
	}
	ok = true
	wallet.TokenBalance = token
	wallet.PointBalance = points
	wallet.UpdatedAt = now
	record := rpcapi.WalletTransactionObject{Id: txID, TokenDelta: mutation.TokenDelta, PointDelta: mutation.PointDelta, Reason: mutation.Reason, CreatedAt: now}
	return wallet, record, nil
}

func (s *Server) validate(peerID string) error {
	if s == nil || s.DB == nil {
		return errors.New("wallet service not configured")
	}
	if strings.TrimSpace(peerID) == "" {
		return errors.New("wallet peer id is required")
	}
	return nil
}

func (s *Server) ensureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS wallets (
			peer_id TEXT PRIMARY KEY,
			id TEXT NOT NULL UNIQUE,
			token_balance INTEGER NOT NULL,
			point_balance INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS wallet_transactions (
			peer_id TEXT NOT NULL,
			id TEXT NOT NULL,
			token_delta INTEGER NOT NULL,
			point_delta INTEGER NOT NULL,
			reason TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (peer_id, id),
			FOREIGN KEY (peer_id) REFERENCES wallets(peer_id)
		)`,
		`CREATE INDEX IF NOT EXISTS wallet_transactions_peer_created_desc ON wallet_transactions(peer_id, created_at DESC, id DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Server) ensureWallet(ctx context.Context, q sqlExecer, peerID string) (rpcapi.WalletObject, error) {
	if q == nil {
		q = s.DB
	}
	row := q.QueryRowContext(ctx, `SELECT id, token_balance, point_balance, created_at, updated_at FROM wallets WHERE peer_id = ?`, peerID)
	wallet, err := scanWallet(row)
	if err == nil {
		return wallet, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return rpcapi.WalletObject{}, err
	}
	now := s.now()
	id := s.walletID(peerID)
	zero := int64(0)
	if _, err := q.ExecContext(ctx, `INSERT INTO wallets (peer_id, id, token_balance, point_balance, created_at, updated_at) VALUES (?, ?, 0, 0, ?, ?)`, peerID, id, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return rpcapi.WalletObject{}, err
	}
	return rpcapi.WalletObject{Id: id, TokenBalance: zero, PointBalance: zero, CreatedAt: now, UpdatedAt: now}, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanWallet(row rowScanner) (rpcapi.WalletObject, error) {
	var id, createdText, updatedText string
	var token, points int64
	if err := row.Scan(&id, &token, &points, &createdText, &updatedText); err != nil {
		return rpcapi.WalletObject{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return rpcapi.WalletObject{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedText)
	if err != nil {
		return rpcapi.WalletObject{}, err
	}
	return rpcapi.WalletObject{Id: id, TokenBalance: token, PointBalance: points, CreatedAt: created, UpdatedAt: updated}, nil
}

func scanTransaction(row rowScanner) (rpcapi.WalletTransactionObject, error) {
	var id, reasonText, createdText string
	var tokenDelta, pointDelta int64
	if err := row.Scan(&id, &tokenDelta, &pointDelta, &reasonText, &createdText); err != nil {
		return rpcapi.WalletTransactionObject{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return rpcapi.WalletTransactionObject{}, err
	}
	reason := rpcapi.WalletTransactionObjectReason(reasonText)
	return rpcapi.WalletTransactionObject{Id: id, TokenDelta: tokenDelta, PointDelta: pointDelta, Reason: reason, CreatedAt: created}, nil
}

func normalizeListParams(cursor string, limit int) (string, int) {
	normalizedCursor := strings.TrimSpace(cursor)
	normalizedLimit := defaultListLimit
	if limit > 0 {
		normalizedLimit = limit
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	return normalizedCursor, normalizedLimit
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Server) walletID(peerID string) string {
	return "wallet-" + peerID
}

func (s *Server) transactionID(now time.Time) string {
	return now.UTC().Format("20060102T150405.000000000Z") + "-" + s.newID()
}

func (s *Server) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
