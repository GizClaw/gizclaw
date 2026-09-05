package peerhttp

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestDebugPublicKeyBearer(t *testing.T) {
	key := giznet.PublicKey{1}
	for _, tc := range []struct {
		value          string
		debug, invalid bool
	}{
		{"Bearer gizclaw_pk_" + key.String(), true, false},
		{"bearer gizclaw_pk_" + key.String(), true, false},
		{"Bearer gizclaw_pk_invalid", true, true},
		{"Bearer gizclaw_pk_", true, true},
		{"Bearer gizclaw_sk_v1_invalid", false, false},
		{"Bearer " + key.String(), false, false},
		{"Basic gizclaw_pk_" + key.String(), false, false},
	} {
		got, debug, err := DebugPublicKey(tc.value)
		if debug != tc.debug || (err != nil) != tc.invalid {
			t.Fatalf("%q: %v %v", tc.value, debug, err)
		}
		if debug && err == nil && got != key {
			t.Fatal("wrong public key")
		}
	}
}
