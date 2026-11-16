//go:build gizclaw_e2e

package multiserver_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"
)

// TestTiKVReadRoutingControls uses independent backend instances, so no local
// write overlay can satisfy the reads. Twelve bounded read-only scans per label
// exercise the SDK's randomized Learner selection without retrying mutations.
func TestTiKVReadRoutingControls(t *testing.T) {
	if os.Getenv("GIZCLAW_E2E_TIKV_READ_ROUTING") != "1" {
		t.Skip("requires the dedicated two-Learner read-routing fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	topology, ready, err := tikvfixture.ReadRouting(ctx, requiredEnv(t, "GIZCLAW_E2E_TIKV_PD"))
	if err != nil || !ready {
		t.Fatalf("two-Learner topology=%+v ready=%v err=%v", topology, ready, err)
	}
	t.Logf("healthy topology: %+v", topology)
	writer, err := kv.NewTiKVWithClient(openSharedTiKV(t), kv.TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "read-routing-" + rand.Text()
	first, second, collection := kv.Key{prefix, "first"}, kv.Key{prefix, "second"}, kv.Key{prefix, "members"}
	if err := writer.Set(ctx, first, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Set(ctx, second, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if err := writer.AddMembers(ctx, collection, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	for _, region := range []string{"local", "remote"} {
		t.Run(region, func(t *testing.T) {
			reader, err := kv.NewTiKVWithClient(openSharedTiKV(t), kv.TiKVReadOptions{Labels: map[string]string{"region": region}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Poll only ordinary point reads until the independently written
			// data and collection head/member are visible; routing errors fail.
			deadline := time.Now().Add(15 * time.Second)
			for {
				value, readErr := reader.Get(ctx, first)
				member, memberErr := reader.HasMember(ctx, collection, "bob")
				if readErr != nil && !errors.Is(readErr, kv.ErrNotFound) {
					t.Fatal(readErr)
				}
				if memberErr != nil {
					t.Fatal(memberErr)
				}
				if bytes.Equal(value, []byte("one")) && member {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("point-read visibility deadline")
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(100 * time.Millisecond):
				}
			}
			results := reader.BatchGet(ctx, []kv.Key{first, second, first})
			for i, want := range []string{"one", "two", "one"} {
				if results[i].Err != nil || string(results[i].Value) != want {
					t.Fatalf("BatchGet[%d]=%+v", i, results[i])
				}
			}
			t.Log("Get/HasMember/BatchGet controls passed before scans")
			for attempt := range 12 {
				members, err := reader.ListMembers(ctx, collection)
				slices.Sort(members)
				if err != nil || !slices.Equal(members, []string{"alice", "bob"}) {
					t.Errorf("scan %d: members=%v error=%v", attempt+1, members, err)
				}
			}
		})
	}
	t.Run("unmatched-label-rejected", func(t *testing.T) {
		reader, err := kv.NewTiKVWithClient(openSharedTiKV(t), kv.TiKVReadOptions{Labels: map[string]string{"region": "missing"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Get(ctx, first); err == nil || !strings.Contains(err.Error(), "no available Learner matching configured read labels") {
			t.Fatalf("strict label guard did not reject unmatched reader: %v", err)
		}
	})
}
