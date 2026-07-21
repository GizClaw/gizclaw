package gameplay

import (
	"math"
	"sort"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const petStatMaximum = 100.0

func initialPetStats() apitypes.PetStats {
	return apitypes.PetStats{
		Life: petStatMaximum, Health: petStatMaximum, Satiety: petStatMaximum,
		Hygiene: petStatMaximum, Mood: petStatMaximum, Energy: petStatMaximum,
	}
}

func initialPetProgression() apitypes.PetProgression {
	return apitypes.PetProgression{Experience: 0, Level: 1}
}

func applyCareBehavior(pet *apitypes.Pet, behavior apitypes.PetBehavior, delta float64) {
	if pet == nil || delta <= 0 {
		return
	}
	switch behavior {
	case apitypes.PetBehaviorFeed:
		pet.Stats.Satiety = clampPetStat(pet.Stats.Satiety + delta)
	case apitypes.PetBehaviorBathe:
		pet.Stats.Hygiene = clampPetStat(pet.Stats.Hygiene + delta)
	case apitypes.PetBehaviorPlay:
		pet.Stats.Mood = clampPetStat(pet.Stats.Mood + delta)
	case apitypes.PetBehaviorHeal:
		pet.Stats.Health = clampPetStat(pet.Stats.Health + delta)
	}
}

func applyPetExp(pet *apitypes.Pet, delta int64, leveling apitypes.RuntimeProfileLevelingSpec) {
	if pet == nil || delta <= 0 {
		return
	}
	if delta > math.MaxInt64-pet.Progression.Experience {
		pet.Progression.Experience = math.MaxInt64
	} else {
		pet.Progression.Experience += delta
	}
	pet.Progression.Level = petLevel(pet.Progression.Experience, leveling)
}

func petLevel(experience int64, leveling apitypes.RuntimeProfileLevelingSpec) int64 {
	if experience <= 0 || leveling.BaseExp <= 0 {
		return 1
	}
	maxCompleted := experience / leveling.BaseExp
	spent := int64(0)
	for level := int64(1); level <= maxCompleted; {
		required := petLevelRequirement(level, leveling)
		last := lastLevelWithRequirement(level, maxCompleted, required, leveling)
		count := last - level + 1
		affordable := (experience - spent) / required
		if affordable < count {
			return level + affordable
		}
		spent += count * required
		if last == math.MaxInt64 {
			return math.MaxInt64
		}
		level = last + 1
	}
	return maxCompleted + 1
}

func petLevelRequirement(level int64, leveling apitypes.RuntimeProfileLevelingSpec) int64 {
	required := math.Ceil(float64(leveling.BaseExp) + leveling.LogScale*math.Log(float64(level)))
	if required >= math.MaxInt64 {
		return math.MaxInt64
	}
	if required < 1 {
		return 1
	}
	return int64(required)
}

func lastLevelWithRequirement(first, last, required int64, leveling apitypes.RuntimeProfileLevelingSpec) int64 {
	for first < last {
		middle := first + (last-first+1)/2
		if petLevelRequirement(middle, leveling) <= required {
			first = middle
		} else {
			last = middle - 1
		}
	}
	return first
}

func settlePetTime(pet *apitypes.Pet, now time.Time, policy apitypes.RuntimeProfilePetTimeSpec) {
	if pet == nil || pet.Stats.Life <= 0 || !now.After(pet.StateSettledAt) {
		return
	}
	start := pet.StateSettledAt
	hours := now.Sub(start).Hours()
	settledHours := hours
	died := false
	lifeLoss := integratedLifeLoss(pet.Stats, hours, policy)
	if lifeLoss >= pet.Stats.Life {
		settledHours = hoursUntilLifeLoss(pet.Stats, pet.Stats.Life, hours, policy)
		died = true
	}
	pet.Stats.Life = clampPetStat(pet.Stats.Life - lifeLoss)
	if died {
		pet.Stats.Life = 0
		nanoseconds := math.Round(settledHours * float64(time.Hour))
		now = start.Add(time.Duration(nanoseconds))
	}
	pet.Stats.Health = decayedStat(pet.Stats.Health, policy.CareDecayPerHour.Health, settledHours)
	pet.Stats.Satiety = decayedStat(pet.Stats.Satiety, policy.CareDecayPerHour.Satiety, settledHours)
	pet.Stats.Hygiene = decayedStat(pet.Stats.Hygiene, policy.CareDecayPerHour.Hygiene, settledHours)
	pet.Stats.Mood = decayedStat(pet.Stats.Mood, policy.CareDecayPerHour.Mood, settledHours)
	pet.Stats.Energy = clampPetStat(pet.Stats.Energy + policy.EnergyRecoveryPerHour*settledHours)
	pet.StateSettledAt = now
}

func hoursUntilLifeLoss(stats apitypes.PetStats, target, maximum float64, policy apitypes.RuntimeProfilePetTimeSpec) float64 {
	lower, upper := 0.0, maximum
	for range 80 {
		middle := lower + (upper-lower)/2
		if integratedLifeLoss(stats, middle, policy) >= target {
			upper = middle
		} else {
			lower = middle
		}
	}
	return upper
}

type careTrajectory struct {
	initial float64
	decay   float64
	weight  float64
}

func integratedLifeLoss(stats apitypes.PetStats, hours float64, policy apitypes.RuntimeProfilePetTimeSpec) float64 {
	if hours <= 0 || policy.LifeDecay.MaxLossPerHour <= 0 {
		return 0
	}
	decay := policy.CareDecayPerHour
	weights := policy.LifeDecay.ContributingWeights
	trajectories := []careTrajectory{
		{initial: stats.Health, decay: decay.Health, weight: weights.Health},
		{initial: stats.Satiety, decay: decay.Satiety, weight: weights.Satiety},
		{initial: stats.Hygiene, decay: decay.Hygiene, weight: weights.Hygiene},
		{initial: stats.Mood, decay: decay.Mood, weight: weights.Mood},
	}
	breaks := []float64{0, hours}
	for _, trajectory := range trajectories {
		if trajectory.decay <= 0 || trajectory.initial <= 0 {
			continue
		}
		zeroAt := trajectory.initial / trajectory.decay
		if zeroAt > 0 && zeroAt < hours {
			breaks = append(breaks, zeroAt)
		}
	}
	sort.Float64s(breaks)
	exponent := policy.LifeDecay.Exponent
	integral := 0.0
	for i := 0; i+1 < len(breaks); i++ {
		start, end := breaks[i], breaks[i+1]
		if end <= start {
			continue
		}
		deficit, slope := 0.0, 0.0
		for _, trajectory := range trajectories {
			value := decayedStat(trajectory.initial, trajectory.decay, start)
			deficit += trajectory.weight * (1 - value/petStatMaximum)
			if value > 0 {
				slope += trajectory.weight * trajectory.decay / petStatMaximum
			}
		}
		duration := end - start
		if math.Abs(slope) < 1e-15 {
			integral += math.Pow(deficit, exponent) * duration
			continue
		}
		integral += (math.Pow(deficit+slope*duration, exponent+1) - math.Pow(deficit, exponent+1)) / (slope * (exponent + 1))
	}
	return policy.LifeDecay.MaxLossPerHour * integral
}

func decayedStat(value, rate, hours float64) float64 {
	return clampPetStat(value - rate*hours)
}

func clampPetStat(value float64) float64 {
	return min(petStatMaximum, max(0, value))
}

func int64Value(in *int64) int64 {
	if in == nil {
		return 0
	}
	return *in
}

func stringPtr(v string) *string { return &v }

func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
