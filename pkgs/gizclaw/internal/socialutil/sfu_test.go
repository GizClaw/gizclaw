package socialutil

import (
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
}
