//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryvolc "github.com/GizClaw/gizclaw-go/pkgs/store/memory/volc"
)

func TestLoCoMoVolcAgentKitProtocolSmoke(t *testing.T) {
	config, _ := requireVolcConfig(t)
	store, err := memoryvolc.Open(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	scope := memorystore.Scope{
		AppID:  "locomo-volc-smoke-app-" + stamp,
		UserID: "locomo-volc-smoke-user-" + stamp,
		RunID:  "locomo-volc-smoke-run-" + stamp,
	}
	text := "The protocol smoke marker is cobalt-" + strings.ReplaceAll(stamp, ".", "-") + "."
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := store.Observe(ctx, memorystore.Observation{
		Scope: scope, ID: "observation-" + stamp,
		Turns:      []memorystore.Turn{{ID: "turn-" + stamp, Role: memorystore.RoleUser, Speaker: "smoke-user", Text: text, ObservedAt: time.Now().UTC()}},
		ObservedAt: time.Now().UTC(),
	})
	if err == nil {
		result, err = awaitObservation(ctx, store, scope, result)
	}
	if err != nil {
		t.Fatal(err)
	}
	deletions := map[string]struct{}{}
	for _, fact := range result.Facts {
		deletions[fact.ID] = struct{}{}
	}
	cleanup := func() {
		for id := range deletions {
			deleteCtx, deleteCancel := context.WithTimeout(context.Background(), time.Minute)
			err := store.Delete(deleteCtx, memorystore.DeleteRequest{Scope: scope, ID: id})
			deleteCancel()
			if err != nil {
				t.Errorf("delete focused smoke fact: %v", err)
			}
		}
	}
	t.Cleanup(cleanup)
	recalled, err := store.Recall(ctx, memorystore.Query{Scope: scope, Text: text, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, match := range recalled.Matches {
		deletions[match.Fact.ID] = struct{}{}
		if strings.Contains(match.Fact.Text, "cobalt-") {
			found = true
		}
	}
	if !found {
		t.Fatal("focused Volc smoke did not recall the observed fact")
	}
	leakScope := scope
	leakScope.UserID += "-other"
	leaked, err := store.Recall(ctx, memorystore.Query{Scope: leakScope, Text: text, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range leaked.Matches {
		if strings.Contains(match.Fact.Text, "cobalt-") {
			t.Fatal("focused Volc smoke leaked the observed fact across scopes")
		}
	}
	if len(deletions) == 0 {
		t.Fatal("focused Volc smoke returned no deletable facts")
	}
}
