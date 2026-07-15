package main

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func testKeyString(t *testing.T, input [32]byte) string {
	t.Helper()
	kp, err := giznet.NewKeyPair(giznet.Key(input))
	if err != nil {
		t.Fatal(err)
	}
	return kp.Private.String()
}
