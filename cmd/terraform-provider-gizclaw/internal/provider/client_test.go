package provider

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
)

const modelManifest = `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"example-model"},"spec":{"kind":"llm","source":"manual"}}`

func TestAdminClientApplyExpandsEnvironmentAndReadsBack(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_PROVIDER_SECRET", `secret"value`)
	server := newFakeServer()
	client, connector := newTestClient(server)
	manifest := `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Credential","metadata":{"id":"example-credential"},"spec":{"provider":"openai","body":{"api_key":"${GIZCLAW_TEST_PROVIDER_SECRET}"}}}`
	stored, err := client.apply(context.Background(), []byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Kind != "Credential" || stored.Metadata.ID != "example-credential" {
		t.Fatalf("stored = %+v", stored)
	}
	if len(server.applied) != 1 || !strings.Contains(string(server.applied[0]), `secret\"value`) {
		t.Fatalf("applied = %s", server.applied)
	}
	if !strings.Contains(string(stored.Spec), `secret\"value`) {
		t.Fatalf("stored spec = %s", stored.Spec)
	}
	if connector.connects.Load() != 1 {
		t.Fatalf("connects = %d", connector.connects.Load())
	}
}

func TestAdminClientApplyRejectsMissingEnvironment(t *testing.T) {
	client, connector := newTestClient(newFakeServer())
	_, err := client.apply(context.Background(), []byte(`{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"m"},"spec":{"k":"${GIZCLAW_TEST_PROVIDER_MISSING}"}}`))
	if _, ok := errors.AsType[*adminresource.MissingEnvError](err); !ok {
		t.Fatalf("error = %v", err)
	}
	if connector.connects.Load() != 0 {
		t.Fatal("invalid manifest opened a connection")
	}
}

func TestAdminClientReusesOneConnection(t *testing.T) {
	server := newFakeServer()
	server.put(t, modelManifest)
	client, connector := newTestClient(server)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := client.get(context.Background(), "Model", "example-model"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if _, err := client.apply(context.Background(), []byte(modelManifest)); err != nil {
		t.Fatal(err)
	}
	if err := client.delete(context.Background(), "Model", "example-model"); err != nil {
		t.Fatal(err)
	}
	if connector.connects.Load() != 1 || connector.closes.Load() != 0 {
		t.Fatalf("connects=%d closes=%d", connector.connects.Load(), connector.closes.Load())
	}
}

func TestAdminClientReconnectsAfterTransportFailure(t *testing.T) {
	server := newFakeServer()
	server.put(t, modelManifest)
	server.failNext.Store(2)
	client, connector := newTestClient(server)
	if _, err := client.get(context.Background(), "Model", "example-model"); err != nil {
		t.Fatal(err)
	}
	if connector.connects.Load() != 3 || connector.closes.Load() != 2 {
		t.Fatalf("connects=%d closes=%d", connector.connects.Load(), connector.closes.Load())
	}
}

func TestAdminClientRetriesConnectFailure(t *testing.T) {
	server := newFakeServer()
	server.put(t, modelManifest)
	client, connector := newTestClient(server)
	connector.failures.Store(1)
	if _, err := client.get(context.Background(), "Model", "example-model"); err != nil {
		t.Fatal(err)
	}
	if connector.connects.Load() != 2 {
		t.Fatalf("connects = %d", connector.connects.Load())
	}
}

func TestAdminClientStopsAfterMaxAttempts(t *testing.T) {
	server := newFakeServer()
	server.failNext.Store(10)
	client, connector := newTestClient(server)
	_, err := client.get(context.Background(), "Model", "example-model")
	if !errors.Is(err, errTransport) {
		t.Fatalf("error = %v", err)
	}
	if connector.connects.Load() != maxOperationAttempts || server.gets.Load() != maxOperationAttempts {
		t.Fatalf("connects=%d gets=%d", connector.connects.Load(), server.gets.Load())
	}
}

func TestAdminClientDoesNotRetryStructuredErrors(t *testing.T) {
	server := newFakeServer()
	client, connector := newTestClient(server)
	_, err := client.get(context.Background(), "Model", "missing")
	if !adminresource.IsNotFound(err) {
		t.Fatalf("error = %v", err)
	}
	if server.gets.Load() != 1 || connector.closes.Load() != 0 {
		t.Fatalf("gets=%d closes=%d", server.gets.Load(), connector.closes.Load())
	}
}

func TestAdminClientRejectsInvalidReferenceBeforeConnecting(t *testing.T) {
	client, connector := newTestClient(newFakeServer())
	if _, err := client.get(context.Background(), "Unknown", "x"); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if err := client.delete(context.Background(), "Model", " x"); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if connector.connects.Load() != 0 {
		t.Fatal("invalid reference opened a connection")
	}
}

func TestAdminClientCanceledContext(t *testing.T) {
	client, connector := newTestClient(newFakeServer())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.get(ctx, "Model", "m"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if connector.connects.Load() != 0 {
		t.Fatal("canceled operation opened a connection")
	}
}

func TestAdminClientConnectIsCancellable(t *testing.T) {
	client, connector := newTestClient(newFakeServer())
	connector.block = make(chan struct{})
	defer close(connector.block)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.get(ctx, "Model", "m")
		done <- err
	}()
	for connector.connects.Load() == 0 {
		runtime.Gosched()
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not stop the blocked connect")
	}
	if connector.connects.Load() != 1 {
		t.Fatalf("connects = %d, want no retry after cancellation", connector.connects.Load())
	}
}

func TestAdminClientWaiterStopsOnOwnCancellation(t *testing.T) {
	server := newFakeServer()
	server.put(t, modelManifest)
	client, connector := newTestClient(server)
	connector.block = make(chan struct{})
	leader := make(chan error, 1)
	go func() {
		_, err := client.get(context.Background(), "Model", "example-model")
		leader <- err
	}()
	for connector.connects.Load() == 0 {
		runtime.Gosched()
	}
	waiterCtx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() {
		_, err := client.get(waiterCtx, "Model", "example-model")
		waiter <- err
	}()
	// Give the waiter time to join the in-flight dial before cancelling it.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-waiter:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiter error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiter ignored its own cancellation")
	}
	close(connector.block)
	if err := <-leader; err != nil {
		t.Fatalf("leader error = %v", err)
	}
	if connector.connects.Load() != 1 {
		t.Fatalf("connects = %d", connector.connects.Load())
	}
}
