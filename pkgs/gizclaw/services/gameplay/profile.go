package gameplay

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type runtimeProfileContextKey struct{}

// WithRuntimeProfile attaches the immutable registration snapshot used by gameplay calls.
func WithRuntimeProfile(ctx context.Context, profile apitypes.RuntimeProfile) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeProfileContextKey{}, profile)
}

type ProfileRules struct {
	Name string
	Spec ProfileRulesSpec
}

type ProfileRulesSpec struct {
	Actions       map[apitypes.PetBehavior]apitypes.RuntimeProfilePetActionSpec
	BadgeDefs     map[string]string
	Experience    apitypes.RuntimeProfilePetExperienceSpec
	Games         map[string]ProfileGameRule
	PetPool       []ProfilePetPoolEntry
	PetWorkflowID string
	Points        *apitypes.RuntimeProfilePointsSpec
	Time          apitypes.RuntimeProfilePetTimeSpec
}

type ProfileGameRule struct {
	GameDefID string
	Policy    apitypes.RuntimeProfileGameSpec
}

type ProfilePetPoolEntry struct {
	AdoptionCost *int64
	PetDefID     string
	Rarity       *string
	Weight       int64
}

func profileRulesFromContext(ctx context.Context, requestedName string) (ProfileRules, error) {
	profile, gameplay, err := gameplayProfileFromContext(ctx, requestedName)
	if err != nil {
		return ProfileRules{}, err
	}
	if gameplay.Pet == nil {
		return ProfileRules{}, errors.New("gameplay: active RuntimeProfile has no pet gameplay configuration")
	}
	petWorkflowID := strings.TrimSpace(profile.Spec.Workflows.System.Pet)
	if petWorkflowID == "" {
		return ProfileRules{}, errors.New("gameplay: active RuntimeProfile has no system Pet Workflow")
	}
	pool := []ProfilePetPoolEntry{}
	if gameplay.Adoption != nil && gameplay.Adoption.Pool != nil {
		for _, entry := range *gameplay.Adoption.Pool {
			petDefID, exists := resourceAlias(profile.Spec.Resources.PetDefs, entry.PetDef)
			if !exists {
				continue
			}
			pool = append(pool, ProfilePetPoolEntry{
				AdoptionCost: entry.AdoptionCost,
				PetDefID:     petDefID,
				Rarity:       entry.Rarity,
				Weight:       entry.Weight,
			})
		}
	}
	pet := gameplay.Pet
	games := make(map[string]ProfileGameRule, len(pet.Games))
	for alias, policy := range pet.Games {
		gameDefID, exists := resourceAlias(profile.Spec.Resources.GameDefs, alias)
		if !exists {
			continue
		}
		if _, duplicate := games[gameDefID]; duplicate {
			return ProfileRules{}, errors.New("gameplay: multiple game aliases resolve to the same GameDef")
		}
		games[gameDefID] = ProfileGameRule{GameDefID: gameDefID, Policy: policy}
	}
	return ProfileRules{
		Name: profile.Name,
		Spec: ProfileRulesSpec{
			Actions: map[apitypes.PetBehavior]apitypes.RuntimeProfilePetActionSpec{
				apitypes.PetBehaviorFeed:  pet.Actions.Feed,
				apitypes.PetBehaviorBathe: pet.Actions.Bathe,
				apitypes.PetBehaviorPlay:  pet.Actions.Play,
				apitypes.PetBehaviorHeal:  pet.Actions.Heal,
			},
			BadgeDefs:     resourceAliasMap(profile.Spec.Resources.BadgeDefs),
			Experience:    pet.Experience,
			Games:         games,
			PetPool:       pool,
			PetWorkflowID: petWorkflowID,
			Points:        gameplay.Points,
			Time:          pet.Time,
		},
	}, nil
}

func pointsRulesFromContext(ctx context.Context, requestedName string) (ProfileRules, error) {
	profile, gameplay, err := gameplayProfileFromContext(ctx, requestedName)
	if err != nil {
		return ProfileRules{}, err
	}
	return ProfileRules{Name: profile.Name, Spec: ProfileRulesSpec{Points: gameplay.Points}}, nil
}

func gameplayProfileFromContext(ctx context.Context, requestedName string) (apitypes.RuntimeProfile, *apitypes.RuntimeProfileGameplaySpec, error) {
	profile, ok := runtimeProfileFromContext(ctx)
	if !ok || strings.TrimSpace(profile.Name) == "" {
		return apitypes.RuntimeProfile{}, nil, errors.New("gameplay: RuntimeProfile is required")
	}
	requestedName = strings.TrimSpace(requestedName)
	if requestedName != "" && requestedName != profile.Name {
		return apitypes.RuntimeProfile{}, nil, errors.New("gameplay: resource belongs to a different RuntimeProfile")
	}
	if profile.Spec.Gameplay == nil {
		return apitypes.RuntimeProfile{}, nil, errors.New("gameplay: active RuntimeProfile has no gameplay configuration")
	}
	return profile, profile.Spec.Gameplay, nil
}

func resourceAlias(resources *map[string]apitypes.RuntimeProfileBinding, alias string) (string, bool) {
	if resources == nil {
		return "", false
	}
	binding, ok := (*resources)[strings.TrimSpace(alias)]
	value := strings.TrimSpace(binding.ResourceId)
	return value, ok && value != ""
}

func resourceAliasMap(resources *map[string]apitypes.RuntimeProfileBinding) map[string]string {
	if resources == nil {
		return map[string]string{}
	}
	aliases := make([]string, 0, len(*resources))
	for alias := range *resources {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	out := make(map[string]string, len(aliases))
	for _, alias := range aliases {
		if value, ok := resourceAlias(resources, alias); ok {
			out[alias] = value
		}
	}
	return out
}

func resourceValues(resources *map[string]apitypes.RuntimeProfileBinding) []string {
	aliases := resourceAliasMap(resources)
	keys := make([]string, 0, len(aliases))
	for alias := range aliases {
		keys = append(keys, alias)
	}
	sort.Strings(keys)
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, alias := range keys {
		value := aliases[alias]
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func runtimeProfileFromContext(ctx context.Context) (apitypes.RuntimeProfile, bool) {
	if ctx == nil {
		return apitypes.RuntimeProfile{}, false
	}
	profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
	return profile, ok
}
