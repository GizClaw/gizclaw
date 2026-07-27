package memory

import (
	"context"
	"errors"
	"testing"
)

func TestBindAppRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	if _, err := BindApp(nil, "workspace"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("BindApp(nil) error = %v, want ErrInvalidInput", err)
	}
	var typedNil *scopeTestStore
	if _, err := BindApp(typedNil, "workspace"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("BindApp(typed nil) error = %v, want ErrInvalidInput", err)
	}
	if _, err := BindApp(&scopeTestStore{}, " \t "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("BindApp(blank app id) error = %v, want ErrInvalidInput", err)
	}
}

func TestBindAppForcesOnlyAppID(t *testing.T) {
	t.Parallel()

	backend := &scopeTestStore{}
	store, err := BindApp(backend, " workspace ")
	if err != nil {
		t.Fatalf("BindApp() error = %v", err)
	}
	inner := Scope{UserID: " user ", AgentID: " agent ", RunID: " run "}
	if _, err := store.Observe(context.Background(), Observation{Scope: inner, Text: "remember"}); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	want := inner
	want.AppID = "workspace"
	if backend.observation.Scope != want {
		t.Fatalf("Observe() scope = %#v, want %#v", backend.observation.Scope, want)
	}

	queryScope := inner
	queryScope.AppID = "workspace"
	if _, err := store.Recall(context.Background(), Query{Scope: queryScope, Text: "remember", Limit: 1}); err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if backend.query.Scope != want {
		t.Fatalf("Recall() scope = %#v, want %#v", backend.query.Scope, want)
	}
}

func TestBindAppRejectsConflictsForEveryScopedOperation(t *testing.T) {
	t.Parallel()

	backend := &scopeAllTestStore{scopeTestStore: &scopeTestStore{}}
	store, err := BindApp(backend, "workspace-a")
	if err != nil {
		t.Fatalf("BindApp() error = %v", err)
	}
	conflict := Scope{AppID: "workspace-b", UserID: "user", AgentID: "agent", RunID: "run"}
	text := "updated"
	calls := []struct {
		name string
		call func() error
	}{
		{name: "observe", call: func() error {
			_, err := store.Observe(context.Background(), Observation{Scope: conflict, Text: "remember"})
			return err
		}},
		{name: "recall", call: func() error {
			_, err := store.Recall(context.Background(), Query{Scope: conflict, Text: "remember", Limit: 1})
			return err
		}},
		{name: "update", call: func() error {
			_, err := store.Update(context.Background(), UpdateRequest{Scope: conflict, ID: "fact", Text: &text})
			return err
		}},
		{name: "delete", call: func() error {
			return store.Delete(context.Background(), DeleteRequest{Scope: conflict, ID: "fact"})
		}},
		{name: "wait", call: func() error {
			_, err := store.(OperationWaiter).Wait(context.Background(), OperationRequest{Scope: conflict, ID: "operation"})
			return err
		}},
		{name: "process async", call: func() error {
			_, err := store.(AsyncOperationProcessor).ProcessAsync(context.Background(), OperationRequest{Scope: conflict, ID: "operation"})
			return err
		}},
		{name: "stats", call: func() error {
			_, err := store.(StatisticsProvider).Stats(context.Background(), conflict)
			return err
		}},
	}
	for _, test := range calls {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.call(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestBindAppPreservesOptionalCapabilitiesExactly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		store          Store
		wantWaiter     bool
		wantProcessor  bool
		wantStatistics bool
	}{
		{name: "none", store: &scopeTestStore{}},
		{name: "waiter", store: &scopeWaiterTestStore{scopeTestStore: &scopeTestStore{}}, wantWaiter: true},
		{name: "processor", store: &scopeProcessorTestStore{scopeTestStore: &scopeTestStore{}}, wantProcessor: true},
		{name: "statistics", store: &scopeStatisticsTestStore{scopeTestStore: &scopeTestStore{}}, wantStatistics: true},
		{name: "waiter processor", store: &scopeWaiterProcessorTestStore{scopeTestStore: &scopeTestStore{}}, wantWaiter: true, wantProcessor: true},
		{name: "waiter statistics", store: &scopeWaiterStatisticsTestStore{scopeTestStore: &scopeTestStore{}}, wantWaiter: true, wantStatistics: true},
		{name: "processor statistics", store: &scopeProcessorStatisticsTestStore{scopeTestStore: &scopeTestStore{}}, wantProcessor: true, wantStatistics: true},
		{name: "all", store: &scopeAllTestStore{scopeTestStore: &scopeTestStore{}}, wantWaiter: true, wantProcessor: true, wantStatistics: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store, err := BindApp(test.store, "workspace")
			if err != nil {
				t.Fatalf("BindApp() error = %v", err)
			}
			if _, ok := store.(OperationWaiter); ok != test.wantWaiter {
				t.Fatalf("OperationWaiter assertion = %v, want %v", ok, test.wantWaiter)
			}
			if _, ok := store.(AsyncOperationProcessor); ok != test.wantProcessor {
				t.Fatalf("AsyncOperationProcessor assertion = %v, want %v", ok, test.wantProcessor)
			}
			if _, ok := store.(StatisticsProvider); ok != test.wantStatistics {
				t.Fatalf("StatisticsProvider assertion = %v, want %v", ok, test.wantStatistics)
			}
		})
	}
}

func TestBindAppPreservesDirectFactCapabilityDiscovery(t *testing.T) {
	t.Parallel()
	plain, err := BindApp(&scopeTestStore{}, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if SupportsDirectFactObservation(plain) {
		t.Fatal("plain Store unexpectedly supports direct Facts")
	}
	direct, err := BindApp(&scopeDirectFactTestStore{scopeTestStore: &scopeTestStore{}}, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if !SupportsDirectFactObservation(direct) {
		t.Fatal("BindApp hid direct Fact capability")
	}
}

func TestBindAppDefensivelyCopiesMutableValues(t *testing.T) {
	t.Parallel()

	backend := &scopeTestStore{mutate: true}
	store, err := BindApp(backend, "workspace")
	if err != nil {
		t.Fatalf("BindApp() error = %v", err)
	}
	attributes := map[string]any{"nested": map[string]any{"value": "original"}}
	observation := Observation{
		Text:    "remember",
		Context: attributes,
		Turns:   []Turn{{Role: RoleUser, Text: "turn", Attributes: map[string]any{"value": "original"}}},
		Facts:   []FactCandidate{{Text: "fact", Attributes: map[string]any{"value": "original"}}},
	}
	result, err := store.Observe(context.Background(), observation)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if got := attributes["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("caller context value = %v, want original", got)
	}
	result.Facts[0].Attributes["value"] = "caller mutation"
	if got := backend.resultFact.Attributes["value"]; got != "backend" {
		t.Fatalf("backend result value = %v, want backend", got)
	}
}

type scopeTestStore struct {
	observation Observation
	query       Query
	update      UpdateRequest
	deletion    DeleteRequest
	resultFact  Fact
	mutate      bool
}

func (s *scopeTestStore) Observe(_ context.Context, observation Observation) (ObserveResult, error) {
	s.observation = observation
	if s.mutate {
		observation.Context["nested"].(map[string]any)["value"] = "backend mutation"
		observation.Turns[0].Attributes["value"] = "backend mutation"
		observation.Facts[0].Attributes["value"] = "backend mutation"
	}
	s.resultFact = Fact{ID: "fact", Attributes: map[string]any{"value": "backend"}}
	return ObserveResult{Facts: []Fact{s.resultFact}}, nil
}

func (s *scopeTestStore) Recall(_ context.Context, query Query) (RecallResult, error) {
	s.query = query
	return RecallResult{}, nil
}

func (s *scopeTestStore) Update(_ context.Context, request UpdateRequest) (Fact, error) {
	s.update = request
	return Fact{ID: request.ID}, nil
}

func (s *scopeTestStore) Delete(_ context.Context, request DeleteRequest) error {
	s.deletion = request
	return nil
}

type scopeWaiterTestStore struct{ *scopeTestStore }

type scopeDirectFactTestStore struct{ *scopeTestStore }

func (*scopeDirectFactTestStore) SupportsDirectFactObservation() bool { return true }

func (*scopeWaiterTestStore) Wait(_ context.Context, _ OperationRequest) (ObserveResult, error) {
	return ObserveResult{}, nil
}

type scopeProcessorTestStore struct{ *scopeTestStore }

func (*scopeProcessorTestStore) ProcessAsync(_ context.Context, _ OperationRequest) (ObserveResult, error) {
	return ObserveResult{}, nil
}

type scopeStatisticsTestStore struct{ *scopeTestStore }

func (*scopeStatisticsTestStore) Stats(_ context.Context, _ Scope) (Statistics, error) {
	return Statistics{}, nil
}

type scopeWaiterProcessorTestStore struct{ *scopeTestStore }

func (*scopeWaiterProcessorTestStore) Wait(_ context.Context, _ OperationRequest) (ObserveResult, error) {
	return ObserveResult{}, nil
}

func (*scopeWaiterProcessorTestStore) ProcessAsync(_ context.Context, _ OperationRequest) (ObserveResult, error) {
	return ObserveResult{}, nil
}

type scopeWaiterStatisticsTestStore struct{ *scopeTestStore }

func (*scopeWaiterStatisticsTestStore) Wait(_ context.Context, _ OperationRequest) (ObserveResult, error) {
	return ObserveResult{}, nil
}

func (*scopeWaiterStatisticsTestStore) Stats(_ context.Context, _ Scope) (Statistics, error) {
	return Statistics{}, nil
}

type scopeProcessorStatisticsTestStore struct{ *scopeTestStore }

func (*scopeProcessorStatisticsTestStore) ProcessAsync(_ context.Context, _ OperationRequest) (ObserveResult, error) {
	return ObserveResult{}, nil
}

func (*scopeProcessorStatisticsTestStore) Stats(_ context.Context, _ Scope) (Statistics, error) {
	return Statistics{}, nil
}

type scopeAllTestStore struct{ *scopeTestStore }

func (*scopeAllTestStore) Wait(_ context.Context, _ OperationRequest) (ObserveResult, error) {
	return ObserveResult{}, nil
}

func (*scopeAllTestStore) ProcessAsync(_ context.Context, _ OperationRequest) (ObserveResult, error) {
	return ObserveResult{}, nil
}

func (*scopeAllTestStore) Stats(_ context.Context, _ Scope) (Statistics, error) {
	return Statistics{}, nil
}
