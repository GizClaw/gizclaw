package gameplay

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func defaultReward(ruleset apitypes.GameRuleset) apitypes.GameRewardSpec {
	if ruleset.Spec.Drive == nil || ruleset.Spec.Drive.DefaultReward == nil {
		return apitypes.GameRewardSpec{}
	}
	return *ruleset.Spec.Drive.DefaultReward
}

func actionReward(ruleset apitypes.GameRuleset, action string) apitypes.GameRewardSpec {
	if action == "" || ruleset.Spec.Drive == nil || ruleset.Spec.Drive.ActionRewards == nil {
		return apitypes.GameRewardSpec{}
	}
	return (*ruleset.Spec.Drive.ActionRewards)[action]
}

func gameReward(ruleset apitypes.GameRuleset, gameDefID string) apitypes.GameRewardSpec {
	if gameDefID == "" || ruleset.Spec.Drive == nil || ruleset.Spec.Drive.GameRewards == nil {
		return apitypes.GameRewardSpec{}
	}
	return (*ruleset.Spec.Drive.GameRewards)[gameDefID]
}

func actionCost(ruleset apitypes.GameRuleset, action string) int64 {
	if action == "" || ruleset.Spec.Drive == nil || ruleset.Spec.Drive.ActionCosts == nil {
		return 0
	}
	return (*ruleset.Spec.Drive.ActionCosts)[action]
}

func mergeRewards(left, right apitypes.GameRewardSpec) apitypes.GameRewardSpec {
	out := apitypes.GameRewardSpec{
		PointsDelta:   int64Ptr(int64Value(left.PointsDelta) + int64Value(right.PointsDelta)),
		PetExpDelta:   int64Ptr(int64Value(left.PetExpDelta) + int64Value(right.PetExpDelta)),
		BadgeExpDelta: mergeIntMap(left.BadgeExpDelta, right.BadgeExpDelta),
		LifeDelta:     mergeStatMap(left.LifeDelta, right.LifeDelta),
		AbilityDelta:  mergeStatMap(left.AbilityDelta, right.AbilityDelta),
	}
	if int64Value(out.PointsDelta) == 0 {
		out.PointsDelta = nil
	}
	if int64Value(out.PetExpDelta) == 0 {
		out.PetExpDelta = nil
	}
	if len(mapValue(out.BadgeExpDelta)) == 0 {
		out.BadgeExpDelta = nil
	}
	if out.LifeDelta != nil && len(*out.LifeDelta) == 0 {
		out.LifeDelta = nil
	}
	if out.AbilityDelta != nil && len(*out.AbilityDelta) == 0 {
		out.AbilityDelta = nil
	}
	return out
}

func rewardEmpty(reward apitypes.GameRewardSpec) bool {
	return int64Value(reward.PointsDelta) == 0 &&
		int64Value(reward.PetExpDelta) == 0 &&
		len(mapValue(reward.BadgeExpDelta)) == 0 &&
		(reward.LifeDelta == nil || len(*reward.LifeDelta) == 0) &&
		(reward.AbilityDelta == nil || len(*reward.AbilityDelta) == 0)
}

func mergeIntMap(left, right *map[string]int64) *map[string]int64 {
	out := map[string]int64{}
	for k, v := range mapValue(left) {
		out[k] += v
	}
	for k, v := range mapValue(right) {
		out[k] += v
	}
	return &out
}

func mergeStatMap(left, right *apitypes.StatMap) *apitypes.StatMap {
	out := apitypes.StatMap{}
	if left != nil {
		for k, v := range *left {
			out[k] += v
		}
	}
	if right != nil {
		for k, v := range *right {
			out[k] += v
		}
	}
	return &out
}

func applyStatDelta(target apitypes.StatMap, delta *apitypes.StatMap) {
	if target == nil || delta == nil {
		return
	}
	for k, v := range *delta {
		target[k] += v
		if target[k] < 0 {
			target[k] = 0
		}
	}
}

func cloneStatMap(in apitypes.StatMap) apitypes.StatMap {
	out := apitypes.StatMap{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapValue(in *map[string]int64) map[string]int64 {
	if in == nil {
		return map[string]int64{}
	}
	return *in
}

func int64Value(in *int64) int64 {
	if in == nil {
		return 0
	}
	return *in
}

func int64Ptr(v int64) *int64 {
	return &v
}

func stringPtr(v string) *string {
	return &v
}

func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func rewardReason(action string, result *apitypes.GameResult) string {
	if result != nil {
		return "game_result." + result.GameDefId
	}
	if action != "" {
		return "action." + action
	}
	return "time"
}

func rewardSource(action string, result *apitypes.GameResult, petID string) (string, string) {
	if result != nil {
		return "game_result", result.Id
	}
	if action != "" {
		return "pet_action", action
	}
	return "time", petID
}
