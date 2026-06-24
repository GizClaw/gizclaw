package pet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/wallet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestServerPetAdoptPagingScopeAndActions(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	nextID := 0
	testWallet := &recordingWallet{}
	srv := &Server{
		Store:           kv.NewMemory(nil),
		Wallet:          testWallet,
		SpeciesSelector: fixedSpecies("rabbit"),
		VoiceSelector:   fixedVoice("voice-a"),
		ActionDecider: actionDeciderFunc(func(_ context.Context, action string, prompt string, _ rpcapi.PetObject) (rpcapi.PetActionDecision, error) {
			if action != "play" {
				t.Fatalf("action = %q, want play", action)
			}
			if prompt != "throw ball" {
				t.Fatalf("prompt = %q, want throw ball", prompt)
			}
			return rpcapi.PetActionDecision{
				PointDelta: -3,
				LifeDelta: rpcapi.PetLifeStats{
					Mood: 7,
				},
				AbilityDelta: rpcapi.PetAbilityStats{
					Exp: 125,
				},
			}, nil
		}),
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string {
			nextID++
			return string(rune('a' + nextID - 1))
		},
	}

	first, err := srv.AdoptPet(ctx, "gear-a", rpcapi.PetAdoptRequest{Name: "one"})
	if err != nil {
		t.Fatalf("AdoptPet first error = %v", err)
	}
	second, err := srv.AdoptPet(ctx, "gear-a", rpcapi.PetAdoptRequest{Name: "two"})
	if err != nil {
		t.Fatalf("AdoptPet second error = %v", err)
	}
	if _, err := srv.AdoptPet(ctx, "gear-b", rpcapi.PetAdoptRequest{Id: stringPtr("foreign"), Name: "other"}); err != nil {
		t.Fatalf("AdoptPet foreign error = %v", err)
	}
	if first.SpeciesId != "rabbit" || first.VoiceId != "voice-a" {
		t.Fatalf("adopted first = %#v, want species rabbit voice voice-a", first)
	}
	if first.Life.Health != 100 || first.Ability.Level != 1 {
		t.Fatalf("adopted first stats = %#v", first)
	}
	if len(testWallet.mutations) != 3 || testWallet.mutations[0].Reason != rpcapi.WalletTransactionObjectReasonPetAdopt {
		t.Fatalf("wallet mutations after adoption = %#v", testWallet.mutations)
	}

	page, err := srv.ListPets(ctx, "gear-a", rpcapi.PetListRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListPets page 1 error = %v", err)
	}
	if got := len(page.Items); got != 1 || !page.HasNext || page.NextCursor == nil {
		t.Fatalf("ListPets page 1 = len %d has_next %v cursor %v", got, page.HasNext, page.NextCursor)
	}
	if page.Items[0].Id != first.Id {
		t.Fatalf("ListPets page 1 first id = %#v, want %q", page.Items[0].Id, first.Id)
	}

	page, err = srv.ListPets(ctx, "gear-a", rpcapi.PetListRequest{Cursor: page.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("ListPets page 2 error = %v", err)
	}
	if got := len(page.Items); got != 1 || page.HasNext {
		t.Fatalf("ListPets page 2 = len %d has_next %v", got, page.HasNext)
	}
	if page.Items[0].Id != second.Id {
		t.Fatalf("ListPets page 2 id = %#v, want %q", page.Items[0].Id, second.Id)
	}

	foreign, err := srv.ListPets(ctx, "gear-b", rpcapi.PetListRequest{})
	if err != nil {
		t.Fatalf("ListPets foreign error = %v", err)
	}
	if got := len(foreign.Items); got != 1 {
		t.Fatalf("ListPets foreign len = %d, want 1", got)
	}

	played, err := srv.PlayPet(ctx, "gear-a", rpcapi.PetPlayRequest{PetId: first.Id, Prompt: "throw ball"})
	if err != nil {
		t.Fatalf("PlayPet error = %v", err)
	}
	if played.Life.Mood != 67 {
		t.Fatalf("PlayPet life = %#v, want mood 67", played.Life)
	}
	if played.Ability.Level != 2 {
		t.Fatalf("PlayPet ability = %#v, want level 2", played.Ability)
	}
	if got := testWallet.mutations[len(testWallet.mutations)-1]; got.PointDelta != -3 || got.Reason != rpcapi.WalletTransactionObjectReasonPetPlay {
		t.Fatalf("PlayPet wallet mutation = %#v, want -3 pet_play", got)
	}

	deleted, err := srv.DeletePet(ctx, "gear-a", rpcapi.PetDeleteRequest{Id: second.Id})
	if err != nil {
		t.Fatalf("DeletePet error = %v", err)
	}
	if deleted.Id != second.Id {
		t.Fatalf("DeletePet id = %#v, want %q", deleted.Id, second.Id)
	}
	if _, err := srv.GetPet(ctx, "gear-a", rpcapi.PetGetRequest{Id: second.Id}); err == nil {
		t.Fatalf("GetPet deleted error = nil, want not found")
	}
}

func TestServerPetPutWashFeedAndValidation(t *testing.T) {
	ctx := context.Background()
	srv := &Server{
		Store:           kv.NewMemory(nil),
		Wallet:          &recordingWallet{},
		SpeciesSelector: fixedSpecies("rabbit"),
		VoiceSelector:   fixedVoice("voice-a"),
		ActionDecider: actionDeciderFunc(func(_ context.Context, _ string, _ string, _ rpcapi.PetObject) (rpcapi.PetActionDecision, error) {
			return rpcapi.PetActionDecision{LifeDelta: rpcapi.PetLifeStats{Cleanliness: 20}}, nil
		}),
		AdoptPointCost: 1,
	}
	created, err := srv.AdoptPet(ctx, "gear-a", rpcapi.PetAdoptRequest{Id: stringPtr("p:1"), Name: "old"})
	if err != nil {
		t.Fatalf("AdoptPet error = %v", err)
	}
	if created.Id != "p:1" {
		t.Fatalf("created id = %#v, want p:1", created.Id)
	}
	updated, err := srv.PutPet(ctx, "gear-a", rpcapi.PetPutRequest{Id: "p:1", Name: "new"})
	if err != nil {
		t.Fatalf("PutPet error = %v", err)
	}
	if updated.Name != "new" || updated.SpeciesId != "rabbit" {
		t.Fatalf("updated pet = %#v", updated)
	}
	washed, err := srv.WashPet(ctx, "gear-a", rpcapi.PetWashRequest{PetId: "p:1", Prompt: "wash carefully"})
	if err != nil {
		t.Fatalf("WashPet error = %v", err)
	}
	if washed.Life.Cleanliness != 80 {
		t.Fatalf("washed life = %#v, want cleanliness 80", washed.Life)
	}
	if _, err := srv.FeedPet(ctx, "gear-a", rpcapi.PetFeedRequest{PetId: "p:1", Prompt: "feed lunch"}); err != nil {
		t.Fatalf("FeedPet error = %v", err)
	}
	if _, err := srv.PlayPet(ctx, "gear-a", rpcapi.PetPlayRequest{PetId: "p:1"}); err == nil {
		t.Fatalf("PlayPet blank prompt error = nil, want error")
	}
	if _, err := srv.AdoptPet(ctx, "gear-a", rpcapi.PetAdoptRequest{Id: stringPtr("p:1")}); err == nil {
		t.Fatalf("AdoptPet duplicate error = nil, want error")
	}
	if _, err := (*Server)(nil).ListPets(ctx, "gear-a", rpcapi.PetListRequest{}); err == nil {
		t.Fatalf("nil server ListPets error = nil, want error")
	}
	if _, err := srv.GetPet(ctx, " ", rpcapi.PetGetRequest{Id: "p:1"}); err == nil {
		t.Fatalf("blank owner GetPet error = nil, want error")
	}
	if _, err := srv.GetPet(ctx, "gear-a", rpcapi.PetGetRequest{}); err == nil {
		t.Fatalf("blank id GetPet error = nil, want error")
	}
	noDecider := *srv
	noDecider.ActionDecider = nil
	if _, err := noDecider.PlayPet(ctx, "gear-a", rpcapi.PetPlayRequest{PetId: "p:1", Prompt: "play"}); err == nil {
		t.Fatalf("PlayPet without decider error = nil, want error")
	}
}

func TestServerPetActionDeciderErrorDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	testWallet := &recordingWallet{}
	srv := &Server{
		Store:           kv.NewMemory(nil),
		Wallet:          testWallet,
		SpeciesSelector: fixedSpecies("rabbit"),
		VoiceSelector:   fixedVoice("voice-a"),
		ActionDecider: actionDeciderFunc(func(context.Context, string, string, rpcapi.PetObject) (rpcapi.PetActionDecision, error) {
			return rpcapi.PetActionDecision{}, errors.New("generator rejected output")
		}),
		AdoptPointCost: 1,
	}
	created, err := srv.AdoptPet(ctx, "gear-a", rpcapi.PetAdoptRequest{Id: stringPtr("pet-a"), Name: "pet"})
	if err != nil {
		t.Fatalf("AdoptPet error = %v", err)
	}
	if len(testWallet.mutations) != 1 {
		t.Fatalf("wallet mutations after adopt = %d, want 1", len(testWallet.mutations))
	}

	if _, err := srv.FeedPet(ctx, "gear-a", rpcapi.PetFeedRequest{PetId: "pet-a", Prompt: "feed"}); err == nil {
		t.Fatalf("FeedPet decider error = nil, want error")
	}
	if len(testWallet.mutations) != 1 {
		t.Fatalf("wallet mutations after failed feed = %d, want 1", len(testWallet.mutations))
	}
	got, err := srv.GetPet(ctx, "gear-a", rpcapi.PetGetRequest{Id: "pet-a"})
	if err != nil {
		t.Fatalf("GetPet after failed feed error = %v", err)
	}
	if got.Life != created.Life || got.Ability != created.Ability || got.UpdatedAt != created.UpdatedAt {
		t.Fatalf("pet changed after failed feed: got %#v want %#v", got, created)
	}
}

func TestPetBoundaryHelpers(t *testing.T) {
	cursor, limit := normalizeListParams("  cursor  ", maxListLimit+1)
	if cursor != "cursor" || limit != maxListLimit {
		t.Fatalf("normalizeListParams() = %q/%d, want cursor/%d", cursor, limit, maxListLimit)
	}
	if got := unescapeStoreSegment("%zz"); got != "%zz" {
		t.Fatalf("unescapeStoreSegment(invalid) = %q, want original", got)
	}
	if _, err := decodePet([]byte("{")); err == nil {
		t.Fatalf("decodePet(invalid json) error = nil, want error")
	}

	srv := &Server{}
	id := srv.newID()
	if id == "" {
		t.Fatalf("default newID() returned empty id")
	}

	life := applyLifeDelta(rpcapi.PetLifeStats{
		Satiety:     1,
		Cleanliness: 99,
		Mood:        50,
		Energy:      50,
		Health:      50,
	}, rpcapi.PetLifeStats{
		Satiety:     -10,
		Cleanliness: 10,
	})
	if life.Satiety != 0 || life.Cleanliness != 100 {
		t.Fatalf("applyLifeDelta clamp = %#v, want satiety 0 cleanliness 100", life)
	}

	ability := applyAbilityDelta(rpcapi.PetAbilityStats{
		Level:        1,
		Exp:          0,
		Charm:        1,
		Intelligence: 1,
		Stamina:      1,
		Luck:         1,
	}, rpcapi.PetAbilityStats{
		Exp:          -10,
		Charm:        -5,
		Intelligence: 2,
		Stamina:      3,
		Luck:         4,
	})
	if ability.Exp != 0 || ability.Charm != 0 || ability.Level != 1 {
		t.Fatalf("applyAbilityDelta floor = %#v, want exp 0 charm 0 level 1", ability)
	}
	if max(1, 2) != 2 || max64(1, 2) != 2 {
		t.Fatalf("max helpers returned wrong result")
	}
}

type fixedSpecies string

func (s fixedSpecies) SelectSpecies(context.Context, string) (string, error) {
	return string(s), nil
}

type fixedVoice string

func (v fixedVoice) SelectVoice(context.Context, string) (string, error) {
	return string(v), nil
}

type actionDeciderFunc func(context.Context, string, string, rpcapi.PetObject) (rpcapi.PetActionDecision, error)

func (f actionDeciderFunc) DecidePetAction(ctx context.Context, action string, prompt string, pet rpcapi.PetObject) (rpcapi.PetActionDecision, error) {
	return f(ctx, action, prompt, pet)
}

type recordingWallet struct {
	mutations []wallet.Mutation
}

func (w *recordingWallet) AddTransaction(_ context.Context, _ string, mutation wallet.Mutation) (rpcapi.WalletObject, rpcapi.WalletTransactionObject, error) {
	w.mutations = append(w.mutations, mutation)
	return rpcapi.WalletObject{}, rpcapi.WalletTransactionObject{}, nil
}

func stringPtr(value string) *string {
	return &value
}
