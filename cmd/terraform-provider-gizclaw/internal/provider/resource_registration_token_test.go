package provider

import (
	"encoding/json"
	"testing"
)

func TestRegistrationTokenLifecycleRefresh(t *testing.T) {
	for _, test := range []struct {
		name, configured, observed string
		consistent                 bool
	}{
		{"default enabled", `{}`, `{"enabled":true}`, true},
		{"null defaults", `{"enabled":null,"expires_at":null,"max_activations":null}`, `{"enabled":true}`, true},
		{"disabled drift", `{}`, `{"enabled":false}`, false},
		{"limit drift", `{"max_activations":2}`, `{"max_activations":1,"enabled":true}`, false},
		{"expiry drift", `{"expires_at":null}`, `{"expires_at":"2035-01-01T00:00:00Z"}`, false},
		{"explicit disabled", `{"enabled":false,"max_activations":0}`, `{"enabled":false,"max_activations":0}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := jsonObservedConsistent("RegistrationToken", test.configured, json.RawMessage(test.observed)); got != test.consistent {
				t.Fatalf("consistent = %v, want %v", got, test.consistent)
			}
		})
	}
}
