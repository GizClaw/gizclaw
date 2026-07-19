package voice

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
)

func TestNewTransformerReturnsGenXTransformer(t *testing.T) {
	var got genx.TransformerMux = NewTransformer(peergenx.Service{})
	if got == nil {
		t.Fatal("NewTransformer() = nil")
	}
}
