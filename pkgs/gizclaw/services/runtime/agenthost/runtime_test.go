package agenthost

import (
	"context"
	"errors"
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

func TestServiceReloadMissingWorkspaceKeepsPending(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "missing"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	wantErr := errors.New("agenthost: workspace \"missing\" not found")
	svc := testService(t, publicKey, store, &fakeHost{err: wantErr})
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
	if status.State != apitypes.PeerRunStatusStateError || status.Message == nil || !strings.Contains(*status.Message, "workspace") {
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

	play, err := svc.PlayWorkspaceHistory(ctx, apitypes.PeerRunHistoryPlayRequest{HistoryId: "h1"})
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
	play, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: "h1"})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if play.Accepted || play.HistoryId != "h1" || play.State != "unsupported" || play.Message == nil || !strings.Contains(*play.Message, unsupportedMessage) {
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
	if _, err := first.PlayWorkspaceHistory(ctx, apitypes.PeerRunHistoryPlayRequest{HistoryId: "h1"}); err != nil {
		t.Fatalf("first PlayWorkspaceHistory() error = %v", err)
	}
	if _, err := second.PlayWorkspaceHistory(ctx, apitypes.PeerRunHistoryPlayRequest{HistoryId: "h2"}); err != nil {
		t.Fatalf("second PlayWorkspaceHistory() error = %v", err)
	}
	if got, want := agent.playGearIDs(), []string{firstKey.String(), secondKey.String()}; !slices.Equal(got, want) {
		t.Fatalf("PlayHistory gear IDs = %v, want %v", got, want)
	}

	if _, err := first.Stop(ctx); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if _, err := host.coordinator().Acquire(ctx, "demo"); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("Acquire after first stop error = %v, want %v", err, ErrWorkspaceBusy)
	}
	if _, err := second.Stop(ctx); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
	lease, err := host.coordinator().Acquire(ctx, "demo")
	if err != nil {
		t.Fatalf("Acquire after both stops error = %v", err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("Release after both stops error = %v", err)
	}
}

func TestRuntimeKeyIsolatesOwnerlessRuntimeProfiles(t *testing.T) {
	spec := Spec{Workspace: apitypes.Workspace{Name: "system"}}
	first := WithResourceAccess(t.Context(), "peer-a", nil, nil, "profile-a")
	second := WithResourceAccess(t.Context(), "peer-b", nil, nil, "profile-b")
	if runtimeKey(first, "system", spec) == runtimeKey(second, "system", spec) {
		t.Fatal("ownerless workspace reused a different caller RuntimeProfile")
	}
	owner := "owner"
	spec.Workspace.OwnerPublicKey = &owner
	if runtimeKey(first, "system", spec) != runtimeKey(second, "system", spec) {
		t.Fatal("owned workspace did not share the owner's runtime")
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
	svc.Source = StreamSourceFunc(func(context.Context) (genx.Stream, error) {
		return nil, sourceErr
	})
	if _, err := svc.Reload(ctx); !errors.Is(err, sourceErr) {
		t.Fatalf("Reload(source error) error = %v, want %v", err, sourceErr)
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

func TestServiceClosesInputWhenOutputEnds(t *testing.T) {
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	input := NewInputStream(1)
	done := make(chan struct{})
	svc := &Service{
		Host:      &fakeHost{output: &sliceStream{doneErr: io.EOF}},
		PeerRun:   store,
		PublicKey: publicKey,
		Source: StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return input, nil
		}),
		Consumer: StreamConsumerFunc(func(context.Context, genx.Stream) error {
			defer close(done)
			return nil
		}),
	}
	if _, err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	<-done
	deadline := time.After(time.Second)
	for !input.closed() {
		select {
		case <-deadline:
			t.Fatal("input stream was not closed after output ended")
		default:
			time.Sleep(time.Millisecond)
		}
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
	return apitypes.PeerRunHistoryPlayResponse{Accepted: true, HistoryId: req.HistoryId, State: "played"}, nil
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
