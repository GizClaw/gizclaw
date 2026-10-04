package memorystore

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/tests/testsupport/mem0fixture"
)

func testMem0Server(t testing.TB) string { return mem0fixture.NewServer(t) }
