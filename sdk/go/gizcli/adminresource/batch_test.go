package adminresource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type getterFunc func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)

func (f getterFunc) GetResource(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
	return f(ctx, kind, id)
}

func TestGetResourcesOrderedBoundedAndPartial(t *testing.T) {
	const count = 21
	refs := make([]Reference, count)
	for i := range refs {
		refs[i] = Reference{Kind: apitypes.ResourceKindModel, ID: fmt.Sprintf("model-%d", i)}
	}
	refs[3].ID = "missing"
	var active, peak atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release := make(chan struct{})
	started := make(chan struct{}, count)
	getter := getterFunc(func(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return apitypes.Resource{}, ctx.Err()
		}
		if id == "missing" {
			resp := apitypes.NewErrorResponse("MODEL_NOT_FOUND", "missing")
			return apitypes.Resource{}, responseError(404, nil, &resp)
		}
		return mustResource(t, fmt.Sprintf(`{"kind":%q,"metadata":{"id":%q}}`, kind, id)), nil
	})
	done := make(chan []Result, 1)
	go func() { done <- GetResources(ctx, getter, refs) }()
	for range BatchConcurrency {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("batch did not start the full concurrency window")
		}
	}
	close(release)
	results := <-done
	if peak.Load() != BatchConcurrency {
		t.Fatalf("peak concurrency = %d", peak.Load())
	}
	for i, result := range results {
		if i == 3 {
			if result.Resource != nil || !IsNotFound(result.Err) {
				t.Fatalf("missing result = %+v", result)
			}
			continue
		}
		if result.Err != nil {
			t.Fatalf("result %d error = %v", i, result.Err)
		}
		if kind, id, _ := resourceKindAndName(*result.Resource); kind != refs[i].Kind || id != refs[i].ID {
			t.Fatalf("result %d = %s/%s", i, kind, id)
		}
	}
}

func TestGetResourcesCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	results := GetResources(ctx, getterFunc(func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
		calls.Add(1)
		return apitypes.Resource{}, nil
	}), []Reference{{Kind: apitypes.ResourceKindModel, ID: "a"}})
	if calls.Load() != 0 || !errors.Is(results[0].Err, context.Canceled) {
		t.Fatalf("calls=%d result=%+v", calls.Load(), results[0])
	}
	if got := GetResources(context.Background(), nil, nil); len(got) != 0 {
		t.Fatalf("empty batch = %+v", got)
	}
}

func TestParseReference(t *testing.T) {
	ref, err := ParseReference("Model", "folder/resource%25:id")
	if err != nil || ref.Kind != apitypes.ResourceKindModel || ref.ID != "folder/resource%25:id" {
		t.Fatalf("ParseReference = %+v, %v", ref, err)
	}
	for _, tc := range [][3]string{
		{"Unknown", "a", "unknown resource kind"},
		{"ResourceList", "a", "cannot be addressed by ID"},
		{"Model", " a", "invalid resource ID"},
		{"Model", "..", "invalid resource ID"},
	} {
		if _, err := ParseReference(tc[0], tc[1]); err == nil || !strings.Contains(err.Error(), tc[2]) {
			t.Fatalf("ParseReference(%q, %q) error = %v", tc[0], tc[1], err)
		}
	}
}
