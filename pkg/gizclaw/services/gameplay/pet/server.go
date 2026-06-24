package pet

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
	defaultAdoptCost = int64(100)
)

var (
	petsRoot = kv.Key{"by-owner"}
)

type SpeciesSelector interface {
	SelectSpecies(ctx context.Context, owner string) (string, error)
}

type VoiceSelector interface {
	SelectVoice(ctx context.Context, owner string) (string, error)
}

type ActionDecider interface {
	DecidePetAction(ctx context.Context, action string, prompt string, pet rpcapi.PetObject) (rpcapi.PetActionDecision, error)
}

type Wallet interface {
	AddTransaction(ctx context.Context, peerID string, mutation wallet.Mutation) (rpcapi.WalletObject, rpcapi.WalletTransactionObject, error)
}

type Server struct {
	Store           kv.Store
	Wallet          Wallet
	SpeciesSelector SpeciesSelector
	VoiceSelector   VoiceSelector
	ActionDecider   ActionDecider
	AdoptPointCost  int64
	Now             func() time.Time
	NewID           func() string
}

func (s *Server) ListPets(ctx context.Context, owner string, request rpcapi.PetListRequest) (rpcapi.PetListResponse, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.PetListResponse{}, err
	}
	cursorValue := ""
	if request.Cursor != nil {
		cursorValue = *request.Cursor
	}
	cursor, limit := normalizeListParams(cursorValue, request.Limit)
	prefix := ownerPrefix(owner)
	entries, err := kv.ListAfter(ctx, s.Store, prefix, cursorAfterKey(prefix, cursor), limit+1)
	if err != nil {
		return rpcapi.PetListResponse{}, err
	}
	page, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]rpcapi.PetObject, 0, len(page))
	for _, entry := range page {
		item, err := decodePet(entry.Value)
		if err != nil {
			return rpcapi.PetListResponse{}, fmt.Errorf("pet: decode list %s: %w", entry.Key.String(), err)
		}
		items = append(items, item)
	}
	return rpcapi.PetListResponse{Items: items, HasNext: hasNext, NextCursor: nextCursor}, nil
}

func (s *Server) GetPet(ctx context.Context, owner string, request rpcapi.PetGetRequest) (rpcapi.PetObject, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.PetObject{}, err
	}
	return s.loadPet(ctx, owner, request.Id)
}

func (s *Server) AdoptPet(ctx context.Context, owner string, request rpcapi.PetAdoptRequest) (rpcapi.PetObject, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.PetObject{}, err
	}
	if s.Wallet == nil {
		return rpcapi.PetObject{}, errors.New("pet adoption requires wallet service")
	}
	if s.SpeciesSelector == nil {
		return rpcapi.PetObject{}, errors.New("pet species selector not configured")
	}
	if s.VoiceSelector == nil {
		return rpcapi.PetObject{}, errors.New("pet voice selector not configured")
	}
	id := optionalString(request.Id)
	if id == "" {
		id = s.newID()
	}
	if _, err := s.loadPet(ctx, owner, id); err == nil {
		return rpcapi.PetObject{}, fmt.Errorf("pet %q already exists", id)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.PetObject{}, err
	}
	speciesID, err := s.SpeciesSelector.SelectSpecies(ctx, owner)
	if err != nil {
		return rpcapi.PetObject{}, err
	}
	speciesID = strings.TrimSpace(speciesID)
	if speciesID == "" {
		return rpcapi.PetObject{}, errors.New("pet species selector returned empty species id")
	}
	voiceID, err := s.VoiceSelector.SelectVoice(ctx, owner)
	if err != nil {
		return rpcapi.PetObject{}, err
	}
	voiceID = strings.TrimSpace(voiceID)
	if voiceID == "" {
		return rpcapi.PetObject{}, errors.New("pet voice selector returned empty voice id")
	}
	cost := s.adoptCost()
	if cost > 0 {
		if _, _, err := s.Wallet.AddTransaction(ctx, owner, wallet.Mutation{
			PointDelta: -cost,
			Reason:     rpcapi.WalletTransactionObjectReasonPetAdopt,
		}); err != nil {
			return rpcapi.PetObject{}, err
		}
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = "Pet"
	}
	now := s.now()
	return s.savePet(ctx, owner, rpcapi.PetObject{
		Id:        id,
		Name:      name,
		SpeciesId: speciesID,
		VoiceId:   voiceID,
		Life:      defaultLifeStats(),
		Ability:   defaultAbilityStats(),
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *Server) PutPet(ctx context.Context, owner string, request rpcapi.PetPutRequest) (rpcapi.PetObject, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.PetObject{}, err
	}
	current, err := s.loadPet(ctx, owner, request.Id)
	if err != nil {
		return rpcapi.PetObject{}, err
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return rpcapi.PetObject{}, errors.New("pet name is required")
	}
	now := s.now()
	current.Name = name
	current.UpdatedAt = now
	return s.savePet(ctx, owner, current)
}

func (s *Server) DeletePet(ctx context.Context, owner string, request rpcapi.PetDeleteRequest) (rpcapi.PetObject, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.PetObject{}, err
	}
	current, err := s.loadPet(ctx, owner, request.Id)
	if err != nil {
		return rpcapi.PetObject{}, err
	}
	if err := s.Store.Delete(ctx, petKey(owner, request.Id)); err != nil {
		return rpcapi.PetObject{}, err
	}
	return current, nil
}

func (s *Server) FeedPet(ctx context.Context, owner string, request rpcapi.PetFeedRequest) (rpcapi.PetObject, error) {
	return s.applyPetAction(ctx, owner, request, "feed", rpcapi.WalletTransactionObjectReasonPetFeed)
}

func (s *Server) WashPet(ctx context.Context, owner string, request rpcapi.PetWashRequest) (rpcapi.PetObject, error) {
	return s.applyPetAction(ctx, owner, request, "wash", rpcapi.WalletTransactionObjectReasonPetWash)
}

func (s *Server) PlayPet(ctx context.Context, owner string, request rpcapi.PetPlayRequest) (rpcapi.PetObject, error) {
	return s.applyPetAction(ctx, owner, request, "play", rpcapi.WalletTransactionObjectReasonPetPlay)
}

func (s *Server) applyPetAction(ctx context.Context, owner string, request rpcapi.PetActionRequest, action string, reason rpcapi.WalletTransactionObjectReason) (rpcapi.PetObject, error) {
	if err := s.validate(owner); err != nil {
		return rpcapi.PetObject{}, err
	}
	if s.ActionDecider == nil {
		return rpcapi.PetObject{}, errors.New("pet action generator not configured")
	}
	current, err := s.loadPet(ctx, owner, request.PetId)
	if err != nil {
		return rpcapi.PetObject{}, err
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return rpcapi.PetObject{}, errors.New("pet action prompt is required")
	}
	decision, err := s.ActionDecider.DecidePetAction(ctx, action, prompt, current)
	if err != nil {
		return rpcapi.PetObject{}, err
	}
	pointDelta := decision.PointDelta
	if pointDelta != 0 {
		if s.Wallet == nil {
			return rpcapi.PetObject{}, errors.New("pet action requires wallet service")
		}
		if _, _, err := s.Wallet.AddTransaction(ctx, owner, wallet.Mutation{
			PointDelta: pointDelta,
			Reason:     reason,
		}); err != nil {
			return rpcapi.PetObject{}, err
		}
	}
	now := s.now()
	current.Life = applyLifeDelta(current.Life, decision.LifeDelta)
	current.Ability = applyAbilityDelta(current.Ability, decision.AbilityDelta)
	current.UpdatedAt = now
	return s.savePet(ctx, owner, current)
}

func (s *Server) validate(owner string) error {
	if s == nil || s.Store == nil {
		return errors.New("pet service not configured")
	}
	if strings.TrimSpace(owner) == "" {
		return errors.New("pet owner is required")
	}
	return nil
}

func (s *Server) loadPet(ctx context.Context, owner, id string) (rpcapi.PetObject, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return rpcapi.PetObject{}, errors.New("pet id is required")
	}
	data, err := s.Store.Get(ctx, petKey(owner, id))
	if err != nil {
		return rpcapi.PetObject{}, err
	}
	return decodePet(data)
}

func (s *Server) savePet(ctx context.Context, owner string, pet rpcapi.PetObject) (rpcapi.PetObject, error) {
	if strings.TrimSpace(pet.Id) == "" {
		return rpcapi.PetObject{}, errors.New("pet id is required")
	}
	data, err := json.Marshal(pet)
	if err != nil {
		return rpcapi.PetObject{}, err
	}
	if err := s.Store.Set(ctx, petKey(owner, pet.Id), data); err != nil {
		return rpcapi.PetObject{}, err
	}
	return pet, nil
}

func (s *Server) adoptCost() int64 {
	if s != nil && s.AdoptPointCost != 0 {
		return s.AdoptPointCost
	}
	return defaultAdoptCost
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

func decodePet(data []byte) (rpcapi.PetObject, error) {
	var pet rpcapi.PetObject
	if err := json.Unmarshal(data, &pet); err != nil {
		return rpcapi.PetObject{}, err
	}
	return pet, nil
}

func ownerPrefix(owner string) kv.Key {
	return append(append(kv.Key{}, petsRoot...), escapeStoreSegment(owner))
}

func petKey(owner, id string) kv.Key {
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

func defaultLifeStats() rpcapi.PetLifeStats {
	return rpcapi.PetLifeStats{
		Satiety:     60,
		Cleanliness: 60,
		Mood:        60,
		Energy:      60,
		Health:      100,
	}
}

func defaultAbilityStats() rpcapi.PetAbilityStats {
	return rpcapi.PetAbilityStats{
		Level:        1,
		Exp:          0,
		Charm:        1,
		Intelligence: 1,
		Stamina:      1,
		Luck:         1,
	}
}

func applyLifeDelta(current rpcapi.PetLifeStats, delta rpcapi.PetLifeStats) rpcapi.PetLifeStats {
	base := current
	if base == (rpcapi.PetLifeStats{}) {
		base = defaultLifeStats()
	}
	base.Satiety = clampStat(base.Satiety+delta.Satiety, 0, 100)
	base.Cleanliness = clampStat(base.Cleanliness+delta.Cleanliness, 0, 100)
	base.Mood = clampStat(base.Mood+delta.Mood, 0, 100)
	base.Energy = clampStat(base.Energy+delta.Energy, 0, 100)
	base.Health = clampStat(base.Health+delta.Health, 0, 100)
	return base
}

func applyAbilityDelta(current rpcapi.PetAbilityStats, delta rpcapi.PetAbilityStats) rpcapi.PetAbilityStats {
	base := current
	if base == (rpcapi.PetAbilityStats{}) {
		base = defaultAbilityStats()
	}
	base.Exp = max64(0, base.Exp+delta.Exp)
	base.Charm = max(0, base.Charm+delta.Charm)
	base.Intelligence = max(0, base.Intelligence+delta.Intelligence)
	base.Stamina = max(0, base.Stamina+delta.Stamina)
	base.Luck = max(0, base.Luck+delta.Luck)
	level := max(1, base.Level)
	exp := base.Exp
	for exp >= int64(level*100) {
		level++
	}
	base.Level = level
	return base
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func clampStat(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
