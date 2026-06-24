package model

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/peergenx"
)

func TestNewGeneratorReturnsGenXGenerator(t *testing.T) {
	var got genx.Generator = NewGenerator(peergenx.Service{})
	if got == nil {
		t.Fatal("NewGenerator() = nil")
	}
}
