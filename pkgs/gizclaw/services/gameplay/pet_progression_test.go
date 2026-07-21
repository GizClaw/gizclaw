package gameplay

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestPetTimeSettlementIsFrequencyIndependent(t *testing.T) {
	policy := testPetGameplaySpec().Time
	start := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	oneShot := apitypes.Pet{Stats: initialPetStats(), StateSettledAt: start}
	hourly := oneShot

	settlePetTime(&oneShot, start.Add(96*time.Hour), policy)
	for hour := 1; hour <= 96; hour++ {
		settlePetTime(&hourly, start.Add(time.Duration(hour)*time.Hour), policy)
	}

	assertPetStatsClose(t, hourly.Stats, oneShot.Stats)
	if hourly.StateSettledAt != oneShot.StateSettledAt {
		t.Fatalf("settled_at = hourly %s, one-shot %s", hourly.StateSettledAt, oneShot.StateSettledAt)
	}
	if oneShot.Stats.Energy != petStatMaximum {
		t.Fatalf("passive energy = %g, want 100", oneShot.Stats.Energy)
	}
}

func TestEmptyPetDriveSettlesAndPersistsTime(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-empty-drive", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	pet := adopted.Pet
	pet.Stats.Health = 70
	pet.Stats.Satiety = 40
	pet.Stats.Hygiene = 50
	pet.Stats.Mood = 60
	pet.Stats.Energy = 20
	updatePetForTest(t, runtime, pet)

	*now = now.Add(2 * time.Hour)
	want := pet
	settlePetTime(&want, *now, testPetGameplaySpec().Time)
	want.UpdatedAt = *now
	response, err := runtime.DrivePet(ctx, "peer-empty-drive", apitypes.PetDriveRequest{PetId: pet.Id})
	if err != nil {
		t.Fatalf("DrivePet(empty) error = %v", err)
	}
	assertPetStatsClose(t, response.Pet.Stats, want.Stats)
	if response.Pet.StateSettledAt != *now || response.Pet.UpdatedAt != *now || response.Pet.LastActiveAt != pet.LastActiveAt {
		t.Fatalf("DrivePet(empty) timestamps = settled %s updated %s active %s", response.Pet.StateSettledAt, response.Pet.UpdatedAt, response.Pet.LastActiveAt)
	}
	if response.GameResult != nil || len(response.Badges) != 0 || len(response.RewardGrants) != 0 || len(response.Transactions) != 0 {
		t.Fatalf("DrivePet(empty) side effects = %#v", response)
	}
	stored, err := runtime.GetPet(ctx, "peer-empty-drive", pet.Id)
	if err != nil {
		t.Fatalf("GetPet() error = %v", err)
	}
	assertPetStatsClose(t, stored.Stats, want.Stats)
	if stored.StateSettledAt != response.Pet.StateSettledAt || stored.LastActiveAt != pet.LastActiveAt {
		t.Fatalf("stored Pet = %#v, want settled response with unchanged activity", stored)
	}
	results, err := runtime.ListGameResults(ctx, "peer-empty-drive", apitypes.GameplayListRequest{})
	if err != nil || len(results.Items) != 0 {
		t.Fatalf("game results after empty Drive = %#v, %v", results, err)
	}
	grants, err := runtime.ListRewardGrants(ctx, "peer-empty-drive", apitypes.GameplayListRequest{})
	if err != nil || len(grants.Items) != 0 {
		t.Fatalf("reward grants after empty Drive = %#v, %v", grants, err)
	}
	transactions, err := runtime.ListPointsTransactions(ctx, "peer-empty-drive", apitypes.GameplayListRequest{})
	if err != nil || len(transactions.Items) != 1 || transactions.Items[0].Reason != "pet.adopt" {
		t.Fatalf("points transactions after empty Drive = %#v, %v", transactions, err)
	}
}

func TestEmptyPetDriveTimeSettlementIsFrequencyIndependent(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	first, err := runtime.AdoptPet(ctx, "peer-empty-frequency", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet(first) error = %v", err)
	}
	second, err := runtime.AdoptPet(ctx, "peer-empty-frequency", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet(second) error = %v", err)
	}
	for _, pet := range []apitypes.Pet{first.Pet, second.Pet} {
		pet.Stats.Health = 70
		pet.Stats.Satiety = 40
		pet.Stats.Hygiene = 50
		pet.Stats.Mood = 60
		pet.Stats.Energy = 20
		updatePetForTest(t, runtime, pet)
	}

	*now = now.Add(time.Hour)
	if _, err := runtime.DrivePet(ctx, "peer-empty-frequency", apitypes.PetDriveRequest{PetId: first.Pet.Id}); err != nil {
		t.Fatalf("DrivePet(first interval) error = %v", err)
	}
	*now = now.Add(2 * time.Hour)
	split, err := runtime.DrivePet(ctx, "peer-empty-frequency", apitypes.PetDriveRequest{PetId: first.Pet.Id})
	if err != nil {
		t.Fatalf("DrivePet(second interval) error = %v", err)
	}
	oneShot, err := runtime.DrivePet(ctx, "peer-empty-frequency", apitypes.PetDriveRequest{PetId: second.Pet.Id})
	if err != nil {
		t.Fatalf("DrivePet(one interval) error = %v", err)
	}
	assertPetStatsClose(t, split.Pet.Stats, oneShot.Pet.Stats)
}

func TestEmptyPetDriveDeathSettlementIsFrequencyIndependent(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	start := *now
	first, err := runtime.AdoptPet(ctx, "peer-empty-death", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet(first) error = %v", err)
	}
	second, err := runtime.AdoptPet(ctx, "peer-empty-death", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet(second) error = %v", err)
	}
	for _, pet := range []apitypes.Pet{first.Pet, second.Pet} {
		pet.Stats = apitypes.PetStats{Life: 5}
		updatePetForTest(t, runtime, pet)
	}

	*now = start.Add(time.Hour)
	if _, err := runtime.DrivePet(ctx, "peer-empty-death", apitypes.PetDriveRequest{PetId: first.Pet.Id}); err != nil {
		t.Fatalf("DrivePet(first interval) error = %v", err)
	}
	*now = start.Add(3 * time.Hour)
	split, err := runtime.DrivePet(ctx, "peer-empty-death", apitypes.PetDriveRequest{PetId: first.Pet.Id})
	if err != nil {
		t.Fatalf("DrivePet(second interval) error = %v", err)
	}
	oneShot, err := runtime.DrivePet(ctx, "peer-empty-death", apitypes.PetDriveRequest{PetId: second.Pet.Id})
	if err != nil {
		t.Fatalf("DrivePet(one interval) error = %v", err)
	}

	wantDeath := start.Add(75 * time.Minute)
	if split.Pet.Lifecycle != apitypes.PetLifecycleDead || oneShot.Pet.Lifecycle != apitypes.PetLifecycleDead {
		t.Fatalf("death lifecycle = split %q, one-shot %q", split.Pet.Lifecycle, oneShot.Pet.Lifecycle)
	}
	if split.Pet.DiedAt == nil || oneShot.Pet.DiedAt == nil || *split.Pet.DiedAt != wantDeath || *oneShot.Pet.DiedAt != wantDeath {
		t.Fatalf("died_at = split %v, one-shot %v, want %s", split.Pet.DiedAt, oneShot.Pet.DiedAt, wantDeath)
	}
	if split.Pet.StateSettledAt != oneShot.Pet.StateSettledAt || split.Pet.UpdatedAt != oneShot.Pet.UpdatedAt {
		t.Fatalf("terminal timestamps = split %#v, one-shot %#v", split.Pet, oneShot.Pet)
	}
	assertPetStatsClose(t, split.Pet.Stats, oneShot.Pet.Stats)
}

func TestEmptyPetDriveIdempotencyKeyPreventsRepeatedTick(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	first, err := runtime.AdoptPet(ctx, "peer-empty-key", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet(first) error = %v", err)
	}
	second, err := runtime.AdoptPet(ctx, "peer-empty-key", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet(second) error = %v", err)
	}
	key := "empty-tick"
	*now = now.Add(time.Hour)
	initial, err := runtime.DrivePet(ctx, "peer-empty-key", apitypes.PetDriveRequest{PetId: first.Pet.Id, IdempotencyKey: &key})
	if err != nil {
		t.Fatalf("DrivePet(first) error = %v", err)
	}
	*now = now.Add(2 * time.Hour)
	replay, err := runtime.DrivePet(ctx, "peer-empty-key", apitypes.PetDriveRequest{PetId: first.Pet.Id, IdempotencyKey: &key})
	if err != nil {
		t.Fatalf("DrivePet(replay) error = %v", err)
	}
	if replay.Pet.StateSettledAt != initial.Pet.StateSettledAt || replay.Pet.UpdatedAt != initial.Pet.UpdatedAt {
		t.Fatalf("DrivePet(replay) advanced Pet = %#v, want settled_at %s", replay.Pet, initial.Pet.StateSettledAt)
	}
	if _, err := runtime.DrivePet(ctx, "peer-empty-key", apitypes.PetDriveRequest{PetId: second.Pet.Id, IdempotencyKey: &key}); err == nil || !strings.Contains(err.Error(), "another pet") {
		t.Fatalf("DrivePet(cross-Pet key) error = %v, want conflict", err)
	}
	failedKey := "failed-empty-tick"
	if _, err := runtime.DrivePet(ctx, "peer-empty-key", apitypes.PetDriveRequest{PetId: "missing", IdempotencyKey: &failedKey}); err == nil {
		t.Fatal("DrivePet(missing Pet) error = nil")
	}
	if _, err := runtime.DrivePet(ctx, "peer-empty-key", apitypes.PetDriveRequest{PetId: second.Pet.Id, IdempotencyKey: &failedKey}); err != nil {
		t.Fatalf("DrivePet(key after failure) error = %v", err)
	}
	restarted := &Runtime{
		DB: runtime.DB, Catalog: runtime.Catalog, Workflows: runtime.Workflows, Workspaces: runtime.Workspaces,
		Now: runtime.Now, NewID: runtime.NewID, PickWeight: runtime.PickWeight,
	}
	if afterRestart, err := restarted.DrivePet(ctx, "peer-empty-key", apitypes.PetDriveRequest{PetId: first.Pet.Id, IdempotencyKey: &key}); err != nil || afterRestart.Pet.StateSettledAt != initial.Pet.StateSettledAt {
		t.Fatalf("DrivePet(replay after restart) = %#v, %v", afterRestart, err)
	}
	nextKey := "empty-tick-next"
	next, err := restarted.DrivePet(ctx, "peer-empty-key", apitypes.PetDriveRequest{PetId: first.Pet.Id, IdempotencyKey: &nextKey})
	if err != nil {
		t.Fatalf("DrivePet(next) error = %v", err)
	}
	if next.Pet.StateSettledAt != *now {
		t.Fatalf("DrivePet(next) settled_at = %s, want %s", next.Pet.StateSettledAt, *now)
	}
}

func TestEmptyPetDriveFailureDoesNotConsumeIdempotencyKey(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-empty-failure", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	const trigger = `CREATE TRIGGER fail_empty_pet_drive
		BEFORE UPDATE ON gameplay_pets
		BEGIN
			SELECT RAISE(ABORT, 'forced empty Drive failure');
		END`
	if _, err := runtime.DB.ExecContext(ctx, trigger); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = runtime.DB.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS fail_empty_pet_drive")
	})

	key := "retry-after-rollback"
	*now = now.Add(time.Hour)
	request := apitypes.PetDriveRequest{PetId: adopted.Pet.Id, IdempotencyKey: &key}
	if _, err := runtime.DrivePet(ctx, "peer-empty-failure", request); err == nil {
		t.Fatal("DrivePet(forced failure) error = nil")
	}
	var ticks int
	if err := runtime.DB.GetContext(ctx, &ticks, `SELECT COUNT(*) FROM gameplay_pet_drive_ticks WHERE idempotency_key = ?`, key); err != nil {
		t.Fatalf("count rolled-back tick: %v", err)
	}
	if ticks != 0 {
		t.Fatalf("ticks after failed Drive = %d, want 0", ticks)
	}
	if _, err := runtime.DB.ExecContext(ctx, "DROP TRIGGER fail_empty_pet_drive"); err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}

	response, err := runtime.DrivePet(ctx, "peer-empty-failure", request)
	if err != nil {
		t.Fatalf("DrivePet(retry) error = %v", err)
	}
	if response.Pet.StateSettledAt != *now {
		t.Fatalf("DrivePet(retry) settled_at = %s, want %s", response.Pet.StateSettledAt, *now)
	}
}

func TestPetDriveRejectsBehaviorAndGameResultTogether(t *testing.T) {
	ctx, runtime, _ := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-invalid-drive", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	behavior := apitypes.PetBehaviorFeed
	_, err = runtime.DrivePet(ctx, "peer-invalid-drive", apitypes.PetDriveRequest{
		PetId: adopted.Pet.Id, Behavior: &behavior,
		GameResult: &apitypes.PetDriveGameResultInput{GameDefId: "game-basic"},
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one behavior or game_result") {
		t.Fatalf("DrivePet(behavior and game) error = %v", err)
	}
}

func TestPetCareBehaviorUsesDeltaCapEnergyAndExperience(t *testing.T) {
	ctx, runtime, _ := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-care", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	key := "feed-once"
	behavior := apitypes.PetBehaviorFeed
	response, err := runtime.DrivePet(ctx, "peer-care", apitypes.PetDriveRequest{
		PetId: adopted.Pet.Id, Behavior: &behavior, IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("DrivePet(feed) error = %v", err)
	}
	if response.Pet.Stats.Satiety != 100 || response.Pet.Stats.Energy != 90 {
		t.Fatalf("feed stats = %#v, want satiety capped at 100 and energy 90", response.Pet.Stats)
	}
	if response.Pet.Progression.Experience != 2 || response.Pet.Progression.Level != 1 {
		t.Fatalf("feed progression = %#v, want EXP 2 level 1", response.Pet.Progression)
	}
	duplicate, err := runtime.DrivePet(ctx, "peer-care", apitypes.PetDriveRequest{
		PetId: adopted.Pet.Id, Behavior: &behavior, IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("DrivePet(feed duplicate) error = %v", err)
	}
	if duplicate.Pet.Stats.Energy != 90 || duplicate.Pet.Progression.Experience != 2 {
		t.Fatalf("duplicate changed Pet = %#v", duplicate.Pet)
	}
	if len(duplicate.RewardGrants) != 1 || duplicate.RewardGrants[0].Id != response.RewardGrants[0].Id {
		t.Fatalf("duplicate reward = %#v, want original grant", duplicate.RewardGrants)
	}
	heal := apitypes.PetBehaviorHeal
	if _, err := runtime.DrivePet(ctx, "peer-care", apitypes.PetDriveRequest{
		PetId: adopted.Pet.Id, Behavior: &heal, IdempotencyKey: &key,
	}); err == nil || !strings.Contains(err.Error(), "another behavior") {
		t.Fatalf("DrivePet(reused behavior key) error = %v, want behavior mismatch", err)
	}
}

func TestPetLevelIsBoundedForMaximumExperience(t *testing.T) {
	leveling := apitypes.RuntimeProfileLevelingSpec{BaseExp: 1, LogScale: 0}
	if got := petLevel(math.MaxInt64, leveling); got != math.MaxInt64 {
		t.Fatalf("petLevel(MaxInt64) = %d, want saturated MaxInt64", got)
	}
}

func TestUnconfiguredGameIsExactNoOp(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-noop", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	original := adopted.Pet
	*now = now.Add(24 * time.Hour)
	evaluations := 0
	ctx = WithRewardEvaluator(ctx, rewardEvaluatorFunc(func(context.Context, RewardEvaluationRequest) (apitypes.GameRewardSpec, error) {
		evaluations++
		return apitypes.GameRewardSpec{}, nil
	}))
	key := "ignored-game"
	response, err := runtime.DrivePet(ctx, "peer-noop", apitypes.PetDriveRequest{
		PetId: original.Id,
		GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId: "not-configured", IdempotencyKey: &key,
		},
	})
	if err != nil {
		t.Fatalf("DrivePet(unconfigured) error = %v", err)
	}
	if evaluations != 0 || response.GameResult != nil || len(response.RewardGrants) != 0 || len(response.Transactions) != 0 {
		t.Fatalf("unconfigured response = %#v, evaluations = %d", response, evaluations)
	}
	stored, err := runtime.GetPet(ctx, "peer-noop", original.Id)
	if err != nil {
		t.Fatalf("GetPet() error = %v", err)
	}
	if stored.StateSettledAt != original.StateSettledAt || stored.Stats != original.Stats || stored.UpdatedAt != original.UpdatedAt {
		t.Fatalf("unconfigured game mutated Pet\n got: %#v\nwant: %#v", stored, original)
	}
}

func TestGameRewardFailureIsAtomicAndDeathIsTerminal(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-atomic", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	original := adopted.Pet
	*now = now.Add(time.Hour)
	modelErr := errors.New("reward unavailable")
	ctx = WithRewardEvaluator(ctx, rewardEvaluatorFunc(func(context.Context, RewardEvaluationRequest) (apitypes.GameRewardSpec, error) {
		return apitypes.GameRewardSpec{}, modelErr
	}))
	key := "failed-game"
	_, err = runtime.DrivePet(ctx, "peer-atomic", apitypes.PetDriveRequest{
		PetId: original.Id,
		GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId: "game-basic", IdempotencyKey: &key,
		},
	})
	if !errors.Is(err, modelErr) {
		t.Fatalf("DrivePet(model failure) error = %v, want %v", err, modelErr)
	}
	stored, err := runtime.GetPet(ctx, "peer-atomic", original.Id)
	if err != nil {
		t.Fatalf("GetPet() error = %v", err)
	}
	if stored.StateSettledAt != original.StateSettledAt || stored.Stats != original.Stats {
		t.Fatalf("model failure mutated Pet = %#v", stored)
	}
	results, err := runtime.ListGameResults(ctx, "peer-atomic", apitypes.GameplayListRequest{})
	if err != nil || len(results.Items) != 0 {
		t.Fatalf("game results after model failure = %#v, %v", results, err)
	}

	stored.Stats.Life = 1
	stored.Stats.Health = 0
	stored.Stats.Satiety = 0
	stored.Stats.Hygiene = 0
	stored.Stats.Mood = 0
	updatePetForTest(t, runtime, stored)
	*now = now.Add(time.Hour)
	behavior := apitypes.PetBehaviorFeed
	dead, err := runtime.DrivePet(ctx, "peer-atomic", apitypes.PetDriveRequest{PetId: stored.Id, Behavior: &behavior})
	if err != nil {
		t.Fatalf("DrivePet(lethal settlement) error = %v", err)
	}
	if dead.Pet.Lifecycle != apitypes.PetLifecycleDead || dead.Pet.Stats.Life != 0 || dead.Pet.DiedAt == nil {
		t.Fatalf("dead Pet = %#v", dead.Pet)
	}
	diedAt := *dead.Pet.DiedAt
	*now = now.Add(24 * time.Hour)
	terminalTick, err := runtime.DrivePet(ctx, "peer-atomic", apitypes.PetDriveRequest{PetId: stored.Id})
	if err != nil {
		t.Fatalf("DrivePet(empty dead) error = %v", err)
	}
	if terminalTick.Pet.DiedAt == nil || !terminalTick.Pet.DiedAt.Equal(diedAt) || terminalTick.Pet.StateSettledAt != dead.Pet.StateSettledAt || terminalTick.Pet.UpdatedAt != dead.Pet.UpdatedAt {
		t.Fatalf("DrivePet(empty dead) = %#v, want unchanged terminal Pet", terminalTick.Pet)
	}
	if _, err := runtime.DrivePet(ctx, "peer-atomic", apitypes.PetDriveRequest{PetId: stored.Id, Behavior: &behavior}); !errors.Is(err, ErrPetDead) {
		t.Fatalf("DrivePet(dead) error = %v", err)
	}
	terminal, err := runtime.GetPet(ctx, "peer-atomic", stored.Id)
	if err != nil || terminal.DiedAt == nil || !terminal.DiedAt.Equal(diedAt) {
		t.Fatalf("terminal Pet = %#v, %v; want died_at %s", terminal, err, diedAt)
	}
}

func TestGameIdempotencyKeyCannotCrossPets(t *testing.T) {
	ctx, runtime, _ := newPetRuntime(t)
	first, err := runtime.AdoptPet(ctx, "peer-game-idempotency", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet(first) error = %v", err)
	}
	second, err := runtime.AdoptPet(ctx, "peer-game-idempotency", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet(second) error = %v", err)
	}
	ctx = WithRewardEvaluator(ctx, rewardEvaluatorFunc(func(context.Context, RewardEvaluationRequest) (apitypes.GameRewardSpec, error) {
		return apitypes.GameRewardSpec{Reason: "validated"}, nil
	}))
	key := "one-game-drive"
	request := func(petID string) apitypes.PetDriveRequest {
		return apitypes.PetDriveRequest{PetId: petID, GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId: "game-basic", IdempotencyKey: &key,
		}}
	}
	if _, err := runtime.DrivePet(ctx, "peer-game-idempotency", request(first.Pet.Id)); err != nil {
		t.Fatalf("DrivePet(first) error = %v", err)
	}
	if _, err := runtime.DrivePet(ctx, "peer-game-idempotency", request(second.Pet.Id)); err == nil || !strings.Contains(err.Error(), "another game Drive") {
		t.Fatalf("DrivePet(second) error = %v, want cross-Pet idempotency rejection", err)
	}
}

func TestGameRewardDropsZeroBadgeDeltaBeforeIdempotentReplay(t *testing.T) {
	ctx, runtime, _ := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-zero-badge", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	ctx = WithRewardEvaluator(ctx, rewardEvaluatorFunc(func(context.Context, RewardEvaluationRequest) (apitypes.GameRewardSpec, error) {
		return apitypes.GameRewardSpec{BadgeExpDelta: map[string]int64{"basic": 0}, Reason: "no badge progress"}, nil
	}))
	key := "zero-badge-result"
	request := apitypes.PetDriveRequest{PetId: adopted.Pet.Id, GameResult: &apitypes.PetDriveGameResultInput{
		GameDefId: "game-basic", IdempotencyKey: &key,
	}}
	first, err := runtime.DrivePet(ctx, "peer-zero-badge", request)
	if err != nil {
		t.Fatalf("DrivePet(first) error = %v", err)
	}
	second, err := runtime.DrivePet(ctx, "peer-zero-badge", request)
	if err != nil {
		t.Fatalf("DrivePet(replay) error = %v", err)
	}
	if len(first.Badges) != 0 || len(second.Badges) != 0 || len(first.RewardGrants) != 1 || len(first.RewardGrants[0].BadgeExpDelta) != 0 {
		t.Fatalf("zero-delta responses = first %#v, replay %#v", first, second)
	}
}

func TestRewardEvaluationIncludesConfiguredGameDefinition(t *testing.T) {
	ctx, runtime, _ := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-reward-context", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	ctx = WithRewardEvaluator(ctx, rewardEvaluatorFunc(func(_ context.Context, request RewardEvaluationRequest) (apitypes.GameRewardSpec, error) {
		if request.GameDef.Id != "game-basic" || request.GameDef.Spec.DisplayName != "Puzzle" {
			t.Fatalf("GameDef = %#v, want configured game-basic definition", request.GameDef)
		}
		return apitypes.GameRewardSpec{Reason: "validated"}, nil
	}))
	_, err = runtime.DrivePet(ctx, "peer-reward-context", apitypes.PetDriveRequest{
		PetId:      adopted.Pet.Id,
		GameResult: &apitypes.PetDriveGameResultInput{GameDefId: "game-basic"},
	})
	if err != nil {
		t.Fatalf("DrivePet() error = %v", err)
	}
}

func TestValidateGameRewardRequiresReasonAndBounds(t *testing.T) {
	rule := ProfileGameRule{Policy: apitypes.RuntimeProfileGameSpec{Reward: apitypes.RuntimeProfileGameRewardSpec{
		PetExpMax: 10, BadgeExpMaxPerBadge: 5,
	}}}
	badges := map[string]string{"basic": "badge-basic"}
	for name, reward := range map[string]apitypes.GameRewardSpec{
		"missing reason":      {PetExpDelta: 1},
		"negative experience": {Reason: "bad", PetExpDelta: -1},
		"excess experience":   {Reason: "bad", PetExpDelta: 11},
		"unknown badge":       {Reason: "bad", BadgeExpDelta: map[string]int64{"unknown": 1}},
		"excess badge":        {Reason: "bad", BadgeExpDelta: map[string]int64{"basic": 6}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateGameReward(reward, rule, badges); err == nil {
				t.Fatalf("validateGameReward(%#v) succeeded", reward)
			}
		})
	}
}

func newPetRuntime(t *testing.T) (context.Context, *Runtime, *time.Time) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	ctx = WithRuntimeProfile(ctx, profile)
	ids := 0
	runtime := &Runtime{
		DB: testDB(t), Catalog: catalog, Workflows: petWorkflowService{},
		Workspaces: &recordingWorkspaceService{}, PickWeight: func(int64) int64 { return 0 },
		Now: func() time.Time { return now },
		NewID: func() string {
			ids++
			return "pet-test-id-" + fmt.Sprint(ids)
		},
	}
	return ctx, runtime, &now
}

func updatePetForTest(t *testing.T, runtime *Runtime, pet apitypes.Pet) {
	t.Helper()
	tx, err := runtime.DB.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx() error = %v", err)
	}
	defer tx.Rollback()
	if err := updatePet(context.Background(), tx, pet); err != nil {
		t.Fatalf("updatePet() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
}

func assertPetStatsClose(t *testing.T, got, want apitypes.PetStats) {
	t.Helper()
	values := [][3]any{
		{"life", got.Life, want.Life}, {"health", got.Health, want.Health},
		{"satiety", got.Satiety, want.Satiety}, {"hygiene", got.Hygiene, want.Hygiene},
		{"mood", got.Mood, want.Mood}, {"energy", got.Energy, want.Energy},
	}
	for _, value := range values {
		if math.Abs(value[1].(float64)-value[2].(float64)) > 1e-9 {
			t.Errorf("%s = %.12f, want %.12f", value[0], value[1], value[2])
		}
	}
}
