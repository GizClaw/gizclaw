package gameplay

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestResolveProfileRulesUsesLocalAliasesAndSkipsMissingResources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog := testCatalog(t, time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC))
	profile := seedGameplayCatalog(t, ctx, catalog)

	petDefs := map[string]apitypes.RuntimeProfileBinding{
		"tragon":  gameplayTestBinding("petdef-basic"),
		"missing": gameplayTestBinding("petdef-missing"),
	}
	voices := map[string]apitypes.RuntimeProfileBinding{
		"pet-voice": gameplayTestBinding("voice-basic"),
	}
	gameDefs := map[string]apitypes.RuntimeProfileBinding{
		"dinodive": gameplayTestBinding("game-basic"),
		"missing":  gameplayTestBinding("game-missing"),
	}
	badgeDefs := map[string]apitypes.RuntimeProfileBinding{
		"dinodive-master": gameplayTestBinding("badge-basic"),
		"missing":         gameplayTestBinding("badge-missing"),
	}
	adoptionCost := int64(10)
	profile.Spec.Resources.PetDefs = &petDefs
	profile.Spec.Resources.Voices = &voices
	profile.Spec.Resources.GameDefs = &gameDefs
	profile.Spec.Resources.BadgeDefs = &badgeDefs
	profile.Spec.Gameplay.Adoption = &apitypes.RuntimeProfileAdoptionSpec{Pool: &[]apitypes.RuntimeProfilePetPoolEntry{
		{PetDef: "tragon", Voice: "pet-voice", Weight: 100, AdoptionCost: &adoptionCost},
		{PetDef: "missing", Voice: "pet-voice", Weight: 1},
	}}
	gamePolicy := apitypes.RuntimeProfileGameSpec{
		EnergyCost: 10, PointsCost: 10,
		Reward: apitypes.RuntimeProfileGameRewardSpec{Model: "reward", PetExpMax: 10, BadgeExpMaxPerBadge: 5, Prompt: "Evaluate."},
	}
	profile.Spec.Gameplay.Pet.Games = map[string]apitypes.RuntimeProfileGameSpec{"dinodive": gamePolicy, "missing": gamePolicy}

	runtime := &Runtime{Catalog: catalog}
	rules, err := runtime.resolveProfileRules(WithRuntimeProfile(ctx, profile), "default")
	if err != nil {
		t.Fatalf("resolveProfileRules() error = %v", err)
	}
	if got, want := rules.Spec.PetPool, []ProfilePetPoolEntry{{
		PetDefID: "petdef-basic", VoiceAlias: "pet-voice", Weight: 100, AdoptionCost: &adoptionCost,
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("PetPool = %#v, want %#v", got, want)
	}
	if got, want := rules.Spec.Games, map[string]ProfileGameRule{"game-basic": {GameDefID: "game-basic", Policy: gamePolicy}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Games = %#v, want %#v", got, want)
	}
	if got, want := rules.Spec.BadgeDefs, map[string]string{"dinodive-master": "badge-basic"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("BadgeDefs = %#v, want %#v", got, want)
	}
}

func TestResolveProfileRulesTreatsEmptyProfileMapAsAllowNone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog := testCatalog(t, time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC))
	profile := seedGameplayCatalog(t, ctx, catalog)
	empty := map[string]apitypes.RuntimeProfileBinding{}
	profile.Spec.Resources.GameDefs = &empty
	profile.Spec.Gameplay.Pet.Games = map[string]apitypes.RuntimeProfileGameSpec{}
	runtime := &Runtime{Catalog: catalog}
	rules, err := runtime.resolveProfileRules(WithRuntimeProfile(ctx, profile), "default")
	if err != nil {
		t.Fatalf("resolveProfileRules() error = %v", err)
	}
	if len(rules.Spec.Games) != 0 {
		t.Fatalf("Games = %#v, want none", rules.Spec.Games)
	}
}
