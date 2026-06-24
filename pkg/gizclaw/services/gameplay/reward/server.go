package reward

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/wallet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
	defaultCooldown  = 30 * time.Minute
)

var (
	rewardsRoot = kv.Key{"by-owner"}
)

type Decider interface {
	DecideReward(ctx context.Context, owner string, prompt string) (rpcapi.RewardDecision, error)
}

type BadgeResolver interface {
	CanGrantBadge(ctx context.Context, owner string, badgeID string) error
}

type Wallet interface {
	AddTransaction(ctx context.Context, peerID string, mutation wallet.Mutation) (rpcapi.WalletObject, rpcapi.WalletTransactionObject, error)
}

type Server struct {
	Store         kv.Store
	Wallet        Wallet
	Decider       Decider
	BadgeResolver BadgeResolver
	Cooldown      time.Duration
	Now           func() time.Time
	NewID         func() string
}

func (s *Server) ListRewards(ctx context.Context, owner string, request rpcapi.RewardListRequest) (rpcapi.RewardListResponse, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.RewardListResponse{}, err
	}
	cursorValue := ""
	if request.Cursor != nil {
		cursorValue = *request.Cursor
	}
	cursor, limit := normalizeListParams(cursorValue, request.Limit)
	prefix := ownerPrefix(owner)
	entries, err := kv.ListAfter(ctx, s.Store, prefix, cursorAfterKey(prefix, cursor), limit+1)
	if err != nil {
		return rpcapi.RewardListResponse{}, err
	}
	page, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]rpcapi.RewardObject, 0, len(page))
	for _, entry := range page {
		item, err := decodeReward(entry.Value)
		if err != nil {
			return rpcapi.RewardListResponse{}, fmt.Errorf("reward: decode list %s: %w", entry.Key.String(), err)
		}
		items = append(items, item)
	}
	return rpcapi.RewardListResponse{Items: items, HasNext: hasNext, NextCursor: nextCursor}, nil
}

func (s *Server) GetReward(ctx context.Context, owner string, request rpcapi.RewardGetRequest) (rpcapi.RewardObject, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.RewardObject{}, err
	}
	return s.loadReward(ctx, owner, request.Id)
}

func (s *Server) ClaimReward(ctx context.Context, owner string, request rpcapi.RewardClaimRequest) (rpcapi.RewardObject, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.RewardObject{}, err
	}
	if s.Decider == nil {
		return rpcapi.RewardObject{}, errors.New("reward claim generator not configured")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return rpcapi.RewardObject{}, errors.New("reward claim prompt is required")
	}
	if err := s.checkCooldown(ctx, owner, s.now()); err != nil {
		return rpcapi.RewardObject{}, err
	}
	decision, err := s.Decider.DecideReward(ctx, owner, prompt)
	if err != nil {
		return rpcapi.RewardObject{}, err
	}
	badgeID := strings.TrimSpace(decision.BadgeId)
	points := decision.PointAmount
	if points < 0 {
		return rpcapi.RewardObject{}, errors.New("reward point_amount cannot be negative")
	}
	if badgeID == "" && points == 0 {
		return rpcapi.RewardObject{}, errors.New("reward decision must include a badge or positive points")
	}
	if badgeID != "" {
		if s.BadgeResolver == nil {
			return rpcapi.RewardObject{}, errors.New("reward badge resolver not configured")
		}
		if err := s.BadgeResolver.CanGrantBadge(ctx, owner, badgeID); err != nil {
			return rpcapi.RewardObject{}, err
		}
	}
	if points > 0 {
		if s.Wallet == nil {
			return rpcapi.RewardObject{}, errors.New("reward claim requires wallet service")
		}
		if _, _, err := s.Wallet.AddTransaction(ctx, owner, wallet.Mutation{
			PointDelta: points,
			Reason:     rpcapi.WalletTransactionObjectReasonRewardClaim,
		}); err != nil {
			return rpcapi.RewardObject{}, err
		}
	}
	id := s.newID()
	now := s.now()
	reward := rpcapi.RewardObject{
		Id:          id,
		Prompt:      prompt,
		PointAmount: points,
		CreatedAt:   now,
	}
	if badgeID != "" {
		reward.BadgeId = badgeID
	}
	return s.saveReward(ctx, owner, reward)
}

func (s *Server) validate(owner string) error {
	if s == nil || s.Store == nil {
		return errors.New("reward service not configured")
	}
	if strings.TrimSpace(owner) == "" {
		return errors.New("reward owner is required")
	}
	return nil
}

func (s *Server) checkCooldown(ctx context.Context, owner string, now time.Time) error {
	cooldown := s.cooldown()
	if cooldown <= 0 {
		return nil
	}
	prefix := ownerPrefix(owner)
	var latest time.Time
	var after kv.Key
	for {
		entries, err := kv.ListAfter(ctx, s.Store, prefix, after, maxListLimit)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			item, err := decodeReward(entry.Value)
			if err != nil {
				return err
			}
			if item.CreatedAt.After(latest) {
				latest = item.CreatedAt
			}
		}
		if len(entries) < maxListLimit {
			break
		}
		after = entries[len(entries)-1].Key
	}
	if !latest.IsZero() && now.Sub(latest) < cooldown {
		return fmt.Errorf("reward claim cooldown active until %s", latest.Add(cooldown).Format(time.RFC3339Nano))
	}
	return nil
}

func (s *Server) loadReward(ctx context.Context, owner, id string) (rpcapi.RewardObject, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return rpcapi.RewardObject{}, errors.New("reward id is required")
	}
	data, err := s.Store.Get(ctx, rewardKey(owner, id))
	if err != nil {
		return rpcapi.RewardObject{}, err
	}
	return decodeReward(data)
}

func (s *Server) saveReward(ctx context.Context, owner string, reward rpcapi.RewardObject) (rpcapi.RewardObject, error) {
	if strings.TrimSpace(reward.Id) == "" {
		return rpcapi.RewardObject{}, errors.New("reward id is required")
	}
	data, err := json.Marshal(reward)
	if err != nil {
		return rpcapi.RewardObject{}, err
	}
	if err := s.Store.Set(ctx, rewardKey(owner, reward.Id), data); err != nil {
		return rpcapi.RewardObject{}, err
	}
	return reward, nil
}

func (s *Server) cooldown() time.Duration {
	if s != nil && s.Cooldown != 0 {
		return s.Cooldown
	}
	return defaultCooldown
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
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

func decodeReward(data []byte) (rpcapi.RewardObject, error) {
	var reward rpcapi.RewardObject
	if err := json.Unmarshal(data, &reward); err != nil {
		return rpcapi.RewardObject{}, err
	}
	return reward, nil
}

func ownerPrefix(owner string) kv.Key {
	return append(append(kv.Key{}, rewardsRoot...), escapeStoreSegment(owner))
}

func rewardKey(owner, id string) kv.Key {
	return append(ownerPrefix(owner), escapeStoreSegment(id))
}

func normalizeListParams(cursor string, limit int) (string, int) {
	normalizedCursor := escapeStoreSegment(strings.TrimSpace(cursor))
	normalizedLimit := defaultListLimit
	if limit > 0 {
		normalizedLimit = limit
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	return normalizedCursor, normalizedLimit
}

func cursorAfterKey(prefix kv.Key, cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	return append(append(kv.Key{}, prefix...), cursor)
}

func paginateEntries(entries []kv.Entry, limit int) ([]kv.Entry, bool, *string) {
	hasNext := len(entries) > limit
	if !hasNext {
		return entries, false, nil
	}
	page := entries[:limit]
	if len(page) == 0 || len(page[len(page)-1].Key) == 0 {
		return page, true, nil
	}
	cursor := unescapeStoreSegment(page[len(page)-1].Key[len(page[len(page)-1].Key)-1])
	return page, true, &cursor
}

func escapeStoreSegment(value string) string {
	return url.QueryEscape(value)
}

func unescapeStoreSegment(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}
