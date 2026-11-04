//go:build gizclaw_e2e

package internal

import (
	"strings"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestIsolatedWorkspaceName(t *testing.T) {
	if got := isolatedWorkspaceName("workflow", "chosen"); got != "chosen" {
		t.Fatalf("explicit workspace name = %q", got)
	}
	if got := isolatedWorkspaceName("workflow", ""); !strings.HasPrefix(got, "workflow-ptt-") {
		t.Fatalf("generated workspace name = %q", got)
	}
}

func TestIsCSDKNotFound(t *testing.T) {
	if !isCSDKNotFound(&RPCStatus{Code: rpcpb.StatusCode_STATUS_CODE_NOT_FOUND}) {
		t.Fatal("C SDK not-found error was not recognized")
	}
	if isCSDKNotFound(&RPCStatus{Code: rpcpb.StatusCode_STATUS_CODE_INTERNAL}) {
		t.Fatal("C SDK internal error was recognized as not-found")
	}
}
