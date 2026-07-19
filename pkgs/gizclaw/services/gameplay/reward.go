package gameplay

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func defaultReward(profileRules ProfileRules) apitypes.RuntimeProfileRewardSpec {
	if profileRules.Spec.Drive == nil || profileRules.Spec.Drive.DefaultReward == nil {
		return apitypes.RuntimeProfileRewardSpec{}
	}
	return *profileRules.Spec.Drive.DefaultReward
}

func gameReward(profileRules ProfileRules, gameDefID string) apitypes.RuntimeProfileRewardSpec {
	if gameDefID == "" || profileRules.Spec.Drive == nil || profileRules.Spec.Drive.GameRewards == nil {
		return apitypes.RuntimeProfileRewardSpec{}
	}
	return (*profileRules.Spec.Drive.GameRewards)[gameDefID]
}

func mergeRewards(left, right apitypes.RuntimeProfileRewardSpec) apitypes.RuntimeProfileRewardSpec {
	out := apitypes.RuntimeProfileRewardSpec{
		PointsDelta:   int64Ptr(int64Value(left.PointsDelta) + int64Value(right.PointsDelta)),
		PetExpDelta:   int64Ptr(int64Value(left.PetExpDelta) + int64Value(right.PetExpDelta)),
		BadgeExpDelta: mergeIntMap(left.BadgeExpDelta, right.BadgeExpDelta),
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
	return out
}

func rewardEmpty(reward apitypes.RuntimeProfileRewardSpec) bool {
	return int64Value(reward.PointsDelta) == 0 &&
		int64Value(reward.PetExpDelta) == 0 &&
		len(mapValue(reward.BadgeExpDelta)) == 0
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

func petDefAction(petDef apitypes.PetDef, action string) (apitypes.PetDefActionSpec, bool) {
	for _, candidate := range petDef.Spec.Drive.Actions {
		if candidate.Id == action {
			return candidate, true
		}
	}
	return apitypes.PetDefActionSpec{}, false
}

func actionEffectReward(action apitypes.PetDefActionSpec) apitypes.RuntimeProfileRewardSpec {
	if action.Effect == nil || int64Value(action.Effect.PetExpDelta) == 0 {
		return apitypes.RuntimeProfileRewardSpec{}
	}
	return apitypes.RuntimeProfileRewardSpec{PetExpDelta: action.Effect.PetExpDelta}
}

func applyActionEffect(pet *apitypes.Pet, action apitypes.PetDefActionSpec) {
	if pet == nil || action.Effect == nil || action.Effect.AttrDelta == nil {
		return
	}
	applyLifeDelta(pet.Life, action.Effect.AttrDelta.Life)
}

func applyLifeDelta(target apitypes.PetLife, delta *apitypes.PetLife) {
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

func initPetLife(in apitypes.PetAttrGroupSpec) apitypes.PetLife {
	out := apitypes.PetLife{}
	for k, spec := range in {
		out[k] = spec.Initial
	}
	return out
}

func initPetProgression(in apitypes.PetAttrGroupSpec) apitypes.PetProgression {
	out := apitypes.PetProgression{}
	for k, spec := range in {
		out[k] = spec.Initial
	}
	return out
}

func petProgressionExp(pet apitypes.Pet) int64 {
	if pet.Progression == nil {
		return 0
	}
	return pet.Progression["xp"]
}

func applyPetExp(pet *apitypes.Pet, delta int64) {
	if pet == nil || delta == 0 {
		return
	}
	if pet.Progression == nil {
		pet.Progression = apitypes.PetProgression{}
	}
	pet.Progression["xp"] += delta
	if pet.Progression["xp"] < 0 {
		pet.Progression["xp"] = 0
	}
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
