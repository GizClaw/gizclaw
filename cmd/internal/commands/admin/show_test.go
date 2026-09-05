package admincmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type batchShowClient struct {
	resourceClient
	get    func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)
	closes atomic.Int32
}

func (c *batchShowClient) GetResource(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
	return c.get(ctx, kind, id)
}
func (c *batchShowClient) Close() error { c.closes.Add(1); return nil }

func installShowClient(t *testing.T, client resourceClient) *atomic.Int32 {
	t.Helper()
	old := openResourceClient
	t.Cleanup(func() { openResourceClient = old })
	opens := new(atomic.Int32)
	openResourceClient = func(name string) (resourceClient, error) {
		opens.Add(1)
		if name != "batch-test" {
			return nil, fmt.Errorf("unexpected context %q", name)
		}
		return client, nil
	}
	return opens
}

func TestShowBatchConcurrentOrderedAndSharedConnection(t *testing.T) {
	const count = 19
	refs := make([]resourceReference, count)
	for i := range refs {
		refs[i] = resourceReference{Kind: apitypes.ResourceKindModel, ID: fmt.Sprintf("model-%d", i)}
	}
	refs[1].Kind = apitypes.ResourceKindRuntimeProfile
	refs[18] = refs[0] // Duplicate references preserve their positions.
	input, _ := json.Marshal(refs)
	entered := make(chan struct{}, count)
	release := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var active, peak atomic.Int32
	client := &batchShowClient{get: func(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p; p = peak.Load() {
			if peak.CompareAndSwap(p, n) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return apitypes.Resource{}, ctx.Err()
		}
		var resource apitypes.Resource
		err := json.Unmarshal(fmt.Appendf(nil, `{"kind":%q,"metadata":{"id":%q}}`, kind, id), &resource)
		return resource, err
	}}
	opens := installShowClient(t, client)
	cmd := NewCmd()
	cmd.SetArgs([]string{"show", "-f", "-", "--context", "batch-test"})
	cmd.SetIn(bytes.NewReader(input))
	var out bytes.Buffer
	cmd.SetOut(&out)
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	for range 8 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("eight requests did not start concurrently")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 8 || opens.Load() != 1 || client.closes.Load() != 1 {
		t.Fatalf("peak=%d opens=%d closes=%d", peak.Load(), opens.Load(), client.closes.Load())
	}
	var got []struct {
		Kind     apitypes.ResourceKind
		Metadata struct{ ID string }
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != count {
		t.Fatalf("results=%d", len(got))
	}
	for i, ref := range refs {
		if got[i].Kind != ref.Kind || got[i].Metadata.ID != ref.ID {
			t.Fatalf("result %d = %+v", i, got[i])
		}
	}
}

func TestShowBatchPartialFailure(t *testing.T) {
	resource := mustResource(t, `{"kind":"Model","metadata":{"id":"ok"}}`)
	client := &batchShowClient{get: func(_ context.Context, _ apitypes.ResourceKind, id string) (apitypes.Resource, error) {
		if id == "missing" {
			return apitypes.Resource{}, errors.New("NOT_FOUND")
		}
		return resource, nil
	}}
	installShowClient(t, client)
	cmd := NewCmd()
	cmd.SetArgs([]string{"show", "-f", "-", "--context", "batch-test"})
	cmd.SetIn(strings.NewReader(`[{"kind":"Model","id":"ok"},{"kind":"Model","id":"missing"},{"kind":"Model","id":"ok"}]`))
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "[1] Model/missing: NOT_FOUND") {
		t.Fatalf("error=%v", err)
	}
	var results []json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || string(results[1]) != "null" || string(results[0]) == "null" || string(results[2]) == "null" {
		t.Fatalf("results=%s", out.Bytes())
	}
	if !strings.Contains(stderr.String(), "Model/missing") {
		t.Fatalf("stderr=%q", stderr.String())
	}
	if client.closes.Load() != 1 {
		t.Fatal("client not closed")
	}
}

func TestShowBatchRejectsInvalidInputBeforeConnect(t *testing.T) {
	opens := installShowClient(t, nil)
	for _, input := range []string{
		`null`, `{}`, ``, `[] []`, `[] garbage`, `[null]`,
		`[{"kind":"Model","id":"ok"},{"kind":"Unknown","id":"bad"}]`,
		`[{"kind":"ResourceList","id":"list"}]`, `[{"kind":"Model"}]`,
		`[{"kind":"Model","id":" bad"}]`, `[{"kind":"Model","id":"ok","typo":true}]`,
	} {
		t.Run(input, func(t *testing.T) {
			cmd := NewCmd()
			cmd.SetArgs([]string{"show", "-f", "-"})
			cmd.SetIn(strings.NewReader(input))
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			if err := cmd.Execute(); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
	if opens.Load() != 0 {
		t.Fatal("invalid input opened a connection")
	}
}

func TestShowBatchFileAndEmpty(t *testing.T) {
	resource := mustResource(t, `{"kind":"Model","metadata":{"id":"ok"}}`)
	client := &batchShowClient{get: func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) { return resource, nil }}
	opens := installShowClient(t, client)
	file := filepath.Join(t.TempDir(), "refs.json")
	if err := os.WriteFile(file, []byte(`[{"kind":"Model","id":"ok"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := NewCmd()
	cmd.SetArgs([]string{"show", "-f", file, "--context", "batch-test"})
	cmd.SetOut(new(bytes.Buffer))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if opens.Load() != 1 {
		t.Fatal("file not queried")
	}
	cmd = NewCmd()
	cmd.SetArgs([]string{"show", "-f", "-"})
	cmd.SetIn(strings.NewReader(`[]`))
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "[]\n" || opens.Load() != 1 {
		t.Fatalf("empty batch output=%q opens=%d", out.String(), opens.Load())
	}
}

func TestShowBatchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &batchShowClient{get: func(ctx context.Context, _ apitypes.ResourceKind, _ string) (apitypes.Resource, error) {
		cancel()
		<-ctx.Done()
		return apitypes.Resource{}, ctx.Err()
	}}
	installShowClient(t, client)
	cmd := NewCmd()
	cmd.SetArgs([]string{"show", "-f", "-", "--context", "batch-test"})
	cmd.SetIn(strings.NewReader(`[{"kind":"Model","id":"a"},{"kind":"Model","id":"b"}]`))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(new(bytes.Buffer))
	if err := cmd.ExecuteContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if out.String() != "[null,null]\n" || client.closes.Load() != 1 {
		t.Fatalf("output=%q closes=%d", out.String(), client.closes.Load())
	}
}

func TestShowBatchArgumentConflicts(t *testing.T) {
	opens := installShowClient(t, nil)
	for _, args := range [][]string{{"show", "Model", "id", "-f", "-"}, {"show", "-f", ""}, {"show"}, {"show", "-f", "-", "--parallel", "8"}} {
		cmd := NewCmd()
		cmd.SetArgs(args)
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if opens.Load() != 0 {
		t.Fatal("invalid arguments opened a connection")
	}
}
