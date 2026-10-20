package genx

import (
	"encoding/json"
	"testing"
)

func TestModelParamsTemperaturePresence(t *testing.T) {
	var absent *ModelParams
	if absent.HasTemperature() {
		t.Fatal("nil parameters have a temperature")
	}
	for _, test := range []struct {
		name string
		json string
		set  bool
	}{
		{name: "absent", json: `{"max_tokens":64}`},
		{name: "null", json: `{"temperature":null}`},
		{name: "zero", json: `{"temperature":0}`, set: true},
		{name: "positive", json: `{"temperature":0.5}`, set: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var value ModelParams
			if err := json.Unmarshal([]byte(test.json), &value); err != nil {
				t.Fatal(err)
			}
			if value.HasTemperature() != test.set {
				t.Fatalf("temperature presence=%v, want %v", value.HasTemperature(), test.set)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if _, exists := fields["temperature"]; exists != test.set {
				t.Fatalf("temperature JSON presence changed: %s", encoded)
			}
			var restored ModelParams
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.HasTemperature() != test.set || restored.Temperature != value.Temperature {
				t.Fatalf("temperature changed after round trip: %s", encoded)
			}
		})
	}
}

func TestModelParamsTemperaturePatch(t *testing.T) {
	value := ModelParams{MaxTokens: 64}
	value.SetTemperature(0.5)
	for _, patch := range []string{`{"n":2}`, `{"temperature":null}`} {
		if err := json.Unmarshal([]byte(patch), &value); err != nil {
			t.Fatal(err)
		}
		if !value.HasTemperature() || value.Temperature != 0.5 || value.MaxTokens != 64 || value.N != 2 {
			t.Fatalf("partial JSON changed existing parameters: %+v", value)
		}
	}
	if err := json.Unmarshal([]byte(`{"temperature":"invalid","max_tokens":32}`), &value); err == nil {
		t.Fatal("invalid temperature was accepted")
	}
	if value.Temperature != 0.5 || value.MaxTokens != 64 {
		t.Fatal("invalid JSON partially changed parameters")
	}
}

func TestModelParamsZeroTemperatureOverridesDefault(t *testing.T) {
	base := &ModelParams{MaxTokens: 64, Temperature: 0.5}
	explicit := &ModelParams{MaxTokens: 32}
	explicit.SetTemperature(0)
	for _, test := range []struct {
		name     string
		override *ModelParams
		want     float64
	}{
		{name: "unset keeps default", override: &ModelParams{MaxTokens: 32}, want: 0.5},
		{name: "legacy negative keeps default", override: &ModelParams{MaxTokens: 32, Temperature: -1}, want: 0.5},
		{name: "explicit zero replaces default", override: explicit},
	} {
		t.Run(test.name, func(t *testing.T) {
			generator := &OpenAIGenerator{Model: "fixture"}
			context := &ModelContextBuilder{Params: test.override}
			context.UserText("", "hello")
			request, err := generator.chatCompletion(context.Build(), base)
			if err != nil {
				t.Fatal(err)
			}
			if !request.Temperature.Valid() || request.Temperature.Value != test.want {
				t.Fatalf("wire temperature valid=%v value=%v, want %v", request.Temperature.Valid(), request.Temperature.Value, test.want)
			}
			encoded, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["temperature"] != test.want || request.MaxCompletionTokens.Value != 32 {
				t.Fatalf("wire parameters changed: %s", encoded)
			}
		})
	}
	if base.Temperature != 0.5 || base.MaxTokens != 64 || explicit.Temperature != 0 {
		t.Fatal("parameter merge mutated its inputs")
	}
}
