package main

import (
	"encoding/json"
	"testing"
)

func TestDecodeAnswer(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		raw  string
		want string
	}{
		"string": {raw: `"January, 2023"`, want: "January, 2023"},
		"number": {raw: `2022`, want: "2022"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := decodeAnswer(json.RawMessage(test.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("decodeAnswer() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDecodeAnswerRejectsStructuredValue(t *testing.T) {
	t.Parallel()
	if _, err := decodeAnswer(json.RawMessage(`["answer"]`)); err == nil {
		t.Fatal("decodeAnswer() should reject structured values")
	}
}
