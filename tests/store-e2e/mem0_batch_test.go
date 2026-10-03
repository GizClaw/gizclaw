//go:build store_e2e

package store_e2e_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
)

// TestSelfHostedMem0Batch exercises the real authenticated service and vector
// backend, including a transport that discards the first committed response.
func TestSelfHostedMem0Batch(t *testing.T) {
	endpoint := os.Getenv("GIZCLAW_MEM0_SELF_HOSTED_URL")
	if endpoint == "" {
		t.Skip("run tests/gizclaw-e2e/run_mem0_tests.sh")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	config := mem0.Config{Endpoint: endpoint, APIKey: os.Getenv("GIZCLAW_MEM0_SELF_HOSTED_API_KEY"), Flavor: mem0.SelfHosted}
	store, err := mem0.New(config)
	if err != nil {
		t.Fatal(err)
	}
	scope := memory.Scope{AppID: fmt.Sprintf("batch-%d", time.Now().UnixNano()), UserID: "owner", AgentID: "assistant", RunID: "turn"}
	t.Cleanup(func() {
		if err := store.PurgeScope(context.Background(), scope); err != nil {
			t.Error(err)
		}
	})
	observation := memory.Observation{Scope: scope, ID: "one-observation", ObservedAt: time.Now().UTC(), Facts: []memory.FactCandidate{
		{Text: "User: Marzipan prefers salmon", Attributes: map[string]any{"kind": "user", "nested": map[string]any{"source": "input"}}},
		{Text: "Assistant: I recorded Marzipan's salmon preference", Attributes: map[string]any{"kind": "assistant"}},
	}}
	dropping := &loseFirstMem0Response{}
	lossConfig := config
	lossConfig.HTTPClient = dropping
	writer, err := mem0.New(lossConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Observe(ctx, observation); err == nil {
		t.Fatal("discarded response was reported successful")
	}
	first, err := store.Observe(ctx, observation)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Facts) != 2 {
		t.Fatalf("facts=%d", len(first.Facts))
	}
	for index, fact := range first.Facts {
		if fact.Text != observation.Facts[index].Text || !reflect.DeepEqual(fact.Attributes, observation.Facts[index].Attributes) ||
			len(fact.Sources) != 1 || fact.Sources[0].ObservationID != observation.ID {
			t.Fatalf("fact lost data: %#v", fact)
		}
	}
	var writers sync.WaitGroup
	for range 6 {
		writers.Go(func() {
			independent, err := mem0.New(config)
			if err != nil {
				t.Error(err)
				return
			}
			result, err := independent.Observe(ctx, observation)
			if err != nil {
				t.Error(err)
				return
			}
			if len(result.Facts) != 2 || result.Facts[0].ID != first.Facts[0].ID || result.Facts[1].ID != first.Facts[1].ID {
				t.Error("retry changed fact identities")
			}
		})
	}
	writers.Wait()
	changed := observation
	changed.ObservedAt = observation.ObservedAt.Add(time.Second)
	if _, err := store.Observe(ctx, changed); !errors.Is(err, memory.ErrConflict) {
		t.Fatalf("changed payload error=%v", err)
	}
	recalled, err := store.Recall(ctx, memory.Query{Scope: scope, Text: "Marzipan salmon preference", Limit: 10})
	if err != nil || len(recalled.Matches) != 2 {
		t.Fatalf("recall facts=%d error=%v", len(recalled.Matches), err)
	}
	other := scope
	other.AppID += "-other"
	isolated, err := store.Recall(ctx, memory.Query{Scope: other, Text: "Marzipan salmon preference", Limit: 10})
	if err != nil || len(isolated.Matches) != 0 {
		t.Fatalf("scope isolation facts=%d error=%v", len(isolated.Matches), err)
	}
	wrongConfig := config
	wrongConfig.APIKey = "incorrect-test-key"
	wrong, err := mem0.New(wrongConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.Observe(ctx, observation); err == nil {
		t.Fatal("unauthenticated batch accepted")
	}
	if err := store.Delete(ctx, memory.DeleteRequest{Scope: scope, ID: first.Facts[0].ID}); err != nil {
		t.Fatal(err)
	}
	recalled, err = store.Recall(ctx, memory.Query{Scope: scope, Text: "Marzipan salmon preference", Limit: 10})
	if err != nil || len(recalled.Matches) != 1 {
		t.Fatalf("after delete facts=%d error=%v", len(recalled.Matches), err)
	}
	if err := store.PurgeScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	empty, err := store.ScopeEmpty(ctx, scope)
	if err != nil || !empty {
		t.Fatalf("purge empty=%v error=%v", empty, err)
	}
	t.Log("one Observe returned 2 facts; recall returned 2; lost-response and 6 concurrent retries retained IDs; conflict, isolation, authentication, delete and purge passed")
}

type loseFirstMem0Response struct{ once sync.Once }

func (client *loseFirstMem0Response) Do(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	lost := false
	client.once.Do(func() { lost = true })
	if lost {
		_ = response.Body.Close()
		return nil, errors.New("test intentionally discarded the committed response")
	}
	return response, nil
}
