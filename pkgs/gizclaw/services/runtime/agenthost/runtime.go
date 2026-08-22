package agenthost

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var (
	ErrNilService            = errors.New("agenthost: nil service")
	ErrMissingHost           = errors.New("agenthost: host is required")
	ErrMissingPeerRun        = errors.New("agenthost: peer run store is required")
	ErrMissingSource         = errors.New("agenthost: stream source is required")
	ErrMissingInputPusher    = errors.New("agenthost: input pusher is required")
	ErrMissingConsumer       = errors.New("agenthost: stream consumer is required")
	ErrInvalidPublicKey      = errors.New("agenthost: invalid public key")
	ErrNoActiveWorkspace     = errors.New("agenthost: no active workspace")
	ErrServiceClosed         = errors.New("agenthost: service is closed")
	ErrMissingSelectionStore = errors.New("agenthost: peer run selection store is required")
	errUnexpectedOutputEnd   = errors.New("agenthost: output consumer completed while runtime remained active")
)

type PeerRunStore interface {
	ResolveRunAgent(context.Context, giznet.PublicKey) (apitypes.AgentSelection, error)
	ActivateRunAgent(context.Context, giznet.PublicKey, apitypes.AgentSelection) (apitypes.PeerRunAgent, error)
}

// PeerRunSelectionStore is the optional selection persistence capability used
// by SetRunAgent and revision-aware input recovery.
type PeerRunSelectionStore interface {
	GetRunAgent(context.Context, giznet.PublicKey) (apitypes.PeerRunAgent, error)
	SetRunAgent(context.Context, giznet.PublicKey, apitypes.AgentSelection) (apitypes.PeerRunAgent, error)
}

type StreamSource interface {
	OpenAgentInput(context.Context) (genx.Stream, error)
}

// InputPusher writes a connection-scoped input chunk to the active source.
type InputPusher interface {
	Push(context.Context, *genx.MessageChunk) error
}

type StreamSourceFunc func(context.Context) (genx.Stream, error)

func (f StreamSourceFunc) OpenAgentInput(ctx context.Context) (genx.Stream, error) {
	return f(ctx)
}

type StreamConsumer interface {
	ConsumeAgentOutput(context.Context, genx.Stream) error
}

type StreamConsumerFunc func(context.Context, genx.Stream) error

func (f StreamConsumerFunc) ConsumeAgentOutput(ctx context.Context, stream genx.Stream) error {
	return f(ctx, stream)
}

type WorkspaceSelectionValidatorFunc func(context.Context, string) (string, error)

type Service struct {
	Host                       genx.TransformerMux
	PeerRun                    PeerRunStore
	RuntimeProfile             func() *apitypes.RuntimeProfile
	ClientTools                ClientToolInvoker
	ValidateWorkspaceSelection WorkspaceSelectionValidatorFunc
	AllowRestrictedReload      func(context.Context, string) bool
	PublicKey                  giznet.PublicKey
	Source                     StreamSource
	Consumer                   StreamConsumer
	OnConsumerError            func(context.Context, string, error)
	OnWorkspaceActivated       func(context.Context, string)
	OnWorkspaceHistoryUpdated  func(context.Context, string, workspace.HistoryEntry)
	Logger                     *slog.Logger
	Now                        func() time.Time

	transitionGateOnce sync.Once
	transitionGate     chan struct{}
	transitionCancelMu sync.Mutex
	transitionCancel   context.CancelFunc
	lifecycleOnce      sync.Once
	lifecycleCtx       context.Context
	lifecycleCancel    context.CancelFunc
	revision           atomic.Uint64

	mu      sync.Mutex
	closed  bool
	runtime *runtime
	status  apitypes.PeerRunStatus
}

func (s *Service) Reload(ctx context.Context) (apitypes.PeerRunStatus, error) {
	if s == nil {
		return apitypes.PeerRunStatus{}, ErrNilService
	}
	if err := s.validate(); err != nil {
		return s.setErrorStatus("", err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.lockTransition(ctx); err != nil {
		status, statusErr := s.Status(context.Background())
		return status, errors.Join(err, statusErr)
	}
	defer s.unlockTransition()
	ctx, finish := s.beginCancellableTransition(ctx)
	defer finish()
	return s.reload(ctx)
}

func (s *Service) reload(ctx context.Context) (apitypes.PeerRunStatus, error) {
	s.beginTransition()
	defer s.finishTransition()

	previous := s.swap(nil)
	workspaceName := ""
	if previous != nil {
		workspaceName = previous.workspace
		s.setStatus(apitypes.PeerRunStatusStateStarting, workspaceName, nil, nil)
		if err := previous.stop(ctx); err != nil {
			return s.reloadFailure(ctx, workspaceName, fmt.Errorf("agenthost: stop previous runtime: %w", err))
		}
	}

	selection, err := s.PeerRun.ResolveRunAgent(ctx, s.PublicKey)
	if err != nil {
		return s.reloadFailure(ctx, workspaceName, err)
	}
	workspaceName = selection.WorkspaceName
	if err := ctx.Err(); err != nil {
		return s.reloadFailure(ctx, workspaceName, err)
	}
	if s.ValidateWorkspaceSelection != nil {
		canonicalName, err := s.ValidateWorkspaceSelection(ctx, selection.WorkspaceName)
		if err != nil {
			if s.AllowRestrictedReload == nil || !s.AllowRestrictedReload(ctx, selection.WorkspaceName) {
				return s.reloadFailure(ctx, workspaceName, err)
			}
		} else {
			selection.WorkspaceName = canonicalName
			workspaceName = canonicalName
		}
	}
	s.setStatus(apitypes.PeerRunStatusStateStarting, workspaceName, nil, nil)

	input, err := s.Source.OpenAgentInput(ctx)
	if err != nil {
		// No input route exists for a fallback to receive BOS, so do not retry.
		return s.setErrorStatus(selection.WorkspaceName, fmt.Errorf("agenthost: open input stream: %w", err)), err
	}
	if input == nil {
		err := errors.New("agenthost: input stream is required")
		return s.setErrorStatus(selection.WorkspaceName, err), err
	}
	if err := ctx.Err(); err != nil {
		_ = input.CloseWithError(err)
		return s.reloadFailure(ctx, workspaceName, err)
	}
	profileToolBindings := map[string]string{}
	profileWorkflowBindings := map[string]string{}
	profileFingerprint := ""
	var profileSnapshot *apitypes.RuntimeProfile
	if s.RuntimeProfile != nil {
		if profile := s.RuntimeProfile(); profile != nil {
			profileToolBindings = runtimeProfileToolBindings(profile.Spec.Resources.Tools)
			profileWorkflowBindings = runtimeProfileWorkflowBindings(*profile)
			profileFingerprint = runtimeProfileFingerprint(*profile)
			snapshot := *profile
			profileSnapshot = &snapshot
		}
	}
	baseCtx := WithResourceAccess(withHistoryGearID(context.WithoutCancel(ctx), s.PublicKey.String()), s.PublicKey.String(), profileToolBindings, profileWorkflowBindings, profileFingerprint)
	if profileSnapshot != nil {
		baseCtx = withRuntimeProfile(baseCtx, *profileSnapshot)
	}
	baseCtx, err = WithToolExecution(baseCtx, profileTools(profileSnapshot), s.ClientTools)
	if err != nil {
		_ = input.CloseWithError(err)
		return s.reloadFailure(ctx, workspaceName, err)
	}
	baseCtx = withWorkspaceHistoryNotifier(baseCtx, s.OnWorkspaceHistoryUpdated)
	runCtx, runCancel := context.WithCancel(baseCtx)
	stopTransitionCancel := context.AfterFunc(ctx, runCancel)
	stopLifecycleCancel := context.AfterFunc(s.lifecycleContext(), runCancel)
	cancel := func() {
		stopTransitionCancel()
		stopLifecycleCancel()
		runCancel()
	}
	pattern := workspacePattern(selection.WorkspaceName)
	replaceGeneration := previous != nil && previous.workspace == selection.WorkspaceName
	agent, release, output, commit, err := s.openAgentOutput(runCtx, pattern, input, replaceGeneration)
	if err != nil {
		cancel()
		_ = input.CloseWithError(err)
		return s.reloadFailure(ctx, workspaceName, err)
	}
	if output == nil {
		cancel()
		if release != nil {
			release()
		}
		_ = input.Close()
		err := errors.New("agenthost: output stream is required")
		return s.reloadFailure(ctx, workspaceName, err)
	}
	if err := ctx.Err(); err != nil {
		cancel()
		if release != nil {
			release()
		}
		_ = errors.Join(output.CloseWithError(err), input.CloseWithError(err))
		return s.reloadFailure(ctx, workspaceName, err)
	}
	if _, err := s.PeerRun.ActivateRunAgent(ctx, s.PublicKey, selection); err != nil {
		cancel()
		if release != nil {
			release()
		}
		_ = errors.Join(output.CloseWithError(err), input.CloseWithError(err))
		return s.reloadFailure(ctx, workspaceName, err)
	}
	transitionDetached := stopTransitionCancel()
	if !transitionDetached || ctx.Err() != nil || runCtx.Err() != nil {
		err := ctx.Err()
		if err == nil {
			err = context.Cause(runCtx)
		}
		if err == nil {
			err = context.Canceled
		}
		if s.isClosed() {
			err = ErrServiceClosed
		}
		cancel()
		if release != nil {
			release()
		}
		_ = errors.Join(output.CloseWithError(err), input.CloseWithError(err))
		return s.reloadFailure(ctx, workspaceName, err)
	}

	now := s.now()
	next := &runtime{
		cancel:    cancel,
		agent:     agent,
		input:     input,
		output:    output,
		release:   release,
		done:      make(chan struct{}),
		workspace: selection.WorkspaceName,
		startedAt: now,
	}
	status, published, err := s.publishWithCommit(next, now, commit)
	if !published {
		cancel()
		if release != nil {
			release()
		}
		if err == nil {
			err = ErrServiceClosed
		}
		_ = errors.Join(output.CloseWithError(err), input.CloseWithError(err))
		return s.reloadFailure(ctx, workspaceName, err)
	}
	go s.consume(runCtx, next)
	if s.OnWorkspaceActivated != nil {
		s.OnWorkspaceActivated(runCtx, selection.WorkspaceName)
	}
	return status, nil
}

func (s *Service) reloadFailure(ctx context.Context, workspaceName string, cause error) (apitypes.PeerRunStatus, error) {
	status := s.setErrorStatus(workspaceName, cause)
	if s.isClosed() {
		return status, cause
	}
	fallbackStatus, err := s.installReloadErrorRuntime(ctx, workspaceName, cause)
	if err != nil {
		s.logger().Error("agenthost: install reload error runtime", "workspace", workspaceName, "error", err)
		return status, cause
	}
	return fallbackStatus, cause
}

func (s *Service) installReloadErrorRuntime(ctx context.Context, workspaceName string, cause error) (apitypes.PeerRunStatus, error) {
	runCtx, runCancel := context.WithCancel(context.WithoutCancel(ctx))
	stopLifecycleCancel := context.AfterFunc(s.lifecycleContext(), runCancel)
	cancel := func() {
		stopLifecycleCancel()
		runCancel()
	}
	input, err := s.Source.OpenAgentInput(runCtx)
	if err != nil {
		cancel()
		return s.setErrorStatus(workspaceName, cause), fmt.Errorf("agenthost: open reload error input: %w", err)
	}
	if input == nil {
		cancel()
		return s.setErrorStatus(workspaceName, cause), errors.New("agenthost: reload error input stream is required")
	}

	agent := newReloadErrorAgent()
	output, err := agent.Transform(runCtx, input)
	if err != nil {
		cancel()
		_ = input.CloseWithError(err)
		return s.setErrorStatus(workspaceName, cause), err
	}
	if output == nil {
		cancel()
		_ = input.Close()
		return s.setErrorStatus(workspaceName, cause), errors.New("agenthost: reload error output stream is required")
	}
	now := s.now()
	next := &runtime{
		cancel:    cancel,
		agent:     agent,
		input:     input,
		output:    output,
		done:      make(chan struct{}),
		workspace: workspaceName,
		startedAt: now,
		failure:   cause,
	}
	status, published := s.publishFailure(next, now)
	if !published {
		cancel()
		_ = errors.Join(output.CloseWithError(ErrServiceClosed), input.CloseWithError(ErrServiceClosed))
		return status, ErrServiceClosed
	}
	go s.consume(runCtx, next)
	return status, nil
}

func profileTools(profile *apitypes.RuntimeProfile) *map[string]apitypes.RuntimeProfileBinding {
	if profile == nil {
		return nil
	}
	return profile.Spec.Resources.Tools
}

// SetRunAgent persists a pending selection without allowing input recovery to
// observe a workspace-changing selection transition. Repeating the active
// workspace remains serialized but keeps its stable revision, so recovery can
// restore an inactive input source for that workspace.
func (s *Service) SetRunAgent(ctx context.Context, selection apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	if s == nil {
		return apitypes.PeerRunAgent{}, ErrNilService
	}
	if err := s.validateRunSelection(); err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	store, ok := s.PeerRun.(PeerRunSelectionStore)
	if !ok {
		return apitypes.PeerRunAgent{}, ErrMissingSelectionStore
	}
	if err := s.lockTransition(ctx); err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	defer s.unlockTransition()
	ctx, finish := s.beginCancellableTransition(ctx)
	defer finish()
	run, err := store.GetRunAgent(ctx, s.PublicKey)
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	if s.activeWorkspace(run) == selection.WorkspaceName {
		return store.SetRunAgent(ctx, s.PublicKey, selection)
	}
	updated, err := store.SetRunAgent(ctx, s.PublicKey, selection)
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	s.beginTransition()
	s.finishTransition()
	return updated, nil
}

// RuntimeRevision returns the current Peer runtime control-plane revision.
// Even revisions are stable; odd revisions have a selection, reload, or stop
// transition in progress.
func (s *Service) RuntimeRevision() uint64 {
	if s == nil {
		return 0
	}
	return s.revision.Load()
}

// PushInputIfCurrentRevision writes an input chunk only while the caller's
// observed revision is still stable. It keeps the push inside the same
// transition boundary as selection, reload, and stop so a completed transition
// cannot redirect an already-observed chunk to a new runtime.
func (s *Service) PushInputIfCurrentRevision(ctx context.Context, revision uint64, input InputPusher, chunk *genx.MessageChunk) (bool, error) {
	if s == nil {
		return false, ErrNilService
	}
	if input == nil {
		return false, ErrMissingInputPusher
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.lockTransition(ctx); err != nil {
		return false, err
	}
	defer s.unlockTransition()
	ctx, finish := s.beginCancellableTransition(ctx)
	defer finish()
	if revision%2 != 0 || s.revision.Load() != revision {
		return false, nil
	}
	return true, input.Push(ctx, chunk)
}

// PushInput writes an input chunk through the transition gate and reports the
// stable revision that owned the write. Sampling inside the gate gives queued
// input and control-plane transitions one serialization point.
func (s *Service) PushInput(ctx context.Context, input InputPusher, chunk *genx.MessageChunk) (uint64, bool, error) {
	if s == nil {
		return 0, false, ErrNilService
	}
	if input == nil {
		return 0, false, ErrMissingInputPusher
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.lockTransition(ctx); err != nil {
		return 0, false, err
	}
	defer s.unlockTransition()
	ctx, finish := s.beginCancellableTransition(ctx)
	defer finish()
	revision := s.revision.Load()
	if revision%2 != 0 {
		return revision, false, nil
	}
	return revision, true, input.Push(ctx, chunk)
}

// ReloadIfCurrentRevision recovers a missing input only when the caller saw
// the same stable runtime revision before its failed push. A changed revision
// means the chunk belongs to a superseded runtime and must be dropped.
func (s *Service) ReloadIfCurrentRevision(ctx context.Context, revision uint64) (bool, error) {
	if s == nil {
		return false, ErrNilService
	}
	if err := s.validate(); err != nil {
		return false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.lockTransition(ctx); err != nil {
		return false, err
	}
	defer s.unlockTransition()
	ctx, finish := s.beginCancellableTransition(ctx)
	defer finish()
	return s.reloadIfCurrentRevision(ctx, revision)
}

// ReloadAndPushInputIfCurrentRevision restores a missing input and writes the
// original chunk while one transition boundary remains held. The retry cannot
// be redirected to a later workspace after recovery completes.
func (s *Service) ReloadAndPushInputIfCurrentRevision(ctx context.Context, revision uint64, input InputPusher, chunk *genx.MessageChunk) (bool, error) {
	if s == nil {
		return false, ErrNilService
	}
	if input == nil {
		return false, ErrMissingInputPusher
	}
	if err := s.validate(); err != nil {
		return false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.lockTransition(ctx); err != nil {
		return false, err
	}
	defer s.unlockTransition()
	ctx, finish := s.beginCancellableTransition(ctx)
	defer finish()
	reloaded, err := s.reloadIfCurrentRevision(ctx, revision)
	if !reloaded || err != nil {
		return reloaded, err
	}
	return true, input.Push(ctx, chunk)
}

func (s *Service) reloadIfCurrentRevision(ctx context.Context, revision uint64) (bool, error) {
	if revision%2 != 0 || s.revision.Load() != revision {
		return false, nil
	}
	store, ok := s.PeerRun.(PeerRunSelectionStore)
	if !ok {
		return false, ErrMissingSelectionStore
	}
	run, err := store.GetRunAgent(ctx, s.PublicKey)
	if err != nil {
		return false, err
	}
	if s.pendingSelectionChangesRuntime(run) {
		return false, nil
	}
	if _, err := s.reload(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) pendingSelectionChangesRuntime(run apitypes.PeerRunAgent) bool {
	if run.Pending == nil {
		return false
	}
	workspace := s.activeWorkspace(run)
	return workspace != "" && workspace != run.Pending.WorkspaceName
}

func (s *Service) activeWorkspace(run apitypes.PeerRunAgent) string {
	if rt := s.currentRuntime(); rt != nil {
		return rt.workspace
	}
	if run.Active != nil {
		return run.Active.WorkspaceName
	}
	return ""
}

func runtimeProfileFingerprint(profile apitypes.RuntimeProfile) string {
	data, err := json.Marshal(profile)
	if err != nil {
		return profile.Id
	}
	digest := sha256.Sum256(data)
	return fmt.Sprintf("%x", digest[:16])
}

func runtimeProfileWorkflowBindings(profile apitypes.RuntimeProfile) map[string]string {
	bindings := make(map[string]string)
	for _, workflows := range profile.Spec.Workflows.Collections {
		for alias, binding := range workflows {
			bindings[alias] = binding.ResourceId
		}
	}
	return bindings
}

func runtimeProfileToolBindings(tools *map[string]apitypes.RuntimeProfileBinding) map[string]string {
	if tools == nil {
		return map[string]string{}
	}
	bindings := make(map[string]string, len(*tools))
	for alias, binding := range *tools {
		bindings[alias] = binding.ResourceId
	}
	return bindings
}

type agentOpener interface {
	OpenAgent(context.Context, string) (Agent, func(), error)
}

type agentReloader interface {
	ReloadAgent(context.Context, string) (Agent, func(), error)
}

type agentReloadPreparer interface {
	PrepareReloadAgent(context.Context, string) (*preparedAgentReplacement, error)
}

func (s *Service) openAgentOutput(ctx context.Context, pattern string, input genx.Stream, replace bool) (Agent, func(), genx.Stream, func() error, error) {
	if opener, ok := s.Host.(agentOpener); ok {
		var agent Agent
		var release func()
		commit := func() error { return nil }
		var err error
		if preparer, ok := s.Host.(agentReloadPreparer); replace && ok {
			replacement, prepareErr := preparer.PrepareReloadAgent(ctx, pattern)
			if prepareErr != nil {
				return nil, nil, nil, nil, prepareErr
			}
			agent = replacement.Agent()
			release = replacement.Release
			commit = replacement.Commit
		} else if reloader, ok := s.Host.(agentReloader); replace && ok {
			agent, release, err = reloader.ReloadAgent(ctx, pattern)
		} else {
			agent, release, err = opener.OpenAgent(ctx, pattern)
		}
		if err != nil {
			return nil, nil, nil, nil, err
		}
		output, err := agent.Transform(ctx, input)
		if err != nil {
			if release != nil {
				release()
			}
			return nil, nil, nil, nil, err
		}
		return agent, release, output, commit, nil
	}
	output, err := s.Host.Transform(ctx, pattern, input)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return asAgent(boundTransformer{mux: s.Host, pattern: pattern}), nil, output, func() error { return nil }, nil
}

type boundTransformer struct {
	mux     genx.TransformerMux
	pattern string
}

func (t boundTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return t.mux.Transform(ctx, t.pattern, input)
}

func (s *Service) Status(context.Context) (apitypes.PeerRunStatus, error) {
	if s == nil {
		return apitypes.PeerRunStatus{}, ErrNilService
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime != nil {
		if s.runtime.failure != nil {
			return s.status, nil
		}
		return runningStatus(s.runtime.workspace, s.runtime.startedAt, s.now()), nil
	}
	if s.status.State == "" {
		return stoppedStatus(s.now()), nil
	}
	return s.status, nil
}

func (s *Service) Stop(ctx context.Context) (apitypes.PeerRunStatus, error) {
	if s == nil {
		return stoppedStatus(time.Now()), nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.lockTransition(ctx); err != nil {
		status, statusErr := s.Status(context.Background())
		return status, errors.Join(err, statusErr)
	}
	defer s.unlockTransition()
	ctx, finish := s.beginCancellableTransition(ctx)
	defer finish()
	s.beginTransition()
	defer s.finishTransition()
	current := s.swap(nil)
	if current == nil {
		return s.setStatus(apitypes.PeerRunStatusStateStopped, "", nil, nil), nil
	}
	s.setStatus(apitypes.PeerRunStatusStateStopping, current.workspace, nil, &current.startedAt)
	if err := current.stop(ctx); err != nil {
		return s.setErrorStatus(current.workspace, err), err
	}
	return s.setStatus(apitypes.PeerRunStatusStateStopped, current.workspace, nil, nil), nil
}

// Shutdown permanently closes the connection-scoped service. It prevents an
// in-flight reload from publishing a runtime after peer teardown has begun.
func (s *Service) Shutdown(ctx context.Context) (apitypes.PeerRunStatus, error) {
	if s == nil {
		return stoppedStatus(time.Now()), nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := s.now()
	status := stoppedStatus(now)
	s.mu.Lock()
	s.closed = true
	current := s.runtime
	s.runtime = nil
	if current != nil && current.workspace != "" {
		status.WorkspaceName = &current.workspace
	}
	s.status = status
	s.mu.Unlock()
	s.cancelLifecycle()
	s.CancelTransition()
	if current == nil {
		return status, nil
	}
	return status, current.stop(ctx)
}

func (s *Service) beginTransition() {
	s.revision.Add(1)
}

func (s *Service) finishTransition() {
	s.revision.Add(1)
}

func (s *Service) lifecycleContext() context.Context {
	s.lifecycleOnce.Do(func() {
		s.lifecycleCtx, s.lifecycleCancel = context.WithCancel(context.Background())
	})
	return s.lifecycleCtx
}

func (s *Service) cancelLifecycle() {
	_ = s.lifecycleContext()
	s.lifecycleCancel()
}

// CancelTransition asks the currently running lifecycle or input operation to
// stop. Shutdown uses it to interrupt work still holding the transition gate.
func (s *Service) CancelTransition() {
	if s == nil {
		return
	}
	s.transitionCancelMu.Lock()
	cancel := s.transitionCancel
	s.transitionCancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) lockTransition(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.isClosed() {
		return ErrServiceClosed
	}
	s.transitionGateOnce.Do(func() {
		s.transitionGate = make(chan struct{}, 1)
		s.transitionGate <- struct{}{}
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.transitionGate:
		if s.isClosed() {
			s.unlockTransition()
			return ErrServiceClosed
		}
		return nil
	}
}

func (s *Service) unlockTransition() {
	s.transitionGate <- struct{}{}
}

func (s *Service) beginCancellableTransition(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	s.transitionCancelMu.Lock()
	s.transitionCancel = cancel
	s.transitionCancelMu.Unlock()
	if s.isClosed() {
		cancel()
	}
	return ctx, func() {
		s.transitionCancelMu.Lock()
		s.transitionCancel = nil
		s.transitionCancelMu.Unlock()
		cancel()
	}
}

func (s *Service) WorkspaceState(ctx context.Context) (apitypes.PeerRunWorkspaceState, error) {
	if s == nil {
		return apitypes.PeerRunWorkspaceState{}, ErrNilService
	}
	status, err := s.Status(ctx)
	if err != nil {
		return apitypes.PeerRunWorkspaceState{}, err
	}
	state := workspaceStateFromStatus(status)
	rt := s.currentRuntime()
	if rt == nil || rt.agent == nil {
		return state, nil
	}
	agentState, err := rt.agent.Status(ctx)
	if err != nil {
		return state, err
	}
	mergeWorkspaceState(&state, agentState)
	if state.WorkspaceName == "" {
		state.WorkspaceName = rt.workspace
	}
	if state.ActiveWorkspaceName == nil && rt.workspace != "" {
		state.ActiveWorkspaceName = &rt.workspace
	}
	if state.StartedAt == nil {
		state.StartedAt = &rt.startedAt
	}
	return state, nil
}

func (s *Service) ListWorkspaceHistory(ctx context.Context, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	rt, err := s.currentRuntimeForFeature(ctx)
	if err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryListResponse{Available: false, Items: []apitypes.PeerRunHistoryEntry{}, HasNext: false, Message: &message}, nil
	}
	return rt.agent.ListHistory(s.gearContext(ctx), req)
}

func (s *Service) PlayWorkspaceHistory(ctx context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	rt, err := s.currentRuntimeForFeature(ctx)
	if err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryName: req.HistoryName, State: "unavailable", Message: &message}, nil
	}
	return rt.agent.PlayHistory(s.gearContext(ctx), req)
}

func (s *Service) WorkspaceMemoryStats(ctx context.Context, req apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	rt, err := s.currentRuntimeForFeature(ctx)
	if err != nil {
		message := err.Error()
		return apitypes.PeerRunMemoryStatsResponse{Available: false, Enabled: false, ItemCount: 0, StorageBytes: 0, Message: &message}, nil
	}
	return rt.agent.MemoryStats(s.gearContext(ctx), req)
}

func (s *Service) WorkspaceRecall(ctx context.Context, req apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	rt, err := s.currentRuntimeForFeature(ctx)
	if err != nil {
		message := err.Error()
		return apitypes.PeerRunRecallResponse{Available: false, Hits: []apitypes.PeerRunRecallHit{}, Message: &message}, nil
	}
	return rt.agent.Recall(s.gearContext(ctx), req)
}

func (s *Service) validate() error {
	switch {
	case s.Host == nil:
		return ErrMissingHost
	case s.PeerRun == nil:
		return ErrMissingPeerRun
	case s.PublicKey.IsZero():
		return ErrInvalidPublicKey
	case s.Source == nil:
		return ErrMissingSource
	case s.Consumer == nil:
		return ErrMissingConsumer
	default:
		return nil
	}
}

func (s *Service) validateRunSelection() error {
	switch {
	case s.PeerRun == nil:
		return ErrMissingPeerRun
	case s.PublicKey.IsZero():
		return ErrInvalidPublicKey
	default:
		return nil
	}
}

func (s *Service) gearContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return withHistoryGearID(ctx, s.PublicKey.String())
}

func (s *Service) swap(next *runtime) *runtime {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.runtime
	s.runtime = next
	return previous
}

func (s *Service) publishWithCommit(next *runtime, now time.Time, commit func() error) (apitypes.PeerRunStatus, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.status, false, ErrServiceClosed
	}
	if commit != nil {
		if err := commit(); err != nil {
			return s.status, false, err
		}
	}
	s.runtime = next
	status := runningStatus(next.workspace, next.startedAt, now)
	s.status = status
	return status, true, nil
}

func (s *Service) publishFailure(next *runtime, now time.Time) (apitypes.PeerRunStatus, bool) {
	message := agentReloadFailedMessage
	status := apitypes.PeerRunStatus{
		State:         apitypes.PeerRunStatusStateError,
		WorkspaceName: nil,
		Message:       &message,
		StartedAt:     &next.startedAt,
		UpdatedAt:     &now,
	}
	if next.workspace != "" {
		status.WorkspaceName = &next.workspace
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.status, false
	}
	s.runtime = next
	s.status = status
	return status, true
}

func (s *Service) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Service) currentRuntime() *runtime {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtime
}

func (s *Service) currentAgent() (Agent, error) {
	rt := s.currentRuntime()
	if rt == nil || rt.agent == nil {
		return nil, ErrNoActiveWorkspace
	}
	return rt.agent, nil
}

func (s *Service) currentRuntimeForFeature(ctx context.Context) (*runtime, error) {
	rt := s.currentRuntime()
	if rt == nil || rt.agent == nil {
		return nil, ErrNoActiveWorkspace
	}
	if s.ValidateWorkspaceSelection != nil {
		canonicalName, err := s.ValidateWorkspaceSelection(ctx, rt.workspace)
		if err != nil {
			return nil, err
		}
		if canonicalName != rt.workspace {
			return nil, fmt.Errorf("agenthost: active workspace changed from %q to %q", rt.workspace, canonicalName)
		}
	}
	return rt, nil
}

func (s *Service) consume(ctx context.Context, rt *runtime) {
	defer close(rt.done)
	defer rt.releaseOnce()
	defer func() {
		if rt.output != nil {
			_ = rt.output.Close()
		}
		if rt.input != nil {
			_ = rt.input.Close()
		}
	}()
	probe := newOutputProbe(rt.output)
	err := s.Consumer.ConsumeAgentOutput(ctx, probe)
	if err == nil && ctx.Err() == nil {
		// A clean end of the agent output while the runtime is active means an
		// upstream stage closed its stream without reporting an error. Name the
		// last observed route so operators can tell which pipeline stage ended.
		err = fmt.Errorf("%w (%s)", errUnexpectedOutputEnd, probe.summary())
	}
	if err != nil && ctx.Err() == nil && !errors.Is(err, errWorkspaceQuiesced) {
		if !s.failRuntime(rt, err) {
			return
		}
		s.logger().Error("agenthost: output consumer failed", append([]any{"workspace", rt.workspace, "error", err}, probe.logAttrs()...)...)
		if s.OnConsumerError != nil {
			s.OnConsumerError(context.WithoutCancel(ctx), rt.workspace, err)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == rt {
		s.runtime = nil
		s.status = stoppedStatus(s.now())
		if rt.workspace != "" {
			s.status.WorkspaceName = &rt.workspace
		}
	}
}

func (s *Service) failRuntime(rt *runtime, err error) bool {
	now := s.now()
	message := err.Error()
	if rt.failure != nil {
		message = agentReloadFailedMessage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime != rt {
		return false
	}
	s.runtime = nil
	if !s.closed {
		s.status = apitypes.PeerRunStatus{
			State:         apitypes.PeerRunStatusStateError,
			WorkspaceName: &rt.workspace,
			Message:       &message,
			UpdatedAt:     &now,
		}
	}
	return true
}

func (s *Service) setErrorStatus(workspace string, err error) apitypes.PeerRunStatus {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return s.setStatus(apitypes.PeerRunStatusStateError, workspace, &message, nil)
}

func (s *Service) setStatus(state apitypes.PeerRunStatusState, workspace string, message *string, startedAt *time.Time) apitypes.PeerRunStatus {
	now := s.now()
	status := apitypes.PeerRunStatus{
		State:     state,
		UpdatedAt: &now,
		Message:   message,
		StartedAt: startedAt,
	}
	if workspace != "" {
		status.WorkspaceName = &workspace
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed && state != apitypes.PeerRunStatusStateStopped {
		return s.status
	}
	s.status = status
	return status
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) logger() *slog.Logger {
	if s != nil && s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func workspacePattern(workspaceName string) string {
	return "workspaces/" + url.PathEscape(workspaceName)
}

func runningStatus(workspace string, startedAt, updatedAt time.Time) apitypes.PeerRunStatus {
	status := apitypes.PeerRunStatus{
		State:     apitypes.PeerRunStatusStateRunning,
		StartedAt: &startedAt,
		UpdatedAt: &updatedAt,
	}
	if workspace != "" {
		status.WorkspaceName = &workspace
	}
	return status
}

func stoppedStatus(updatedAt time.Time) apitypes.PeerRunStatus {
	return apitypes.PeerRunStatus{
		State:     apitypes.PeerRunStatusStateStopped,
		UpdatedAt: &updatedAt,
	}
}

type runtime struct {
	cancel    context.CancelFunc
	agent     Agent
	input     genx.Stream
	output    genx.Stream
	release   func()
	once      sync.Once
	stopOnce  sync.Once
	stopErr   error
	done      chan struct{}
	workspace string
	startedAt time.Time
	failure   error
}

func (r *runtime) stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.stopOnce.Do(func() {
		r.cancel()
		r.stopErr = errors.Join(r.output.Close(), r.input.Close())
	})
	select {
	case <-r.done:
		r.releaseOnce()
		return r.stopErr
	case <-ctx.Done():
		r.releaseOnce()
		return errors.Join(r.stopErr, ctx.Err())
	}
}

func (r *runtime) releaseOnce() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		if r.release != nil {
			r.release()
		}
	})
}

func workspaceStateFromStatus(status apitypes.PeerRunStatus) apitypes.PeerRunWorkspaceState {
	state := apitypes.PeerRunWorkspaceState{
		RuntimeState:  status.State,
		WorkspaceName: "",
		StartedAt:     status.StartedAt,
		UpdatedAt:     status.UpdatedAt,
		Message:       status.Message,
	}
	if status.WorkspaceName != nil {
		workspace := *status.WorkspaceName
		state.WorkspaceName = workspace
		state.ActiveWorkspaceName = &workspace
	}
	return state
}

func mergeWorkspaceState(dst *apitypes.PeerRunWorkspaceState, src apitypes.PeerRunWorkspaceState) {
	if src.WorkspaceName != "" {
		dst.WorkspaceName = src.WorkspaceName
	}
	if src.RuntimeState != "" {
		dst.RuntimeState = src.RuntimeState
	}
	if src.SelectedWorkspaceName != nil {
		dst.SelectedWorkspaceName = src.SelectedWorkspaceName
	}
	if src.PendingWorkspaceName != nil {
		dst.PendingWorkspaceName = src.PendingWorkspaceName
	}
	if src.ActiveWorkspaceName != nil {
		dst.ActiveWorkspaceName = src.ActiveWorkspaceName
	}
	if src.WorkflowName != nil {
		dst.WorkflowName = src.WorkflowName
	}
	if src.AgentType != nil {
		dst.AgentType = src.AgentType
	}
	if src.Message != nil {
		dst.Message = src.Message
	}
	if src.HistoryAvailable != nil {
		dst.HistoryAvailable = src.HistoryAvailable
	}
	if src.MemoryStatsAvailable != nil {
		dst.MemoryStatsAvailable = src.MemoryStatsAvailable
	}
	if src.RecallAvailable != nil {
		dst.RecallAvailable = src.RecallAvailable
	}
	if src.StartedAt != nil {
		dst.StartedAt = src.StartedAt
	}
	if src.UpdatedAt != nil {
		dst.UpdatedAt = src.UpdatedAt
	}
}
