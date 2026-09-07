package gizclaw

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func permissionTestPeer(t *testing.T) (*PeerConn, *stubSFUBindings) {
	t.Helper()
	runs := peerruntest.New(t)
	key := giznet.PublicKey{77}
	selection := apitypes.AgentSelection{WorkspaceName: testSFUWorkspaceName}
	if _, err := runs.SetRunAgent(t.Context(), key, selection); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.ActivateRunAgent(t.Context(), key, selection); err != nil {
		t.Fatal(err)
	}
	bindings := &stubSFUBindings{}
	peer := &PeerConn{Conn: &testGiznetConn{publicKey: key}, Service: &PeerService{manager: newSFUTestManager(runs, bindings)}, events: newPeerStreamEventBroker()}
	t.Cleanup(peer.stopInputAccessRefresh)
	return peer, bindings
}

func TestPermissionCacheAvoidsDatabaseAcrossTurns(t *testing.T) {
	peer, _ := permissionTestPeer(t)
	if value := peer.inputPermission(t.Context(), false); value.denial != nil {
		t.Fatal(value.denial)
	}
	if _, err := peer.Service.manager.PeerRun.DB.ExecContext(t.Context(), `DROP TABLE peer_runs`); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if allowed, err := peer.authorizeInputEvent(t.Context(), audioBOS("cached-turn")); err != nil || !allowed {
			t.Fatalf("BOS queried storage: %v %v", allowed, err)
		}
		if allowed, err := peer.authorizeAudioPacket(t.Context()); err != nil || !allowed {
			t.Fatalf("packet queried storage: %v %v", allowed, err)
		}
		if allowed, err := peer.authorizeInputEvent(t.Context(), audioEndEvent("cached-turn")); err != nil || !allowed {
			t.Fatalf("EOS queried storage: %v %v", allowed, err)
		}
	}
	peer.permissions.mu.Lock()
	peer.permissions.value.checkedAt = time.Now().Add(-peerPermissionMaxAge - time.Second)
	peer.permissions.mu.Unlock()
	if value := peer.inputPermission(t.Context(), false); value.denial == nil {
		t.Fatal("expired permission admitted input")
	}
}

func TestPermissionRefreshRevokesCachedMembership(t *testing.T) {
	peer, bindings := permissionTestPeer(t)
	if value := peer.inputPermission(t.Context(), false); value.denial != nil {
		t.Fatal(value.denial)
	}
	bindings.set(sfu.ErrNotMember)
	if value := peer.inputPermission(t.Context(), false); value.denial != nil {
		t.Fatal("ordinary turn unexpectedly refreshed remote membership")
	}
	if value := peer.inputPermission(t.Context(), true); value.denial == nil || value.denial.Code != sfuAccessRevokedCode {
		t.Fatalf("refresh did not revoke: %+v", value)
	}
	if peer.permissionAllowsAudio(0) {
		t.Fatal("revoked cache still admitted audio")
	}
}

type countingPermissionCatalog struct {
	sfuTestWorkspaceCatalog
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *countingPermissionCatalog) GetWorkspaceByName(ctx context.Context, name string) (apitypes.Workspace, error) {
	c.calls.Add(1)
	c.once.Do(func() { close(c.entered) })
	select {
	case <-c.release:
	case <-ctx.Done():
		return apitypes.Workspace{}, ctx.Err()
	}
	return c.sfuTestWorkspaceCatalog.GetWorkspaceByName(ctx, name)
}

func TestConcurrentPermissionChecksShareOneLookup(t *testing.T) {
	peer, _ := permissionTestPeer(t)
	catalog := &countingPermissionCatalog{sfuTestWorkspaceCatalog: sfuTestWorkspaces(), entered: make(chan struct{}), release: make(chan struct{})}
	peer.Service.manager.Workspaces = catalog
	done := make(chan peerInputPermission, 20)
	for range 20 {
		go func() { done <- peer.inputPermission(t.Context(), false) }()
	}
	select {
	case <-catalog.entered:
	case <-time.After(time.Second):
		t.Fatal("lookup did not start")
	}
	close(catalog.release)
	for range 20 {
		select {
		case value := <-done:
			if value.denial != nil {
				t.Fatal(value.denial)
			}
		case <-time.After(time.Second):
			t.Fatal("coalesced lookup stuck")
		}
	}
	if calls := catalog.calls.Load(); calls != 1 {
		t.Fatalf("catalog queries=%d want=1", calls)
	}
}

func TestRuntimeRevisionCannotReuseCachedPermission(t *testing.T) {
	peer, _ := permissionTestPeer(t)
	peer.agentHost = &agenthost.Service{PeerRun: peer.Service.manager.PeerRun, PublicKey: peer.Conn.PublicKey()}
	if value := peer.inputPermission(t.Context(), false); value.denial != nil {
		t.Fatal(value.denial)
	}
	if _, err := peer.agentHost.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.Service.manager.PeerRun.DB.ExecContext(t.Context(), `DROP TABLE peer_runs`); err != nil {
		t.Fatal(err)
	}
	if value := peer.inputPermission(t.Context(), false); value.denial == nil {
		t.Fatal("new runtime reused old permission without validation")
	}
}

func TestSlowPermissionRefreshDoesNotBlockVoiceBoundaries(t *testing.T) {
	peer, _ := permissionTestPeer(t)
	if value := peer.inputPermission(t.Context(), false); value.denial != nil {
		t.Fatal(value.denial)
	}
	catalog := &countingPermissionCatalog{sfuTestWorkspaceCatalog: sfuTestWorkspaces(), entered: make(chan struct{}), release: make(chan struct{})}
	peer.Service.manager.Workspaces = catalog
	refreshed := make(chan peerInputPermission, 1)
	go func() { refreshed <- peer.inputPermission(t.Context(), true) }()
	select {
	case <-catalog.entered:
	case <-time.After(time.Second):
		t.Fatal("background lookup did not start")
	}
	turns := make(chan error, 1)
	go func() {
		for range 20 {
			for _, event := range []*eventpb.PeerEvent{audioBOS("slow-refresh"), audioEndEvent("slow-refresh")} {
				allowed, err := peer.authorizeInputEvent(t.Context(), event)
				if err != nil || !allowed {
					turns <- fmt.Errorf("voice boundary rejected: allowed=%v error=%v", allowed, err)
					return
				}
			}
		}
		turns <- nil
	}()
	select {
	case err := <-turns:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("voice boundaries waited for slow storage")
	}
	select {
	case value := <-refreshed:
		if value.denial == nil || value.denial.Code != sfuAccessCheckFailedCode {
			t.Fatalf("timed-out refresh did not deny access: %+v", value)
		}
	case <-time.After(peerPermissionRequestTimeout + time.Second):
		t.Fatal("background storage lookup exceeded its deadline")
	}
	if calls := catalog.calls.Load(); calls != 1 {
		t.Fatalf("slow storage queries=%d want=1", calls)
	}
}

func TestPermissionRefreshWorkerStopsWithConnection(t *testing.T) {
	peer, _ := permissionTestPeer(t)
	peer.startInputAccessRefresh()
	peer.stopInputAccessRefresh()
	peer.permissions.mu.Lock()
	done := peer.permissions.done
	peer.permissions.mu.Unlock()
	select {
	case <-done:
	default:
		t.Fatal("permission worker remains after stop")
	}
	if value := peer.inputPermission(t.Context(), false); value.denial == nil {
		t.Fatal("closed connection admitted input")
	}
}

func TestBackgroundPermissionRefreshRevokesMembership(t *testing.T) {
	peer, bindings := permissionTestPeer(t)
	if value := peer.inputPermission(t.Context(), false); value.denial != nil {
		t.Fatal(value.denial)
	}
	// Production serves the mandatory Event stream through readEventStream,
	// without passing through the optional subscription helper.
	serverStream, clientStream := net.Pipe()
	streamDone := make(chan error, 1)
	go func() { streamDone <- peer.readEventStream(serverStream) }()
	t.Cleanup(func() {
		_ = clientStream.Close()
		_ = serverStream.Close()
		select {
		case err := <-streamDone:
			if err != nil {
				t.Errorf("event stream: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("event stream refresh worker did not stop")
		}
	})
	bindings.set(sfu.ErrNotMember)
	deadline := time.NewTimer(peerPermissionMaxAge + time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("background refresh did not revoke membership")
		case <-tick.C:
			value := peer.inputPermission(t.Context(), false)
			if value.denial != nil {
				if value.denial.Code != sfuAccessRevokedCode {
					t.Fatalf("refresh did not publish revocation: %+v", value.denial)
				}
				return
			}
		}
	}
}

func TestPermissionAdmissionWaitsForReloadPublication(t *testing.T) {
	peer, _ := permissionTestPeer(t)
	source := newPeerConnBlockingOpenInput()
	host := &agenthost.Service{
		Host:      peerConnTestHost{output: &peerConnBlockingStream{done: make(chan struct{})}},
		PeerRun:   peer.Service.manager.PeerRun,
		PublicKey: peer.Conn.PublicKey(),
		Source:    source,
		Consumer:  agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error { <-ctx.Done(); return nil }),
	}
	peer.agentHost = host
	reloadDone := make(chan error, 1)
	go func() { _, err := host.Reload(t.Context()); reloadDone <- err }()
	var release sync.Once
	defer func() {
		release.Do(func() { close(source.openRelease) })
		if err := <-reloadDone; err != nil {
			t.Errorf("reload: %v", err)
		}
		if _, err := host.Stop(context.Background()); err != nil {
			t.Errorf("stop: %v", err)
		}
	}()
	select {
	case <-source.openEntered:
	case <-time.After(time.Second):
		t.Fatal("reload did not enter input replacement")
	}
	waitCtx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if value := peer.inputPermission(waitCtx, false); value.denial == nil {
		t.Fatal("canceled admission accepted an unpublished runtime")
	}
	// The SDK can send the replacement BOS as soon as the old route ends,
	// while OpenAgentInput and runtime publication are still in progress.
	admitted := make(chan peerInputPermission, 1)
	go func() { admitted <- peer.inputPermission(t.Context(), false) }()
	select {
	case value := <-admitted:
		t.Fatalf("admission finished before runtime publication: %+v", value)
	case <-time.After(50 * time.Millisecond):
	}
	release.Do(func() { close(source.openRelease) })
	select {
	case value := <-admitted:
		if value.denial != nil || value.revision%2 != 0 || value.revision != host.RuntimeRevision() {
			t.Fatalf("replacement route was not admitted on the published revision: %+v", value)
		}
	case <-time.After(time.Second):
		t.Fatal("admission did not resume after publication")
	}
}
