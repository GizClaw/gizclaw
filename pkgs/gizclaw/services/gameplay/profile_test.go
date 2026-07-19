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

	petDefs := map[string]string{
		"tragon":  "petdef-basic",
		"missing": "petdef-missing",
	}
	gameDefs := map[string]string{
		"dinodive": "game-basic",
		"missing":  "game-missing",
	}
	badgeDefs := map[string]string{
		"dinodive-master": "badge-basic",
		"missing":         "badge-missing",
	}
	adoptionCost := int64(10)
	profile.Spec.Resources.PetDefs = &petDefs
	profile.Spec.Resources.GameDefs = &gameDefs
	profile.Spec.Resources.BadgeDefs = &badgeDefs
	profile.Spec.Gameplay.PetPool = &[]apitypes.RuntimeProfilePetPoolEntry{
		{PetDef: "tragon", Weight: 100, AdoptionCost: &adoptionCost},
		{PetDef: "missing", Weight: 1},
	}
	badgeDelta := map[string]int64{"dinodive-master": 100, "missing": 200}
	missingBadgeDelta := map[string]int64{"missing": 300}
	profile.Spec.Gameplay.Drive = &apitypes.RuntimeProfileDriveSpec{
		DefaultReward: &apitypes.RuntimeProfileRewardSpec{BadgeExpDelta: &badgeDelta},
		GameRewards: &map[string]apitypes.RuntimeProfileRewardSpec{
			"dinodive": {BadgeExpDelta: &badgeDelta},
			"missing":  {BadgeExpDelta: &missingBadgeDelta},
		},
	}

	runtime := &Runtime{Catalog: catalog}
	rules, err := runtime.resolveProfileRules(WithRuntimeProfile(ctx, profile), "default")
	if err != nil {
		t.Fatalf("resolveProfileRules() error = %v", err)
	}
	if got, want := rules.Spec.PetPool, []ProfilePetPoolEntry{{
		PetDefID: "petdef-basic", Weight: 100, AdoptionCost: &adoptionCost,
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("PetPool = %#v, want %#v", got, want)
	}
	if got, want := rules.Spec.GameDefIds, []string{"game-basic"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GameDefIds = %#v, want %#v", got, want)
	}
	if got, want := rules.Spec.BadgeDefIds, []string{"badge-basic"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("BadgeDefIds = %#v, want %#v", got, want)
	}
	if rules.Spec.Drive == nil || rules.Spec.Drive.GameRewards == nil {
		t.Fatalf("Drive = %#v, want resolved rewards", rules.Spec.Drive)
	}
	wantRewards := map[string]apitypes.RuntimeProfileRewardSpec{
		"game-basic": {BadgeExpDelta: &map[string]int64{"badge-basic": 100}},
	}
	if got := *rules.Spec.Drive.GameRewards; !reflect.DeepEqual(got, wantRewards) {
		t.Fatalf("GameRewards = %#v, want %#v", got, wantRewards)
	}
	wantDefault := map[string]int64{"badge-basic": 100}
	if rules.Spec.Drive.DefaultReward == nil || rules.Spec.Drive.DefaultReward.BadgeExpDelta == nil ||
		!reflect.DeepEqual(*rules.Spec.Drive.DefaultReward.BadgeExpDelta, wantDefault) {
		t.Fatalf("DefaultReward = %#v, want badge aliases resolved and missing refs skipped", rules.Spec.Drive.DefaultReward)
	}
}

func TestValidateGameResultTreatsEmptyProfileMapAsAllowNone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog := testCatalog(t, time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC))
	profile := seedGameplayCatalog(t, ctx, catalog)
	empty := map[string]string{}
	profile.Spec.Resources.GameDefs = &empty
	runtime := &Runtime{Catalog: catalog}
	rules, err := runtime.resolveProfileRules(WithRuntimeProfile(ctx, profile), "default")
	if err != nil {
		t.Fatalf("resolveProfileRules() error = %v", err)
	}
	if err := runtime.validateGameResult(ctx, rules, "game-basic"); err == nil {
		t.Fatal("validateGameResult() allowed a GameDef absent from RuntimeProfile")
	}
}
