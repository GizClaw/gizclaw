package genx

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type safeTerminalError struct{}

func (*safeTerminalError) Error() string { return "private endpoint and credential" }
func (*safeTerminalError) PublicError() (string, string, bool) {
	return "LIMIT_REACHED", "Limit reached.", false
}

func TestTerminalErrorPreservesWrappedCauseWithoutSerializingIt(t *testing.T) {
	cause := &safeTerminalError{}
	wrapped := fmt.Errorf("driver: %w", fmt.Errorf("provider: %w", cause))
	chunk := NewTextEndOfStream()
	SetStreamError(chunk.Ctrl, wrapped)
	copy := chunk.Clone()
	if !errors.Is(StreamError(copy.Ctrl), cause) || copy.Ctrl.Error != "Limit reached." || copy.Ctrl.ErrorCode != "LIMIT_REACHED" || copy.Ctrl.ErrorRetryable {
		t.Fatalf("terminal control = %+v", copy.Ctrl)
	}
	encoded, err := json.Marshal(copy.Ctrl)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "Cause") {
		t.Fatalf("serialized private cause: %s", encoded)
	}
	SetStreamError(copy.Ctrl, nil)
	if StreamError(copy.Ctrl) != nil || copy.Ctrl.ErrorCode != "" {
		t.Fatal("cleared error retained details")
	}
	wire := &StreamCtrl{Error: "QUOTA_EXHAUSTED: untrusted text", ErrorCode: "QUOTA_EXHAUSTED"}
	if _, _, _, ok := PublicErrorDetails(StreamError(wire)); ok {
		t.Fatal("classified an untyped wire error")
	}
}
