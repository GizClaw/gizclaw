package flowcraft

import (
	"context"
	"testing"
)

func TestSafetyFenceIsOnlyABoardVariable(t *testing.T) {
	product := func(context.Context) (map[string]any, error) {
		return map[string]any{"device": "h106", SafetyFenceBoardVariable: "product value"}, nil
	}
	for _, test := range []struct {
		provider InputProvider
		fence    string
		want     map[string]any
	}{
		{nil, "", map[string]any{SafetyFenceBoardVariable: ""}},
		{nil, "profile fence", map[string]any{SafetyFenceBoardVariable: "profile fence"}},
		// The RuntimeProfile fence wins over a product input with the same name.
		{product, "profile fence", map[string]any{"device": "h106", SafetyFenceBoardVariable: "profile fence"}},
	} {
		got, err := genxflowcraftBoardInputs(test.provider, test.fence)(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(test.want) {
			t.Fatalf("inputs = %#v; want %#v", got, test.want)
		}
		for key, value := range test.want {
			if got[key] != value {
				t.Fatalf("inputs[%q] = %#v; want %#v", key, got[key], value)
			}
		}
	}
}
