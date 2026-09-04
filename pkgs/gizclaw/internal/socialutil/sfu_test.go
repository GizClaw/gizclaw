package socialutil

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewRoomTokenIsRandomAndOpaque(t *testing.T) {
	t.Parallel()
	first, err := NewRoomToken()
	if err != nil {
		t.Fatalf("NewRoomToken() error = %v", err)
	}
	second, err := NewRoomToken()
	if err != nil {
		t.Fatalf("NewRoomToken() error = %v", err)
	}
	if first == second || !strings.HasPrefix(first, "room-") || len(first) != len("room-")+32 {
		t.Fatalf("NewRoomToken() = %q, %q, want distinct opaque tokens", first, second)
	}
}

func TestSFUBindingValidateRequiresIdentity(t *testing.T) {
	t.Parallel()
	if err := (SFUBinding{URL: "wss://sfu", RoomToken: "room-a", Generation: 1}).Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}
	if err := (SFUBinding{RoomToken: "room-a"}).Validate(); err == nil {
		t.Fatal("Validate(missing url) error = nil")
	}
	if err := (SFUBinding{URL: "wss://sfu"}).Validate(); err == nil {
		t.Fatal("Validate(missing room_token) error = nil")
	}
	if err := (SFUBinding{URL: "wss://sfu", RoomToken: "room-a"}).Validate(); err == nil {
		t.Fatal("Validate(zero generation) error = nil")
	}
}

// A record written before the generation field existed, or a truncated one,
// decodes with Generation 0 and must fail closed instead of attaching.
func TestSFUBindingValidateRejectsDecodedRecordWithoutGeneration(t *testing.T) {
	t.Parallel()
	var binding SFUBinding
	if err := json.Unmarshal([]byte(`{"url":"wss://sfu","room_token":"room-a"}`), &binding); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if err := binding.Validate(); err == nil || !strings.Contains(err.Error(), "generation") {
		t.Fatalf("Validate(short record) error = %v, want a generation failure", err)
	}
}
