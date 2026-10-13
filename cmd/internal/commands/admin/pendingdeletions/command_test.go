package pendingdeletionscmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func recoveryTask(code string) apitypes.PendingDeletionTask {
	return apitypes.PendingDeletionTask{Source: "peer", Kind: apitypes.PendingDeletionKindPeer, DeletionId: uuid.New(), ResourceId: "fixture-peer", Status: apitypes.PendingDeletionStatusFailed, Phase: "finalize", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), LastErrorCode: &code}
}

func testCommand(t *testing.T, handler http.HandlerFunc) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	api, err := adminhttp.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cmd := newCmd(func(context.Context, string) (*adminhttp.ClientWithResponses, func(), error) {
		return api, func() {}, nil
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	return cmd, &out
}

func TestRecoveryPreviewIsReadOnlyAndCountsOnlyBoundedSelection(t *testing.T) {
	var requests atomic.Int64
	cmd, output := testCommand(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/pending-deletions" || r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("status") != "failed" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.PendingDeletionList{Items: []apitypes.PendingDeletionTask{recoveryTask("peer_replacement_ambiguous"), recoveryTask("peer_record_missing")}, NextCursor: new("next-page")})
	})
	cmd.SetArgs([]string{"recover", "--source", "peer", "--limit", "2", "--error-code", "peer_replacement_ambiguous"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || strings.Contains(output.String(), "peer_record_missing") || !strings.Contains(output.String(), `"selected":1`) || !strings.Contains(output.String(), `"next_cursor":"next-page"`) {
		t.Fatalf("preview: %s", output)
	}
}

func TestRecoveryReportsAdmissionRejectionAndCompletionSeparately(t *testing.T) {
	good, blocked := recoveryTask("peer_replacement_ambiguous"), recoveryTask("peer_record_missing")
	var queued atomic.Bool
	cmd, output := testCommand(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/pending-deletions":
			_ = json.NewEncoder(w).Encode(apitypes.PendingDeletionList{Items: []apitypes.PendingDeletionTask{good, blocked}})
		case strings.Contains(r.URL.Path, good.DeletionId.String()):
			if r.Method == http.MethodPost {
				queued.Store(true)
				task := good
				task.Status = apitypes.PendingDeletionStatusQueued
				_ = json.NewEncoder(w).Encode(task)
			} else if queued.Load() {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(apitypes.NewErrorResponse("PENDING_DELETION_NOT_FOUND", "gone"))
			} else {
				_ = json.NewEncoder(w).Encode(good)
			}
		case strings.Contains(r.URL.Path, blocked.DeletionId.String()):
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(apitypes.NewErrorResponse("peer_record_missing", "Retain the fence"))
			} else {
				_ = json.NewEncoder(w).Encode(blocked)
			}
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})
	cmd.SetArgs([]string{"recover", "--source", "peer", "--execute", "--interval", "100ms", "--wait", "2s"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("partial recovery returned success")
	}
	for _, expected := range []string{`"outcome":"queued"`, `"outcome":"rejected"`, `"outcome":"completed"`, `"completed":1`, `"rejected":1`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %s: %s", expected, output)
		}
	}
}

func TestRecoveryCancellationStopsNewRetriesAndLeavesProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first, second := recoveryTask("transient"), recoveryTask("transient")
	var posts atomic.Int64
	cmd, output := testCommand(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/pending-deletions" {
			_ = json.NewEncoder(w).Encode(apitypes.PendingDeletionList{Items: []apitypes.PendingDeletionTask{first, second}})
			return
		}
		if r.Method == http.MethodPost {
			posts.Add(1)
			_ = json.NewEncoder(w).Encode(first)
			return
		}
		if strings.Contains(r.URL.Path, first.DeletionId.String()) {
			_ = json.NewEncoder(w).Encode(first)
			return
		}
		cancel()
		_ = json.NewEncoder(w).Encode(second)
	})
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"recover", "--source", "peer", "--execute", "--interval", "100ms"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("canceled run succeeded")
	}
	if posts.Load() != 1 || !strings.Contains(output.String(), `"type":"retry"`) {
		t.Fatalf("posts=%d output=%s", posts.Load(), output)
	}
}

func TestRecoveryValidatesBoundsBeforeConnecting(t *testing.T) {
	for _, args := range [][]string{{}, {"--source", "peer", "--limit", "501"}, {"--source", "peer", "--interval", "0s"}, {"--source", "peer", "--wait", "1s"}, {"--source", "peer", "--from", "invalid"}, {"--source", "peer", "--kind", "invalid"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			cmd := newCmd(func(context.Context, string) (*adminhttp.ClientWithResponses, func(), error) {
				t.Fatal("connected with invalid bounds")
				return nil, nil, nil
			})
			cmd.SetArgs(append([]string{"recover"}, args...))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			if err := cmd.Execute(); err == nil {
				t.Fatal("accepted invalid bounds")
			}
		})
	}
}

func TestRecoveryDoesNotBurstAfterSlowResponse(t *testing.T) {
	first, second := recoveryTask("transient"), recoveryTask("transient")
	var lastRequest time.Time
	cmd, _ := testCommand(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/pending-deletions" {
			_ = json.NewEncoder(w).Encode(apitypes.PendingDeletionList{Items: []apitypes.PendingDeletionTask{first, second}})
			return
		}
		now := time.Now()
		if !lastRequest.IsZero() && now.Sub(lastRequest) < 95*time.Millisecond {
			t.Errorf("recovery requests burst after slow response: %s", now.Sub(lastRequest))
		}
		lastRequest = now
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, first.DeletionId.String()) {
			time.Sleep(175 * time.Millisecond)
		}
		task := first
		if strings.Contains(r.URL.Path, second.DeletionId.String()) {
			task = second
		}
		_ = json.NewEncoder(w).Encode(task)
	})
	cmd.SetArgs([]string{"recover", "--source", "peer", "--execute", "--interval", "100ms"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
