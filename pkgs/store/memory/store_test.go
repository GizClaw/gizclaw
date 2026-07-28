package memory

import (
	"errors"
	"testing"
	"time"
)

func TestRequestValidation(t *testing.T) {
	t.Parallel()
	if err := validateObservation(Observation{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("validateObservation() error = %v, want ErrInvalidInput", err)
	}
	if err := validateObservation(Observation{Turns: []Turn{{Role: RoleUser, Text: "remember", Attributes: map[string]any{"channel": "voice"}}}}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("validateObservation() turn attributes error = %v, want ErrUnsupported", err)
	}
	if err := validateObservation(Observation{Facts: []FactCandidate{{Text: "remember", Attributes: map[string]any{"kind": "fact"}}}}); err != nil {
		t.Fatalf("validateObservation() fact candidate error = %v", err)
	}
	if err := validateObservation(Observation{Facts: []FactCandidate{{Text: " "}}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("validateObservation() empty fact candidate error = %v, want ErrInvalidInput", err)
	}
	if err := validateQuery(Query{Text: "where", Limit: 0}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("validateQuery() error = %v, want ErrInvalidInput", err)
	}
	if err := validateFilter(Filter{Field: "kind", Operator: FilterIn, Value: []string{}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("validateFilter() error = %v, want ErrInvalidInput", err)
	}
	text := "updated"
	if err := validateUpdate(UpdateRequest{ID: "fact", Text: &text, Attributes: AttributePatch{Set: map[string]any{"lane": "clues"}, Delete: []string{"lane"}}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("validateUpdate() error = %v, want ErrInvalidInput", err)
	}
}

func TestObservationPayloadDigestIsStableAndSensitive(t *testing.T) {
	observation := Observation{
		Scope: Scope{AppID: "workspace-a"}, ID: "observation-a",
		Facts:      []FactCandidate{{Text: "fact", Attributes: map[string]any{"b": 2, "a": 1}}},
		ObservedAt: time.Date(2026, 7, 28, 1, 2, 3, 4, time.UTC),
	}
	first, err := ObservationPayloadDigest(observation)
	if err != nil {
		t.Fatal(err)
	}
	reordered := observation
	reordered.Scope = Scope{AppID: "workspace-b"}
	reordered.ID = "observation-b"
	reordered.Facts = []FactCandidate{{Text: "fact", Attributes: map[string]any{"a": 1, "b": 2}}}
	second, err := ObservationPayloadDigest(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("digest includes Scope/ID or map order: %q != %q", first, second)
	}
	reordered.Facts[0].Text = "changed"
	changed, err := ObservationPayloadDigest(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("digest did not change with payload")
	}
}

func TestCommonValidationDoesNotInventScopeHierarchy(t *testing.T) {
	t.Parallel()

	for name, scope := range map[string]Scope{
		"empty":      {},
		"app only":   {AppID: "app"},
		"user only":  {UserID: "user"},
		"agent only": {AgentID: "agent"},
		"run only":   {RunID: "run"},
		"all":        {AppID: "app", UserID: "user", AgentID: "agent", RunID: "run"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateObservation(Observation{Scope: scope, Text: "remember"}); err != nil {
				t.Fatalf("ValidateObservation() error = %v", err)
			}
			if err := ValidateQuery(Query{Scope: scope, Text: "remember", Limit: 1}); err != nil {
				t.Fatalf("ValidateQuery() error = %v", err)
			}
		})
	}
}

func TestCloneFactOwnsNestedValues(t *testing.T) {
	t.Parallel()
	original := Fact{Attributes: map[string]any{"nested": map[string]any{"values": []any{"a"}}, "typed": []map[string]any{{"value": "a"}}}, Sources: []SourceRef{{TurnIDs: []string{"turn"}}}}
	cloned := cloneFact(original)
	cloned.Attributes["nested"].(map[string]any)["values"].([]any)[0] = "changed"
	cloned.Sources[0].TurnIDs[0] = "changed"
	cloned.Attributes["typed"].([]map[string]any)[0]["value"] = "changed"
	if got := original.Attributes["nested"].(map[string]any)["values"].([]any)[0]; got != "a" {
		t.Fatalf("original nested value = %v, want a", got)
	}
	if got := original.Sources[0].TurnIDs[0]; got != "turn" {
		t.Fatalf("original source turn = %q, want turn", got)
	}
	if got := original.Attributes["typed"].([]map[string]any)[0]["value"]; got != "a" {
		t.Fatalf("original typed value = %v, want a", got)
	}
}

func TestFilterOperatorsValidate(t *testing.T) {
	t.Parallel()
	valid := []Filter{
		{Field: "x", Operator: FilterEqual, Value: 1}, {Field: "x", Operator: FilterNotEqual, Value: 1},
		{Field: "x", Operator: FilterIn, Value: []string{"a"}}, {Field: "x", Operator: FilterNotIn, Value: [1]string{"a"}},
		{Field: "x", Operator: FilterExists, Value: true}, {Field: "x", Operator: FilterGreaterThan, Value: 1},
		{Field: "x", Operator: FilterGreaterEqual, Value: 1}, {Field: "x", Operator: FilterLessThan, Value: 1},
		{Field: "x", Operator: FilterLessEqual, Value: 1},
	}
	for _, filter := range valid {
		if err := validateFilter(filter); err != nil {
			t.Fatalf("validateFilter(%+v) = %v", filter, err)
		}
	}
	for _, filter := range []Filter{{Operator: FilterEqual, Value: 1}, {Field: "x", Operator: FilterExists, Value: "yes"}, {Field: "x", Operator: "unknown", Value: 1}} {
		if err := validateFilter(filter); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("validateFilter(%+v) = %v", filter, err)
		}
	}
}
