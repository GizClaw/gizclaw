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
	BadgeDefIds []string
	Drive       *apitypes.RuntimeProfileDriveSpec
	GameDefIds  []string
	PetPool     []ProfilePetPoolEntry
	Points      *apitypes.RuntimeProfilePointsSpec
}

type ProfilePetPoolEntry struct {
	AdoptionCost *int64
	PetDefID     string
	Rarity       *string
	Weight       int64
}

func profileRulesFromContext(ctx context.Context, requestedName string) (ProfileRules, error) {
	profile, ok := runtimeProfileFromContext(ctx)
	if !ok || strings.TrimSpace(profile.Name) == "" {
		return ProfileRules{}, errors.New("gameplay: RuntimeProfile is required")
	}
	requestedName = strings.TrimSpace(requestedName)
	if requestedName != "" && requestedName != profile.Name {
		return ProfileRules{}, errors.New("gameplay: resource belongs to a different RuntimeProfile")
	}
	if profile.Spec.Gameplay == nil {
		return ProfileRules{}, errors.New("gameplay: active RuntimeProfile has no gameplay configuration")
	}
	gameplay := profile.Spec.Gameplay
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
	return ProfileRules{
		Name: profile.Name,
		Spec: ProfileRulesSpec{
			BadgeDefIds: resourceValues(profile.Spec.Resources.BadgeDefs),
			Drive:       resolveDrive(gameplay.Rewards, profile.Spec.Resources.GameDefs, profile.Spec.Resources.BadgeDefs),
			GameDefIds:  resourceValues(profile.Spec.Resources.GameDefs),
			PetPool:     pool,
			Points:      gameplay.Points,
		},
	}, nil
}

func resourceAlias(resources *map[string]apitypes.RuntimeProfileBinding, alias string) (string, bool) {
	if resources == nil {
		return "", false
	}
	binding, ok := (*resources)[strings.TrimSpace(alias)]
	value := strings.TrimSpace(binding.ResourceId)
	return value, ok && value != ""
}

func resourceValues(resources *map[string]apitypes.RuntimeProfileBinding) []string {
	if resources == nil {
		return nil
	}
	aliases := make([]string, 0, len(*resources))
	for alias := range *resources {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	seen := make(map[string]struct{}, len(aliases))
	out := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		value := strings.TrimSpace((*resources)[alias].ResourceId)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func resolveDrive(in *apitypes.RuntimeProfileDriveSpec, gameDefs, badgeDefs *map[string]apitypes.RuntimeProfileBinding) *apitypes.RuntimeProfileDriveSpec {
	if in == nil {
		return nil
	}
	out := &apitypes.RuntimeProfileDriveSpec{}
	if in.Default != nil {
		reward := resolveReward(*in.Default, badgeDefs)
		out.Default = &reward
	}
	if in.Games != nil {
		aliases := make([]string, 0, len(*in.Games))
		for alias := range *in.Games {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		resolved := make(map[string]apitypes.RuntimeProfileRewardSpec, len(aliases))
		for _, alias := range aliases {
			gameDefID, exists := resourceAlias(gameDefs, alias)
			if !exists {
				continue
			}
			if _, exists := resolved[gameDefID]; exists {
				continue
			}
			resolved[gameDefID] = resolveReward((*in.Games)[alias], badgeDefs)
		}
		out.Games = &resolved
	}
	if in.PetActions != nil {
		resolved := make(map[string]apitypes.RuntimeProfileRewardSpec, len(*in.PetActions))
		for action, reward := range *in.PetActions {
			resolved[action] = resolveReward(reward, badgeDefs)
		}
		out.PetActions = &resolved
	}
	return out
}

func resolveReward(in apitypes.RuntimeProfileRewardSpec, badgeDefs *map[string]apitypes.RuntimeProfileBinding) apitypes.RuntimeProfileRewardSpec {
	out := apitypes.RuntimeProfileRewardSpec{
		PetExpDelta: in.PetExpDelta,
		PointsDelta: in.PointsDelta,
	}
	if in.BadgeExpDelta == nil {
		return out
	}
	aliases := make([]string, 0, len(*in.BadgeExpDelta))
	for alias := range *in.BadgeExpDelta {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	resolved := make(map[string]int64, len(aliases))
	for _, alias := range aliases {
		badgeDefID, exists := resourceAlias(badgeDefs, alias)
		if !exists {
			continue
		}
		if _, exists := resolved[badgeDefID]; exists {
			continue
		}
		resolved[badgeDefID] = (*in.BadgeExpDelta)[alias]
	}
	out.BadgeExpDelta = &resolved
	return out
}

func runtimeProfileFromContext(ctx context.Context) (apitypes.RuntimeProfile, bool) {
	if ctx == nil {
		return apitypes.RuntimeProfile{}, false
	}
	profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
	return profile, ok
}
