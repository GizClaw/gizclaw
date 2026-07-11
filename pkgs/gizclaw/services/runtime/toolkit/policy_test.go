package toolkit

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestNormalizePolicyValidatesTrimsAndDeduplicatesToolIDs(t *testing.T) {
	ids := []string{" system.mode.switch ", "system.music.play", "system.music.play"}
	policy, err := NormalizePolicy(&apitypes.ToolkitPolicy{ToolIds: &ids})
	if err != nil {
		t.Fatalf("NormalizePolicy() error = %v", err)
	}
	if policy == nil || policy.ToolIds == nil {
		t.Fatalf("NormalizePolicy() = %#v", policy)
	}
	got := *policy.ToolIds
	if len(got) != 2 || got[0] != "system.mode.switch" || got[1] != "system.music.play" {
		t.Fatalf("ToolIds = %#v", got)
	}
	ids[0] = "mutated"
	if (*policy.ToolIds)[0] != "system.mode.switch" {
		t.Fatalf("normalized policy aliases input slice: %#v", *policy.ToolIds)
	}
}

func TestNormalizePolicyRejectsEmptyToolID(t *testing.T) {
	ids := []string{"system.music.play", " "}
	_, err := NormalizePolicy(&apitypes.ToolkitPolicy{ToolIds: &ids})
	if !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("NormalizePolicy() error = %v, want %v", err, ErrInvalidTool)
	}
}

func TestEchoExecutorReturnsCallArgs(t *testing.T) {
	result, err := (EchoExecutor{}).Invoke(context.Background(), Call{Args: []byte(`{"text":"hello"}`)})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if string(result.Data) != `{"text":"hello"}` {
		t.Fatalf("Data = %s", result.Data)
	}
}
