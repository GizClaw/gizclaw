package agenthost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServiceReloadAppliesPendingAndStop(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	output := newBlockingStream()
	host := &fakeHost{output: output}
	input := NewInputStream(1)
	svc := &Service{
		Host:      host,
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return input, nil
		}),
		Consumer: StreamConsumerFunc(func(ctx context.Context, stream genx.Stream) error {
			for {
				_, err := stream.Next()
				if IsStreamDone(err) || errors.Is(err, io.ErrClosedPipe) {
					return nil
				}
				if err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
			}
		}),
		Now: fixedClock(time.Unix(100, 0)),
	}

	status, err := svc.Reload(ctx)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateRunning || status.WorkspaceName == nil || *status.WorkspaceName != "demo" {
		t.Fatalf("Reload() status = %+v", status)
	}
	if host.pattern != "workspaces/demo" {
		t.Fatalf("host pattern = %q, want workspaces/demo", host.pattern)
	}
	agent, err := store.GetRunAgent(ctx, publicKey)
	if err != nil {
		t.Fatalf("GetRunAgent() error = %v", err)
	}
	if agent.Pending != nil || agent.Active == nil || agent.Active.WorkspaceName != "demo" {
		t.Fatalf("run agent after reload = %+v", agent)
	}

	status, err = svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateRunning {
		t.Fatalf("Status() = %+v", status)
	}
	status, err = svc.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("Stop() status = %+v", status)
	}
	if !input.closed() || !output.closed() {
		t.Fatalf("runtime streams closed: input=%v output=%v", input.closed(), output.closed())
	}
}

func TestServiceReloadNotifiesOnlyPublishedWorkspaceActivation(t *testing.T) {
	ctx := t.Context()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	activated := make(chan string, 1)
	svc := testService(t, publicKey, store, &fakeHost{output: newBlockingStream()})
	svc.OnWorkspaceActivated = func(_ context.Context, workspaceName string) {
		activated <- workspaceName
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	select {
	case workspaceName := <-activated:
		if workspaceName != "demo" {
			t.Fatalf("activated Workspace = %q, want demo", workspaceName)
		}
	default:
		t.Fatal("published Workspace activation was not reported")
	}
	if _, err := svc.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	failed := testService(t, publicKey, store, &fakeHost{err: errors.New("open failed")})
	failed.OnWorkspaceActivated = func(_ context.Context, workspaceName string) {
		activated <- workspaceName
	}
	if _, err := failed.Reload(ctx); err == nil {
		t.Fatal("failed Reload() succeeded")
	}
	select {
	case workspaceName := <-activated:
		t.Fatalf("failed Workspace activation reported for %q", workspaceName)
	default:
	}
}

func TestServiceReloadAndStopSerializeTransitions(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	source := newBlockingOpenSource()
	svc := &Service{
		Host:      &fakeHost{output: newBlockingStream()},
		PeerRun:   store,
		PublicKey: publicKey,
		Source:    source,
		Consumer: StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}

	reloadDone := make(chan error, 1)
	go func() {
		_, err := svc.Reload(ctx)
		reloadDone <- err
	}()
	select {
	case <-source.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Reload to open input")
	}

	stopDone := make(chan error, 1)
	go func() {
		_, err := svc.Stop(ctx)
		stopDone <- err
	}()
	secondReloadDone := make(chan error, 1)
	go func() {
		_, err := svc.Reload(ctx)
		secondReloadDone <- err
	}()
	for name, done := range map[string]<-chan error{"Stop": stopDone, "second Reload": secondReloadDone} {
		select {
		case err := <-done:
			t.Fatalf("%s completed before the first transition released: %v", name, err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	if got := source.openCalls(); got != 1 {
		t.Fatalf("OpenAgentInput calls while first transition blocked = %d, want 1", got)
	}
	close(source.release)
	for name, done := range map[string]<-chan error{"first Reload": reloadDone, "Stop": stopDone, "second Reload": secondReloadDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s error = %v", name, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", name)
		}
	}
	if _, err := svc.Stop(ctx); err != nil {
		t.Fatalf("final Stop() error = %v", err)
	}
}

func TestServiceShutdownPreventsConcurrentReloadPublication(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	blockingStore := newBlockingActivateStore(store)
	input := NewInputStream(1)
	output := newBlockingStream()
	svc := &Service{
		Host:      &fakeHost{output: output},
		PeerRun:   blockingStore,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return input, nil
		}),
		Consumer: StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}

	reloadDone := make(chan error, 1)
	go func() {
		_, err := svc.Reload(ctx)
		reloadDone <- err
	}()
	select {
	case <-blockingStore.activateEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Reload activation")
	}
	status, err := svc.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("Shutdown() status = %+v", status)
	}
	close(blockingStore.activateRelease)
	select {
	case err := <-reloadDone:
		if !errors.Is(err, ErrServiceClosed) {
			t.Fatalf("Reload() error = %v, want service closed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Reload after Shutdown")
	}
	if !input.closed() || !output.closed() {
		t.Fatalf("streams closed after Shutdown: input=%v output=%v", input.closed(), output.closed())
	}
	status, err = svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("Status() after Shutdown = %+v", status)
	}
	if _, err := svc.Reload(ctx); !errors.Is(err, ErrServiceClosed) {
		t.Fatalf("Reload() after Shutdown error = %v, want service closed", err)
	}
}

func TestServiceReloadCanceledWhileWaitingKeepsPublishedStatus(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	svc := testService(t, publicKey, store, &fakeHost{})
	if err := svc.lockTransition(ctx); err != nil {
		t.Fatalf("lockTransition() error = %v", err)
	}
	defer svc.unlockTransition()

	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	status, err := svc.Reload(waitCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Reload() error = %v, want deadline exceeded", err)
	}
	if status.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("Reload() status = %+v, want stopped", status)
	}
	published, statusErr := svc.Status(ctx)
	if statusErr != nil {
		t.Fatalf("Status() error = %v", statusErr)
	}
	if published.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("published status = %+v, want stopped", published)
	}
}

func TestServiceInputRecoveryDropsSupersededTransition(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	source := newBlockingOpenSource()
	svc := &Service{
		Host:      &fakeHost{output: newBlockingStream()},
		PeerRun:   store,
		PublicKey: publicKey,
		Source:    source,
		Consumer: StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	observed := svc.RuntimeRevision()
	reloadDone := make(chan error, 1)
	go func() {
		_, err := svc.Reload(ctx)
		reloadDone <- err
	}()
	select {
	case <-source.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Reload to open input")
	}
	recoverDone := make(chan struct {
		reloaded bool
		err      error
	}, 1)
	go func() {
		reloaded, err := svc.ReloadIfCurrentRevision(ctx, observed)
		recoverDone <- struct {
			reloaded bool
			err      error
		}{reloaded: reloaded, err: err}
	}()
	close(source.release)
	select {
	case err := <-reloadDone:
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Reload")
	}
	select {
	case result := <-recoverDone:
		if result.err != nil || result.reloaded {
			t.Fatalf("ReloadIfCurrentRevision() = (%v, %v), want (false, nil)", result.reloaded, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for input recovery")
	}
	if got := source.openCalls(); got != 1 {
		t.Fatalf("OpenAgentInput calls = %d, want 1", got)
	}
	if _, err := svc.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestServicePushInputDropsSupersededRevision(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	svc := &Service{
		Host:      &fakeHost{output: newBlockingStream()},
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return NewInputStream(1), nil
		}),
		Consumer: StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("initial Reload() error = %v", err)
	}
	defer func() {
		if _, err := svc.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()
	observed := svc.RuntimeRevision()
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("superseding Reload() error = %v", err)
	}
	pushed, err := svc.PushInputIfCurrentRevision(ctx, observed, inputPusherFunc(func(context.Context, *genx.MessageChunk) error {
		t.Fatal("Push() called for a superseded runtime revision")
		return nil
	}), &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}})
	if err != nil || pushed {
		t.Fatalf("PushInputIfCurrentRevision() = (%v, %v), want (false, nil)", pushed, err)
	}
}

func TestServicePushInputRequiresPusher(t *testing.T) {
	pushed, err := (&Service{}).PushInputIfCurrentRevision(context.Background(), 0, nil, nil)
	if pushed || !errors.Is(err, ErrMissingInputPusher) {
		t.Fatalf("PushInputIfCurrentRevision() = (%v, %v), want (false, %v)", pushed, err, ErrMissingInputPusher)
	}
	revision, pushed, err := (&Service{}).PushInput(context.Background(), nil, nil)
	if revision != 0 || pushed || !errors.Is(err, ErrMissingInputPusher) {
		t.Fatalf("PushInput() = (%d, %v, %v), want (0, false, %v)", revision, pushed, err, ErrMissingInputPusher)
	}
}

func TestServiceReloadAndPushKeepsRetryInsideTransition(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	svc := &Service{
		Host:      &fakeHost{output: newBlockingStream()},
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return NewInputStream(1), nil
		}),
		Consumer: StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	defer func() {
		if _, err := svc.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()
	pushEntered := make(chan struct{})
	pushRelease := make(chan struct{})
	recoveryDone := make(chan struct {
		reloaded bool
		err      error
	}, 1)
	go func() {
		reloaded, err := svc.ReloadAndPushInputIfCurrentRevision(ctx, svc.RuntimeRevision(), inputPusherFunc(func(context.Context, *genx.MessageChunk) error {
			close(pushEntered)
			<-pushRelease
			return nil
		}), &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}})
		recoveryDone <- struct {
			reloaded bool
			err      error
		}{reloaded: reloaded, err: err}
	}()
	select {
	case <-pushEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recovery retry")
	}
	selectionDone := make(chan error, 1)
	go func() {
		_, err := svc.SetRunAgent(ctx, apitypes.AgentSelection{WorkspaceName: "assistant"})
		selectionDone <- err
	}()
	select {
	case err := <-selectionDone:
		t.Fatalf("SetRunAgent() completed while recovery retry held the transition boundary: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(pushRelease)
	select {
	case result := <-recoveryDone:
		if result.err != nil || !result.reloaded {
			t.Fatalf("ReloadAndPushInputIfCurrentRevision() = (%v, %v), want (true, nil)", result.reloaded, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recovery retry")
	}
	select {
	case err := <-selectionDone:
		if err != nil {
			t.Fatalf("SetRunAgent() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for selection")
	}
}

func TestServiceSelectionChangeInvalidatesInputRecovery(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := newBlockingSelectionStore()
	svc := &Service{
		Host:      &fakeHost{},
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return NewInputStream(1), nil
		}),
		Consumer: StreamConsumerFunc(func(context.Context, genx.Stream) error { return nil }),
	}
	observed := svc.RuntimeRevision()
	setDone := make(chan error, 1)
	go func() {
		_, err := svc.SetRunAgent(ctx, apitypes.AgentSelection{WorkspaceName: "assistant"})
		setDone <- err
	}()
	select {
	case <-store.setEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for selection write")
	}
	recoverDone := make(chan struct {
		reloaded bool
		err      error
	}, 1)
	go func() {
		reloaded, err := svc.ReloadIfCurrentRevision(ctx, observed)
		recoverDone <- struct {
			reloaded bool
			err      error
		}{reloaded: reloaded, err: err}
	}()
	close(store.setRelease)
	select {
	case err := <-setDone:
		if err != nil {
			t.Fatalf("SetRunAgent() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for selection write")
	}
	select {
	case result := <-recoverDone:
		if result.err != nil || result.reloaded {
			t.Fatalf("ReloadIfCurrentRevision() = (%v, %v), want (false, nil)", result.reloaded, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for input recovery")
	}
}

func TestServiceInputRecoveryDropsPendingWorkspaceAfterRuntimeStops(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	demo := apitypes.AgentSelection{WorkspaceName: "demo"}
	assistant := apitypes.AgentSelection{WorkspaceName: "assistant"}
	if _, err := store.SetRunAgent(ctx, publicKey, demo); err != nil {
		t.Fatalf("SetRunAgent(demo) error = %v", err)
	}
	if _, err := store.ActivateRunAgent(ctx, publicKey, demo); err != nil {
		t.Fatalf("ActivateRunAgent(demo) error = %v", err)
	}
	if _, err := store.SetRunAgent(ctx, publicKey, assistant); err != nil {
		t.Fatalf("SetRunAgent(assistant) error = %v", err)
	}
	openCalls := 0
	svc := &Service{
		Host:      &fakeHost{},
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			openCalls++
			return NewInputStream(1), nil
		}),
		Consumer: StreamConsumerFunc(func(context.Context, genx.Stream) error { return nil }),
	}
	reloaded, err := svc.ReloadIfCurrentRevision(ctx, svc.RuntimeRevision())
	if err != nil || reloaded {
		t.Fatalf("ReloadIfCurrentRevision() = (%v, %v), want (false, nil)", reloaded, err)
	}
	if openCalls != 0 {
		t.Fatalf("OpenAgentInput calls = %d, want 0", openCalls)
	}
}

func TestServiceSameActiveSelectionAfterRuntimeStopsKeepsRevision(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	demo := apitypes.AgentSelection{WorkspaceName: "demo"}
	if _, err := store.SetRunAgent(ctx, publicKey, demo); err != nil {
		t.Fatalf("SetRunAgent(demo) error = %v", err)
	}
	if _, err := store.ActivateRunAgent(ctx, publicKey, demo); err != nil {
		t.Fatalf("ActivateRunAgent(demo) error = %v", err)
	}
	svc := testService(t, publicKey, store, &fakeHost{})
	observed := svc.RuntimeRevision()
	if _, err := svc.SetRunAgent(ctx, demo); err != nil {
		t.Fatalf("SetRunAgent(demo) error = %v", err)
	}
	if got := svc.RuntimeRevision(); got != observed {
		t.Fatalf("RuntimeRevision() = %d, want %d", got, observed)
	}
}

func TestServiceSelectionPersistenceFailureKeepsRevision(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	wantErr := errors.New("persist selection")
	store := selectionErrorStore{
		PeerRunStore: fakePeerRunStore{},
		run: apitypes.PeerRunAgent{
			Active: &apitypes.AgentSelection{WorkspaceName: "demo"},
		},
		err: wantErr,
	}
	svc := &Service{PeerRun: store, PublicKey: publicKey}
	observed := svc.RuntimeRevision()

	if _, err := svc.SetRunAgent(ctx, apitypes.AgentSelection{WorkspaceName: "assistant"}); !errors.Is(err, wantErr) {
		t.Fatalf("SetRunAgent() error = %v, want %v", err, wantErr)
	}
	if got := svc.RuntimeRevision(); got != observed {
		t.Fatalf("RuntimeRevision() = %d, want %d", got, observed)
	}
}

func TestRuntimeProfileToolBindingsPreserveAliases(t *testing.T) {
	tools := map[string]apitypes.RuntimeProfileBinding{
		"weather": {ResourceId: "tool-weather"},
		"clock":   {ResourceId: "tool-clock"},
		"alarm":   {ResourceId: "tool-alarm"},
	}
	want := map[string]string{"weather": "tool-weather", "clock": "tool-clock", "alarm": "tool-alarm"}
	if got := runtimeProfileToolBindings(&tools); !maps.Equal(got, want) {
		t.Fatalf("runtimeProfileToolBindings() = %#v, want %#v", got, want)
	}
}

func TestServiceReloadMissingWorkspaceInstallsSafeErrorRuntime(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "missing"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	wantErr := errors.New("agenthost: workspace \"missing\" not found")
	svc := testService(t, publicKey, store, &fakeHost{err: wantErr})
	svc.Consumer = StreamConsumerFunc(func(ctx context.Context, stream genx.Stream) error {
		for {
			if _, err := stream.Next(); err != nil {
				if ctx.Err() != nil || IsStreamDone(err) || errors.Is(err, io.ErrClosedPipe) {
					return nil
				}
				return err
			}
		}
	})
	if _, err := svc.Reload(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("Reload() error = %v, want %v", err, wantErr)
	}
	agent, err := store.GetRunAgent(ctx, publicKey)
	if err != nil {
		t.Fatalf("GetRunAgent() error = %v", err)
	}
	if agent.Active != nil || agent.Pending == nil || agent.Pending.WorkspaceName != "missing" {
		t.Fatalf("run agent after failed reload = %+v", agent)
	}
	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateError ||
		status.Message == nil || *status.Message != agentReloadFailedMessage {
		t.Fatalf("Status() after failed reload = %+v", status)
	}
}

func TestServiceReloadRevalidatesPersistedWorkspaceSelection(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "removed-profile-workspace"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	wantErr := errors.New("workspace is no longer accessible")
	host := &fakeHost{output: newBlockingStream()}
	svc := testService(t, publicKey, store, host)
	svc.ValidateWorkspaceSelection = func(context.Context, string) (string, error) {
		return "", wantErr
	}

	if _, err := svc.Reload(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("Reload() error = %v, want %v", err, wantErr)
	}
	if host.pattern != "" {
		t.Fatalf("host pattern = %q, want no runtime open", host.pattern)
	}
	agent, err := store.GetRunAgent(ctx, publicKey)
	if err != nil {
		t.Fatalf("GetRunAgent() error = %v", err)
	}
	if agent.Active != nil || agent.Pending == nil || agent.Pending.WorkspaceName != "removed-profile-workspace" {
		t.Fatalf("run agent after rejected reload = %+v", agent)
	}
}

func TestServiceValidationAndDefaultStatus(t *testing.T) {
	if _, err := (*Service)(nil).Reload(context.Background()); !errors.Is(err, ErrNilService) {
		t.Fatalf("Reload(nil) error = %v, want %v", err, ErrNilService)
	}
	if _, err := (*Service)(nil).Status(context.Background()); !errors.Is(err, ErrNilService) {
		t.Fatalf("Status(nil) error = %v, want %v", err, ErrNilService)
	}
	status, err := (*Service)(nil).Stop(context.Background())
	if err != nil || status.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("Stop(nil) = %+v, %v", status, err)
	}

	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		svc  *Service
		err  error
	}{
		{name: "missing host", svc: &Service{}, err: ErrMissingHost},
		{name: "missing peer run", svc: &Service{Host: &fakeHost{}}, err: ErrMissingPeerRun},
		{name: "invalid public key", svc: &Service{Host: &fakeHost{}, PeerRun: store}, err: ErrInvalidPublicKey},
		{name: "missing source", svc: &Service{Host: &fakeHost{}, PeerRun: store, PublicKey: publicKey}, err: ErrMissingSource},
		{name: "missing consumer", svc: &Service{Host: &fakeHost{}, PeerRun: store, PublicKey: publicKey, Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return NewInputStream(1), nil
		})}, err: ErrMissingConsumer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.svc.Reload(context.Background()); !errors.Is(err, tc.err) {
				t.Fatalf("Reload() error = %v, want %v", err, tc.err)
			}
		})
	}

	svc := &Service{}
	status, err = svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status(default) error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("Status(default) = %+v", status)
	}
}

func TestServiceWorkspaceFeatureResponsesWithoutActiveWorkspace(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	state, err := svc.WorkspaceState(ctx)
	if err != nil {
		t.Fatalf("WorkspaceState() error = %v", err)
	}
	if state.RuntimeState != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("WorkspaceState() = %+v", state)
	}

	history, err := svc.ListWorkspaceHistory(ctx, apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListWorkspaceHistory() error = %v", err)
	}
	if history.Available || history.Message == nil || !strings.Contains(*history.Message, ErrNoActiveWorkspace.Error()) {
		t.Fatalf("ListWorkspaceHistory() = %+v", history)
	}

	play, err := svc.PlayWorkspaceHistory(ctx, apitypes.PeerRunHistoryPlayRequest{HistoryName: "h1"})
	if err != nil {
		t.Fatalf("PlayWorkspaceHistory() error = %v", err)
	}
	if play.Accepted || play.State != "unavailable" || play.Message == nil || !strings.Contains(*play.Message, ErrNoActiveWorkspace.Error()) {
		t.Fatalf("PlayWorkspaceHistory() = %+v", play)
	}

	memory, err := svc.WorkspaceMemoryStats(ctx, apitypes.PeerRunMemoryStatsRequest{})
	if err != nil {
		t.Fatalf("WorkspaceMemoryStats() error = %v", err)
	}
	if memory.Available || memory.Message == nil || !strings.Contains(*memory.Message, ErrNoActiveWorkspace.Error()) {
		t.Fatalf("WorkspaceMemoryStats() = %+v", memory)
	}

	recall, err := svc.WorkspaceRecall(ctx, apitypes.PeerRunRecallRequest{Query: "hello"})
	if err != nil {
		t.Fatalf("WorkspaceRecall() error = %v", err)
	}
	if recall.Available || recall.Message == nil || !strings.Contains(*recall.Message, ErrNoActiveWorkspace.Error()) {
		t.Fatalf("WorkspaceRecall() = %+v", recall)
	}
}

func TestServiceWorkspaceFeaturesRevalidateActiveWorkspace(t *testing.T) {
	wantErr := errors.New("workspace access revoked")
	svc := &Service{
		ValidateWorkspaceSelection: func(_ context.Context, name string) (string, error) {
			if name != "friend-workspace" {
				t.Fatalf("ValidateWorkspaceSelection name = %q, want friend-workspace", name)
			}
			return "", wantErr
		},
		runtime: &runtime{
			agent:     asAgent(fixedTransformer{text: "unused"}),
			workspace: "friend-workspace",
		},
	}

	history, err := svc.ListWorkspaceHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListWorkspaceHistory() error = %v", err)
	}
	if history.Available || history.Message == nil || !strings.Contains(*history.Message, wantErr.Error()) {
		t.Fatalf("ListWorkspaceHistory() = %+v", history)
	}
}

func TestTransformerAgentDefaults(t *testing.T) {
	agent := asAgent(fixedTransformer{text: "ok"})
	if agent == nil {
		t.Fatal("asAgent() = nil")
	}
	state, err := agent.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if state.RuntimeState != apitypes.PeerRunStatusStateRunning || state.HistoryAvailable == nil || *state.HistoryAvailable || state.MemoryStatsAvailable == nil || *state.MemoryStatsAvailable || state.RecallAvailable == nil || *state.RecallAvailable {
		t.Fatalf("Status() = %+v", state)
	}
	history, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if history.Available || len(history.Items) != 0 || history.Message == nil || !strings.Contains(*history.Message, unsupportedMessage) {
		t.Fatalf("ListHistory() = %+v", history)
	}
	play, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryName: "h1"})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if play.Accepted || play.HistoryName != "h1" || play.State != "unsupported" || play.Message == nil || !strings.Contains(*play.Message, unsupportedMessage) {
		t.Fatalf("PlayHistory() = %+v", play)
	}
	memory, err := agent.MemoryStats(context.Background(), apitypes.PeerRunMemoryStatsRequest{})
	if err != nil {
		t.Fatalf("MemoryStats() error = %v", err)
	}
	if memory.Available || memory.Message == nil || !strings.Contains(*memory.Message, unsupportedMessage) {
		t.Fatalf("MemoryStats() = %+v", memory)
	}
	recall, err := agent.Recall(context.Background(), apitypes.PeerRunRecallRequest{Query: "hello"})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if recall.Available || len(recall.Hits) != 0 || recall.Message == nil || !strings.Contains(*recall.Message, unsupportedMessage) {
		t.Fatalf("Recall() = %+v", recall)
	}
}

func TestServiceWorkspaceStateMergesOpenAgentStatus(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	available := true
	workflowName := "chat"
	agentType := "flowcraft"
	output := newBlockingStream()
	agent := &runtimeTestAgent{
		output: output,
		state: apitypes.PeerRunWorkspaceState{
			RuntimeState:         apitypes.PeerRunStatusStateRunning,
			WorkflowName:         &workflowName,
			AgentType:            &agentType,
			HistoryAvailable:     &available,
			MemoryStatsAvailable: &available,
			RecallAvailable:      &available,
		},
	}
	host := &runtimeTestOpenAgentHost{agent: agent}
	input := NewInputStream(1)
	svc := &Service{
		Host:      host,
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return input, nil
		}),
		Consumer: StreamConsumerFunc(func(ctx context.Context, stream genx.Stream) error {
			for {
				_, err := stream.Next()
				if IsStreamDone(err) || errors.Is(err, io.ErrClosedPipe) {
					return nil
				}
				if err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
			}
		}),
		Now: fixedClock(time.Unix(100, 0)),
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if host.pattern != "workspaces/demo" {
		t.Fatalf("host pattern = %q", host.pattern)
	}
	state, err := svc.WorkspaceState(ctx)
	if err != nil {
		t.Fatalf("WorkspaceState() error = %v", err)
	}
	if state.WorkspaceName != "demo" || state.ActiveWorkspaceName == nil || *state.ActiveWorkspaceName != "demo" {
		t.Fatalf("WorkspaceState() workspace = %+v", state)
	}
	if state.WorkflowName == nil || *state.WorkflowName != workflowName || state.AgentType == nil || *state.AgentType != agentType {
		t.Fatalf("WorkspaceState() agent fields = %+v", state)
	}
	if state.HistoryAvailable == nil || !*state.HistoryAvailable || state.MemoryStatsAvailable == nil || !*state.MemoryStatsAvailable || state.RecallAvailable == nil || !*state.RecallAvailable {
		t.Fatalf("WorkspaceState() availability = %+v", state)
	}
	if _, err := svc.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("release calls = %d, want 1", host.releaseCalls)
	}
}

func TestServiceReusesWorkspaceRuntimeForMultipleGears(t *testing.T) {
	ctx := context.Background()
	firstKey := testPublicKey(t)
	secondKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	for _, key := range []giznet.PublicKey{firstKey, secondKey} {
		if _, err := store.SetRunAgent(ctx, key, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
			t.Fatalf("SetRunAgent(%s) error = %v", key, err)
		}
	}
	agent := &multiAttachAgent{}
	factoryCalls := 0
	owner := "workspace-owner"
	host := New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo", OwnerPublicKey: &owner}, AgentType: "multi"}})
	if err := host.Register("multi", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		factoryCalls++
		return agent, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	firstConsumer := newRecordingConsumer()
	secondConsumer := newRecordingConsumer()
	first := testService(t, firstKey, store, host)
	first.Consumer = firstConsumer
	second := testService(t, secondKey, store, host)
	second.Consumer = secondConsumer

	if _, err := first.Reload(ctx); err != nil {
		t.Fatalf("first Reload() error = %v", err)
	}
	if _, err := second.Reload(ctx); err != nil {
		t.Fatalf("second Reload() error = %v", err)
	}
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
	if agent.transformCalls() != 2 {
		t.Fatalf("Transform calls = %d, want 2", agent.transformCalls())
	}
	if got := firstConsumer.nextText(t); got != firstKey.String() {
		t.Fatalf("first output = %q, want %q", got, firstKey.String())
	}
	if got := secondConsumer.nextText(t); got != secondKey.String() {
		t.Fatalf("second output = %q, want %q", got, secondKey.String())
	}
	if _, err := first.PlayWorkspaceHistory(ctx, apitypes.PeerRunHistoryPlayRequest{HistoryName: "h1"}); err != nil {
		t.Fatalf("first PlayWorkspaceHistory() error = %v", err)
	}
	if _, err := second.PlayWorkspaceHistory(ctx, apitypes.PeerRunHistoryPlayRequest{HistoryName: "h2"}); err != nil {
		t.Fatalf("second PlayWorkspaceHistory() error = %v", err)
	}
	if got, want := agent.playGearIDs(), []string{firstKey.String(), secondKey.String()}; !slices.Equal(got, want) {
		t.Fatalf("PlayHistory gear IDs = %v, want %v", got, want)
	}

	if _, err := first.Stop(ctx); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if _, err := host.coordinator().Acquire(ctx, "id-demo"); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("Acquire after first stop error = %v, want %v", err, ErrWorkspaceBusy)
	}
	if _, err := second.Stop(ctx); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
	lease, err := host.coordinator().Acquire(ctx, "id-demo")
	if err != nil {
		t.Fatalf("Acquire after both stops error = %v", err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("Release after both stops error = %v", err)
	}
}

func TestRuntimeKeyUsesCanonicalWorkspaceOnly(t *testing.T) {
	if runtimeKey("system") != runtimeKey("system") {
		t.Fatal("same Workspace produced different runtime keys")
	}
	if runtimeKey("system") == runtimeKey("other") {
		t.Fatal("different Workspaces produced the same runtime key")
	}
}

func TestServiceReloadSourceAndOutputErrors(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	sourceErr := errors.New("source failed")
	svc := testService(t, publicKey, store, &fakeHost{})
	sourceCalls := 0
	svc.Source = StreamSourceFunc(func(context.Context) (genx.Stream, error) {
		sourceCalls++
		return nil, sourceErr
	})
	if _, err := svc.Reload(ctx); !errors.Is(err, sourceErr) {
		t.Fatalf("Reload(source error) error = %v, want %v", err, sourceErr)
	}
	if sourceCalls != 1 || svc.currentRuntime() != nil {
		t.Fatalf(
			"Reload(source error) calls/runtime = %d/%p, want 1/nil",
			sourceCalls,
			svc.currentRuntime(),
		)
	}

	svc = testService(t, publicKey, store, &fakeHost{})
	svc.Source = StreamSourceFunc(func(context.Context) (genx.Stream, error) {
		return nil, nil
	})
	if _, err := svc.Reload(ctx); err == nil || !strings.Contains(err.Error(), "input stream") {
		t.Fatalf("Reload(nil input) error = %v", err)
	}

	svc = testService(t, publicKey, store, &nilOutputHost{})
	if _, err := svc.Reload(ctx); err == nil || !strings.Contains(err.Error(), "output stream") {
		t.Fatalf("Reload(nil output) error = %v", err)
	}
}

func TestServiceReloadCancellationAfterInputOpenInstallsErrorRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{
		WorkspaceName: "demo",
	}); err != nil {
		t.Fatal(err)
	}
	source := NewPushSource(4)
	openCalls := 0
	outputs := make(chan *genx.MessageChunk, 1)
	svc := testService(t, publicKey, store, &fakeHost{})
	svc.Source = StreamSourceFunc(func(ctx context.Context) (genx.Stream, error) {
		openCalls++
		input, err := source.OpenAgentInput(ctx)
		if openCalls == 1 {
			cancel()
		}
		return input, err
	})
	svc.Consumer = StreamConsumerFunc(func(ctx context.Context, stream genx.Stream) error {
		for {
			chunk, err := stream.Next()
			if err != nil {
				if ctx.Err() != nil || IsStreamDone(err) || errors.Is(err, io.ErrClosedPipe) {
					return nil
				}
				return err
			}
			select {
			case outputs <- chunk:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	if _, err := svc.Reload(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Reload() error = %v, want context canceled", err)
	}
	if openCalls != 2 {
		t.Fatalf("OpenAgentInput() calls = %d, want primary plus fallback", openCalls)
	}
	bos := &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "after-cancel", BeginOfStream: true},
	}
	if err := source.Push(context.Background(), bos); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	select {
	case eos := <-outputs:
		if eos == nil || !eos.IsEndOfStream() ||
			eos.Ctrl.StreamID != bos.Ctrl.StreamID ||
			eos.Ctrl.ErrorCode != agentReloadFailedCode {
			t.Fatalf("error EOS = %#v", eos)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error EOS")
	}
	if _, err := svc.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestServiceReloadActivateFailureClosesStreams(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	input := NewInputStream(1)
	output := newBlockingStream()
	store := fakePeerRunStore{
		selection: apitypes.AgentSelection{WorkspaceName: "demo"},
		err:       peerrun.ErrRunAgentChanged,
	}
	svc := &Service{
		Host:      &fakeHost{output: output},
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return input, nil
		}),
		Consumer: StreamConsumerFunc(func(context.Context, genx.Stream) error { return nil }),
	}
	if _, err := svc.Reload(ctx); !errors.Is(err, peerrun.ErrRunAgentChanged) {
		t.Fatalf("Reload() error = %v, want %v", err, peerrun.ErrRunAgentChanged)
	}
	if !input.closed() || !output.closed() {
		t.Fatalf("streams closed after activate failure: input=%v output=%v", input.closed(), output.closed())
	}
}

func TestServiceReloadTransformFailureInstallsErrorRuntime(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatal(err)
	}
	host := New(fakeResolver{spec: Spec{
		Workspace: apitypes.Workspace{Name: "demo"},
		AgentType: "reloadable",
	}})
	wantErr := errors.New("replacement transform failed")
	var mu sync.Mutex
	var agents []*closeTrackingAgent
	var closed []int
	if err := host.Register("reloadable", agentFactoryFunc(func(context.Context, Spec) (Agent, error) {
		mu.Lock()
		index := len(agents)
		closed = append(closed, 0)
		mu.Unlock()
		var transformer genx.Transformer
		if index != 1 {
			transformer = runtimeTransformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
				return newBlockingStream(), nil
			})
		} else {
			transformer = runtimeTransformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
				return nil, wantErr
			})
		}
		agent := &closeTrackingAgent{Agent: NewTransformerAgent(transformer)}
		agent.close = func() {
			mu.Lock()
			closed[index]++
			mu.Unlock()
		}
		mu.Lock()
		agents = append(agents, agent)
		mu.Unlock()
		return agent, nil
	})); err != nil {
		t.Fatal(err)
	}
	source := NewPushSource(4)
	outputs := make(chan *genx.MessageChunk, 4)
	svc := &Service{
		Host:      host,
		PeerRun:   store,
		PublicKey: publicKey,
		Source:    source,
		Consumer: StreamConsumerFunc(func(ctx context.Context, stream genx.Stream) error {
			for {
				chunk, err := stream.Next()
				if err != nil {
					if ctx.Err() != nil || IsStreamDone(err) || errors.Is(err, io.ErrClosedPipe) {
						return nil
					}
					return err
				}
				select {
				case outputs <- chunk:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}),
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("initial Reload() error = %v", err)
	}
	if _, err := svc.Reload(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("replacement Reload() error = %v, want %v", err, wantErr)
	}
	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != apitypes.PeerRunStatusStateError ||
		status.Message == nil || *status.Message != agentReloadFailedMessage {
		t.Fatalf("Status() = %+v, want reload error", status)
	}
	state, err := svc.WorkspaceState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.RuntimeState != apitypes.PeerRunStatusStateError || state.HistoryAvailable == nil || *state.HistoryAvailable {
		t.Fatalf("WorkspaceState() = %+v, want error/unavailable", state)
	}

	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "conversation-ignored"},
	}); err != nil {
		t.Fatalf("Push(chunk before BOS) error = %v", err)
	}
	select {
	case unexpected := <-outputs:
		t.Fatalf("chunk before BOS produced output: %#v", unexpected)
	case <-time.After(20 * time.Millisecond):
	}

	bos := &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{
			StreamID:      "conversation-1",
			Label:         "microphone",
			Timestamp:     123,
			BeginOfStream: true,
		},
	}
	if err := source.Push(ctx, bos); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	select {
	case eos := <-outputs:
		if eos == nil || !eos.IsEndOfStream() || eos.Ctrl.StreamID != bos.Ctrl.StreamID ||
			eos.Ctrl.Label != bos.Ctrl.Label || eos.Ctrl.Timestamp != bos.Ctrl.Timestamp ||
			eos.Ctrl.ErrorCode != agentReloadFailedCode || eos.Ctrl.Error == "" || eos.Ctrl.ErrorRetryable {
			t.Fatalf("error EOS = %#v", eos)
		}
		blob, ok := eos.Part.(*genx.Blob)
		if !ok || blob.MIMEType != "audio/opus" || len(blob.Data) != 0 {
			t.Fatalf("error EOS part = %#v, want empty audio/opus", eos.Part)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error EOS")
	}

	if err := source.Push(ctx, bos.Clone()); err != nil {
		t.Fatalf("Push(duplicate BOS) error = %v", err)
	}
	select {
	case duplicate := <-outputs:
		t.Fatalf("duplicate BOS produced output: %#v", duplicate)
	case <-time.After(20 * time.Millisecond):
	}

	secondBOS := bos.Clone()
	secondBOS.Ctrl.StreamID = "conversation-2"
	if err := source.Push(ctx, secondBOS); err != nil {
		t.Fatalf("Push(second BOS) error = %v", err)
	}
	select {
	case eos := <-outputs:
		if eos == nil || !eos.IsEndOfStream() ||
			eos.Ctrl.StreamID != secondBOS.Ctrl.StreamID ||
			eos.Ctrl.ErrorCode != agentReloadFailedCode {
			t.Fatalf("second error EOS = %#v", eos)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second error EOS")
	}

	mu.Lock()
	if closed[0] != 1 || closed[1] != 1 {
		mu.Unlock()
		t.Fatalf("close counts after failed replacement = %v, want [1 1]", closed)
	}
	mu.Unlock()
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() after fallback error = %v", err)
	}
	status, err = svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != apitypes.PeerRunStatusStateRunning {
		t.Fatalf("Status() after replacing fallback = %+v, want running", status)
	}
	if _, err := svc.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(closed) != 3 || !slices.Equal(closed, []int{1, 1, 1}) {
		t.Fatalf("close counts after replacing fallback = %v, want [1 1 1]", closed)
	}
}

func TestServiceFailedReplacementKeepsOtherPeerGenerationLive(t *testing.T) {
	ctx := context.Background()
	firstKey := testPublicKey(t)
	secondKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	for _, key := range []giznet.PublicKey{firstKey, secondKey} {
		if _, err := store.SetRunAgent(ctx, key, apitypes.AgentSelection{
			WorkspaceName: "demo",
		}); err != nil {
			t.Fatal(err)
		}
	}

	shared := &multiAttachAgent{}
	wantErr := errors.New("replacement transform failed")
	var mu sync.Mutex
	var closed []int
	host := New(fakeResolver{spec: Spec{
		Workspace: apitypes.Workspace{Name: "demo"},
		AgentType: "reloadable",
	}})
	if err := host.Register("reloadable", agentFactoryFunc(func(context.Context, Spec) (Agent, error) {
		mu.Lock()
		index := len(closed)
		closed = append(closed, 0)
		mu.Unlock()
		var agent Agent = shared
		if index != 0 {
			agent = NewTransformerAgent(runtimeTransformerFunc(
				func(context.Context, genx.Stream) (genx.Stream, error) {
					return nil, wantErr
				},
			))
		}
		return &closeTrackingAgent{
			Agent: agent,
			close: func() {
				mu.Lock()
				closed[index]++
				mu.Unlock()
			},
		}, nil
	})); err != nil {
		t.Fatal(err)
	}

	firstConsumer := newRecordingConsumer()
	secondConsumer := newRecordingConsumer()
	first := testService(t, firstKey, store, host)
	first.Consumer = firstConsumer
	second := testService(t, secondKey, store, host)
	second.Consumer = secondConsumer
	if _, err := first.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	_ = firstConsumer.nextText(t)
	_ = secondConsumer.nextText(t)

	if _, err := first.Reload(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("first Reload() error = %v, want %v", err, wantErr)
	}
	if err := shared.addOutput(1, "second-peer-still-live"); err != nil {
		t.Fatalf("add second Peer output: %v", err)
	}
	if got := secondConsumer.nextText(t); got != "second-peer-still-live" {
		t.Fatalf("second Peer output = %q", got)
	}
	mu.Lock()
	if !slices.Equal(closed, []int{0, 1}) {
		mu.Unlock()
		t.Fatalf("close counts after failed replacement = %v, want [0 1]", closed)
	}
	mu.Unlock()

	if _, err := first.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(closed, []int{1, 1}) {
		t.Fatalf("final close counts = %v, want [1 1]", closed)
	}
}

func TestServiceConsumerErrorSetsStatus(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	consumerErr := errors.New("consumer failed")
	done := make(chan struct{})
	hookCh := make(chan struct {
		workspace string
		err       error
	}, 1)
	svc := testService(t, publicKey, store, &fakeHost{output: &sliceStream{doneErr: genx.ErrDone}})
	svc.Consumer = StreamConsumerFunc(func(context.Context, genx.Stream) error {
		defer close(done)
		return consumerErr
	})
	svc.OnConsumerError = func(_ context.Context, workspace string, err error) {
		hookCh <- struct {
			workspace string
			err       error
		}{workspace: workspace, err: err}
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	<-done
	hook := <-hookCh
	if hook.workspace != "demo" || !errors.Is(hook.err, consumerErr) {
		t.Fatalf("OnConsumerError() = workspace=%q err=%v, want demo/%v", hook.workspace, hook.err, consumerErr)
	}
	deadline := time.After(time.Second)
	for {
		status, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}
		if status.State == apitypes.PeerRunStatusStateError && status.Message != nil && strings.Contains(*status.Message, "consumer failed") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("Status() after consumer error = %+v", status)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestServiceWorkspaceQuiescenceSetsStoppedStatus(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	done := make(chan struct{})
	hookCh := make(chan error, 1)
	svc := testService(t, publicKey, store, &fakeHost{output: &sliceStream{doneErr: genx.ErrDone}})
	svc.Consumer = StreamConsumerFunc(func(context.Context, genx.Stream) error {
		defer close(done)
		return errWorkspaceQuiesced
	})
	svc.OnConsumerError = func(_ context.Context, _ string, err error) {
		hookCh <- err
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	<-done
	deadline := time.After(time.Second)
	for {
		status, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}
		if status.State == apitypes.PeerRunStatusStateStopped {
			if status.WorkspaceName == nil || *status.WorkspaceName != "demo" {
				t.Fatalf("Status().WorkspaceName = %v, want demo", status.WorkspaceName)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("Status() after Workspace quiescence = %+v, want stopped", status)
		default:
			time.Sleep(time.Millisecond)
		}
	}
	select {
	case err := <-hookCh:
		t.Fatalf("OnConsumerError() error = %v, want no call", err)
	default:
	}
}

func TestServiceTreatsActiveOutputCompletionAsFailure(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	input := NewInputStream(1)
	output := newBlockingStream()
	done := make(chan struct{})
	hookCh := make(chan error, 1)
	svc := &Service{
		Host:      &fakeHost{output: output},
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return input, nil
		}),
		Consumer: StreamConsumerFunc(func(context.Context, genx.Stream) error {
			defer close(done)
			return nil
		}),
		OnConsumerError: func(_ context.Context, workspace string, err error) {
			if workspace != "demo" {
				t.Errorf("OnConsumerError() workspace = %q, want demo", workspace)
			}
			hookCh <- err
		},
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	<-done
	select {
	case err := <-hookCh:
		if !errors.Is(err, errUnexpectedOutputEnd) {
			t.Fatalf("OnConsumerError() error = %v, want %v", err, errUnexpectedOutputEnd)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for consumer error hook")
	}
	deadline := time.After(time.Second)
	for !input.closed() || !output.closed() {
		select {
		case <-deadline:
			t.Fatalf("runtime streams closed after output completion: input=%t output=%t", input.closed(), output.closed())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateError || status.Message == nil || !strings.Contains(*status.Message, errUnexpectedOutputEnd.Error()) {
		t.Fatalf("Status() after active output completion = %+v, want error", status)
	}
}

func TestServiceNamesLastRouteWhenOutputEndsWhileActive(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	input := NewInputStream(1)
	// The output ends cleanly (io.EOF) after a mid-turn assistant audio route,
	// exactly the shape of the reported failure where a stage closed the stream
	// without an error while the runtime was still active.
	output := &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleModel, Name: "narrator", Part: &genx.Blob{MIMEType: "audio/ogg"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Name: "narrator", Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2", Label: "assistant"}},
	}}
	hookCh := make(chan error, 1)
	svc := &Service{
		Host:      &fakeHost{output: output},
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return input, nil
		}),
		Consumer: StreamConsumerFunc(func(_ context.Context, stream genx.Stream) error {
			for {
				_, err := stream.Next()
				if err != nil {
					if IsStreamDone(err) {
						return nil
					}
					return err
				}
			}
		}),
		OnConsumerError: func(_ context.Context, _ string, err error) {
			hookCh <- err
		},
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	select {
	case err := <-hookCh:
		if !errors.Is(err, errUnexpectedOutputEnd) {
			t.Fatalf("OnConsumerError() error = %v, want %v", err, errUnexpectedOutputEnd)
		}
		message := err.Error()
		for _, want := range []string{`last_stream_id="turn-2"`, `last_label="assistant"`, `last_mime="audio/ogg"`, "chunks=2"} {
			if !strings.Contains(message, want) {
				t.Fatalf("diagnostic %q missing %q", message, want)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for consumer error hook")
	}
	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateError || status.Message == nil || !strings.Contains(*status.Message, `last_stream_id="turn-2"`) {
		t.Fatalf("Status() after active output completion = %+v, want route-named error", status)
	}
}

func TestServiceKeepsRuntimeAvailableForRepeatedHistoryReplayAfterRouteEOS(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	agent := &multiAttachAgent{}
	host := &runtimeTestOpenAgentHost{agent: agent}
	routeDone := make(chan struct{}, 1)
	svc := &Service{
		Host:      host,
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return NewInputStream(4), nil
		}),
		Consumer: StreamConsumerFunc(func(ctx context.Context, output genx.Stream) error {
			for {
				chunk, err := output.Next()
				if err != nil {
					if IsStreamDone(err) || errors.Is(err, io.ErrClosedPipe) {
						return nil
					}
					return err
				}
				if chunk != nil && chunk.IsEndOfStream() {
					select {
					case routeDone <- struct{}{}:
					default:
					}
				}
				if err := ctx.Err(); err != nil {
					return err
				}
			}
		}),
		Now: fixedClock(time.Unix(100, 0)),
	}
	status, err := svc.Reload(ctx)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if status.StartedAt == nil {
		t.Fatalf("Reload() status = %+v, want StartedAt", status)
	}
	startedAt := *status.StartedAt
	revision := svc.RuntimeRevision()

	agent.mu.Lock()
	output := agent.output[0]
	agent.mu.Unlock()
	for _, responseID := range []string{"response-a", "response-b", "response-c"} {
		if err := output.Add(&genx.MessageChunk{
			Role: genx.RoleModel,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: responseID, EndOfStream: true, Error: "interrupted"},
		}); err != nil {
			t.Fatalf("output.Add(%s route EOS) error = %v", responseID, err)
		}
		select {
		case <-routeDone:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for logical output route %s completion", responseID)
		}

		state, err := svc.WorkspaceState(ctx)
		if err != nil {
			t.Fatalf("WorkspaceState() after %s error = %v", responseID, err)
		}
		if state.RuntimeState != apitypes.PeerRunStatusStateRunning || state.StartedAt == nil || !state.StartedAt.Equal(startedAt) {
			t.Fatalf("WorkspaceState() after %s route EOS = %+v, want same running runtime", responseID, state)
		}
		if got := svc.RuntimeRevision(); got != revision {
			t.Fatalf("RuntimeRevision() after %s route EOS = %d, want %d", responseID, got, revision)
		}
	}
	for _, historyID := range []string{"h1", "h1"} {
		play, err := svc.PlayWorkspaceHistory(ctx, apitypes.PeerRunHistoryPlayRequest{HistoryName: historyID})
		if err != nil {
			t.Fatalf("PlayWorkspaceHistory(%s) error = %v", historyID, err)
		}
		if !play.Accepted {
			t.Fatalf("PlayWorkspaceHistory(%s) = %+v, want accepted", historyID, play)
		}
	}
	if got, want := agent.playGearIDs(), []string{publicKey.String(), publicKey.String()}; !slices.Equal(got, want) {
		t.Fatalf("PlayHistory gear IDs = %v, want %v", got, want)
	}
	if _, err := svc.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestMixerOutputConsumesPCMBlob(t *testing.T) {
	tracks := &fakeTracks{}
	output := &sliceStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("ignored")},
		{Part: &genx.Blob{MIMEType: "audio/L16; rate=16000; channels=1", Data: []byte{1, 0, 2, 0}}},
	}, doneErr: genx.ErrDone}
	if err := (MixerOutput{Tracks: tracks}).ConsumeAgentOutput(context.Background(), output); err != nil {
		t.Fatalf("ConsumeAgentOutput() error = %v", err)
	}
	if tracks.created != 1 || len(tracks.track.chunks) != 1 {
		t.Fatalf("tracks created=%d chunks=%d", tracks.created, len(tracks.track.chunks))
	}
	if tracks.track.chunks[0].Len() != 4 {
		t.Fatalf("chunk len = %d, want 4", tracks.track.chunks[0].Len())
	}
}

func TestMixerOutputErrors(t *testing.T) {
	if err := (MixerOutput{}).ConsumeAgentOutput(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "output stream") {
		t.Fatalf("ConsumeAgentOutput(nil) error = %v", err)
	}
	if err := (MixerOutput{}).ConsumeAgentOutput(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}},
	}}); err == nil || !strings.Contains(err.Error(), "audio track creator") {
		t.Fatalf("ConsumeAgentOutput(nil tracks) error = %v", err)
	}
	writeErr := errors.New("write failed")
	tracks := &fakeTracks{writeErr: writeErr}
	if err := (MixerOutput{Tracks: tracks}).ConsumeAgentOutput(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}},
	}}); !errors.Is(err, writeErr) {
		t.Fatalf("ConsumeAgentOutput(write error) error = %v, want %v", err, writeErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (MixerOutput{Tracks: &fakeTracks{}}).ConsumeAgentOutput(ctx, &sliceStream{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ConsumeAgentOutput(canceled) error = %v, want %v", err, context.Canceled)
	}
}

func TestInputStreamRejectsPushAfterClose(t *testing.T) {
	stream := NewInputStream(1)
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Push(context.Background(), &genx.MessageChunk{Part: genx.Text("late")}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Push(after close) error = %v, want %v", err, io.ErrClosedPipe)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("Next(after close) error = %v, want %v", err, io.EOF)
	}
}

func TestInputStreamPushNextAndNil(t *testing.T) {
	var nilStream *InputStream
	if err := nilStream.Push(context.Background(), &genx.MessageChunk{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("nil Push() error = %v, want %v", err, io.ErrClosedPipe)
	}
	if _, err := nilStream.Next(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("nil Next() error = %v, want %v", err, io.ErrClosedPipe)
	}
	if err := nilStream.CloseWithError(nil); err != nil {
		t.Fatalf("nil CloseWithError() error = %v", err)
	}

	stream := NewInputStream(0)
	chunk := &genx.MessageChunk{Part: genx.Text("hello")}
	if err := stream.Push(context.Background(), chunk); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	got, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got != chunk {
		t.Fatalf("Next() = %p, want %p", got, chunk)
	}
	wantErr := errors.New("closed")
	if err := stream.CloseWithError(wantErr); err != nil {
		t.Fatalf("CloseWithError() error = %v", err)
	}
	if _, err := stream.Next(); !errors.Is(err, wantErr) {
		t.Fatalf("Next(after CloseWithError) error = %v, want %v", err, wantErr)
	}
}

type blockingOpenSource struct {
	mu      sync.Mutex
	entered chan struct{}
	release chan struct{}
	calls   int
	once    sync.Once
}

type inputPusherFunc func(context.Context, *genx.MessageChunk) error

func (f inputPusherFunc) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	return f(ctx, chunk)
}

type blockingSelectionStore struct {
	selection  apitypes.AgentSelection
	setEntered chan struct{}
	setRelease chan struct{}
	once       sync.Once
}

type blockingActivateStore struct {
	PeerRunStore
	activateEntered chan struct{}
	activateRelease chan struct{}
	once            sync.Once
}

func newBlockingActivateStore(store PeerRunStore) *blockingActivateStore {
	return &blockingActivateStore{
		PeerRunStore:    store,
		activateEntered: make(chan struct{}),
		activateRelease: make(chan struct{}),
	}
}

func (s *blockingActivateStore) ActivateRunAgent(_ context.Context, publicKey giznet.PublicKey, selection apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	s.once.Do(func() { close(s.activateEntered) })
	<-s.activateRelease
	return s.PeerRunStore.ActivateRunAgent(context.Background(), publicKey, selection)
}

func newBlockingSelectionStore() *blockingSelectionStore {
	return &blockingSelectionStore{setEntered: make(chan struct{}), setRelease: make(chan struct{})}
}

func (s *blockingSelectionStore) GetRunAgent(context.Context, giznet.PublicKey) (apitypes.PeerRunAgent, error) {
	return apitypes.PeerRunAgent{}, nil
}

func (s *blockingSelectionStore) SetRunAgent(_ context.Context, _ giznet.PublicKey, selection apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	s.once.Do(func() { close(s.setEntered) })
	<-s.setRelease
	s.selection = selection
	return apitypes.PeerRunAgent{Pending: &selection}, nil
}

func (s *blockingSelectionStore) ResolveRunAgent(context.Context, giznet.PublicKey) (apitypes.AgentSelection, error) {
	return s.selection, nil
}

func (s *blockingSelectionStore) ActivateRunAgent(context.Context, giznet.PublicKey, apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	return apitypes.PeerRunAgent{}, nil
}

func newBlockingOpenSource() *blockingOpenSource {
	return &blockingOpenSource{entered: make(chan struct{}), release: make(chan struct{})}
}

func (s *blockingOpenSource) OpenAgentInput(context.Context) (genx.Stream, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 1 {
		s.once.Do(func() { close(s.entered) })
		<-s.release
	}
	return NewInputStream(1), nil
}

func (s *blockingOpenSource) openCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func testService(t *testing.T, publicKey giznet.PublicKey, store *peerrun.Server, host genx.TransformerMux) *Service {
	t.Helper()
	return &Service{
		Host:      host,
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return NewInputStream(1), nil
		}),
		Consumer: StreamConsumerFunc(func(context.Context, genx.Stream) error { return nil }),
		Now:      fixedClock(time.Unix(100, 0)),
	}
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func testPublicKey(t *testing.T) giznet.PublicKey {
	t.Helper()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	return keyPair.Public
}

type fakeHost struct {
	pattern string
	output  genx.Stream
	err     error
}

func (h *fakeHost) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	h.pattern = pattern
	if h.err != nil {
		return nil, h.err
	}
	if input == nil {
		return nil, errors.New("input required")
	}
	if h.output != nil {
		return h.output, nil
	}
	return &sliceStream{doneErr: genx.ErrDone}, nil
}

type nilOutputHost struct{}

func (nilOutputHost) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return nil, nil
}

type fakePeerRunStore struct {
	selection apitypes.AgentSelection
	err       error
}

type selectionErrorStore struct {
	PeerRunStore
	run apitypes.PeerRunAgent
	err error
}

func (s selectionErrorStore) GetRunAgent(context.Context, giznet.PublicKey) (apitypes.PeerRunAgent, error) {
	return s.run, nil
}

func (s selectionErrorStore) SetRunAgent(context.Context, giznet.PublicKey, apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	return apitypes.PeerRunAgent{}, s.err
}

func (s fakePeerRunStore) ResolveRunAgent(context.Context, giznet.PublicKey) (apitypes.AgentSelection, error) {
	return s.selection, nil
}

func (s fakePeerRunStore) ActivateRunAgent(context.Context, giznet.PublicKey, apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	return apitypes.PeerRunAgent{}, s.err
}

type runtimeTestOpenAgentHost struct {
	agent        Agent
	pattern      string
	releaseCalls int
}

func (h *runtimeTestOpenAgentHost) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return nil, errors.New("unexpected Transform call")
}

func (h *runtimeTestOpenAgentHost) OpenAgent(_ context.Context, pattern string) (Agent, func(), error) {
	h.pattern = pattern
	return h.agent, func() {
		h.releaseCalls++
	}, nil
}

type runtimeTestAgent struct {
	output genx.Stream
	state  apitypes.PeerRunWorkspaceState
}

type runtimeTransformerFunc func(context.Context, genx.Stream) (genx.Stream, error)

func (transformer runtimeTransformerFunc) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return transformer(ctx, input)
}

func (a *runtimeTestAgent) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	if input == nil {
		return nil, errors.New("input required")
	}
	return a.output, nil
}

func (a *runtimeTestAgent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	return a.state, nil
}

func (a *runtimeTestAgent) ListHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return apitypes.PeerRunHistoryListResponse{}, nil
}

func (a *runtimeTestAgent) PlayHistory(context.Context, apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	return apitypes.PeerRunHistoryPlayResponse{}, nil
}

func (a *runtimeTestAgent) MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	return apitypes.PeerRunMemoryStatsResponse{}, nil
}

func (a *runtimeTestAgent) Recall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	return apitypes.PeerRunRecallResponse{}, nil
}

type multiAttachAgent struct {
	mu        sync.Mutex
	calls     int
	playGears []string
	output    []*genx.StreamBuilder
}

func (a *multiAttachAgent) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if input == nil {
		return nil, errors.New("input required")
	}
	sb := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	if err := sb.Add(&genx.MessageChunk{Part: genx.Text(historyGearID(ctx))}); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.calls++
	a.output = append(a.output, sb)
	a.mu.Unlock()
	return sb.Stream(), nil
}

func (a *multiAttachAgent) transformCalls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func (a *multiAttachAgent) addOutput(index int, text string) error {
	a.mu.Lock()
	if index < 0 || index >= len(a.output) {
		a.mu.Unlock()
		return fmt.Errorf("output index %d out of range", index)
	}
	output := a.output[index]
	a.mu.Unlock()
	return output.Add(&genx.MessageChunk{Part: genx.Text(text)})
}

func (a *multiAttachAgent) playGearIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.playGears...)
}

func (a *multiAttachAgent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	return apitypes.PeerRunWorkspaceState{}, nil
}

func (a *multiAttachAgent) ListHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return apitypes.PeerRunHistoryListResponse{}, nil
}

func (a *multiAttachAgent) PlayHistory(ctx context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	a.mu.Lock()
	a.playGears = append(a.playGears, historyGearID(ctx))
	a.mu.Unlock()
	return apitypes.PeerRunHistoryPlayResponse{Accepted: true, HistoryName: req.HistoryName, State: "played"}, nil
}

func (a *multiAttachAgent) MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	return apitypes.PeerRunMemoryStatsResponse{}, nil
}

func (a *multiAttachAgent) Recall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	return apitypes.PeerRunRecallResponse{}, nil
}

type recordingConsumer struct {
	ch chan string
}

func newRecordingConsumer() *recordingConsumer {
	return &recordingConsumer{ch: make(chan string, 4)}
}

func (c *recordingConsumer) ConsumeAgentOutput(ctx context.Context, stream genx.Stream) error {
	for {
		chunk, err := stream.Next()
		if err != nil {
			if IsStreamDone(err) || errors.Is(err, io.ErrClosedPipe) {
				return nil
			}
			return err
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			select {
			case c.ch <- string(text):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (c *recordingConsumer) nextText(t *testing.T) string {
	t.Helper()
	select {
	case text := <-c.ch:
		return text
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for output text")
		return ""
	}
}

type blockingStream struct {
	done chan struct{}
	once sync.Once
}

func newBlockingStream() *blockingStream {
	return &blockingStream{done: make(chan struct{})}
}

func (s *blockingStream) Next() (*genx.MessageChunk, error) {
	<-s.done
	return nil, io.ErrClosedPipe
}

func (s *blockingStream) Close() error {
	return s.CloseWithError(io.ErrClosedPipe)
}

func (s *blockingStream) CloseWithError(error) error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func (s *blockingStream) closed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

type sliceStream struct {
	chunks  []*genx.MessageChunk
	doneErr error
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		if s.doneErr != nil {
			return nil, s.doneErr
		}
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *sliceStream) Close() error {
	return nil
}

func (s *sliceStream) CloseWithError(error) error {
	return nil
}

type fakeTracks struct {
	created  int
	track    *fakeTrack
	writeErr error
	mixer    *pcm.Mixer
}

func (t *fakeTracks) CreateAudioTrack(...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	t.created++
	t.track = &fakeTrack{err: t.writeErr}
	if t.mixer == nil {
		t.mixer = pcm.NewMixer(pcm.L16Mono16K)
	}
	_, ctrl, err := t.mixer.CreateTrack()
	return t.track, ctrl, err
}

type fakeTrack struct {
	chunks []pcm.Chunk
	err    error
}

func (t *fakeTrack) Write(chunk pcm.Chunk) error {
	if t.err != nil {
		return t.err
	}
	t.chunks = append(t.chunks, chunk)
	return nil
}
