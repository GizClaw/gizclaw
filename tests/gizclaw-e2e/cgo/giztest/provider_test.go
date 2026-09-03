package main

import (
	"maps"
	"slices"
	"testing"
)

// delay_ms is a runner instruction, not a payload field: it must be removed
// before the rest is encoded, and it must never silently become an immediate
// answer, which would let a timeout scenario pass without reaching that path.
func TestScriptedDelay(t *testing.T) {
	tests := []struct {
		name      string
		response  any
		wantValue any
		wantMs    int64
		wantErr   bool
	}{
		{name: "not an object", response: "text", wantValue: "text"},
		{name: "absent", response: map[string]any{"networks": []any{}}, wantValue: map[string]any{"networks": []any{}}},
		{name: "removed before encoding", response: map[string]any{"delay_ms": 1200, "networks": []any{}}, wantValue: map[string]any{"networks": []any{}}, wantMs: 1200},
		{name: "at the bound", response: map[string]any{"delay_ms": maxScriptedDelayMs}, wantValue: map[string]any{}, wantMs: maxScriptedDelayMs},
		{name: "above the bound", response: map[string]any{"delay_ms": int64(maxScriptedDelayMs) + 1}, wantErr: true},
		{name: "negative", response: map[string]any{"delay_ms": -1}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, delay, err := scriptedDelay(test.response)
			if test.wantErr {
				if err == nil {
					t.Fatalf("value=%#v delay=%v, want an error", value, delay)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if delay.Milliseconds() != test.wantMs {
				t.Fatalf("delay = %v, want %dms", delay, test.wantMs)
			}
			want, wantObject := test.wantValue.(map[string]any)
			got, gotObject := value.(map[string]any)
			if wantObject != gotObject {
				t.Fatalf("value = %#v, want %#v", value, test.wantValue)
			}
			if !wantObject {
				if value != test.wantValue {
					t.Fatalf("value = %#v, want %#v", value, test.wantValue)
				}
				return
			}
			if _, ok := got["delay_ms"]; ok {
				t.Fatalf("delay_ms survived into the encoded payload: %#v", got)
			}
			if !slices.Equal(slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want))) {
				t.Fatalf("value keys = %#v, want %#v", got, want)
			}
		})
	}
}
