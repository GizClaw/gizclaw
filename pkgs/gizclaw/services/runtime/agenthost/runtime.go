package agenthost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var (
	ErrNilService        = errors.New("agenthost: nil service")
	ErrMissingHost       = errors.New("agenthost: host is required")
	ErrMissingPeerRun    = errors.New("agenthost: peer run store is required")
	ErrMissingSource     = errors.New("agenthost: stream source is required")
	ErrMissingConsumer   = errors.New("agenthost: stream consumer is required")
	ErrInvalidPublicKey  = errors.New("agenthost: invalid public key")
	ErrNoActiveWorkspace = errors.New("agenthost: no active workspace")
)

type PeerRunStore interface {
	ResolveRunAgent(context.Context, giznet.PublicKey) (apitypes.AgentSelection, error)
	ActivateRunAgent(context.Context, giznet.PublicKey, apitypes.AgentSelection) (apitypes.PeerRunAgent, error)
}

type StreamSource interface {
	OpenAgentInput(context.Context) (genx.Stream, error)
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
	Host                       genx.Transformer
	PeerRun                    PeerRunStore
	RuntimeProfile             func() *apitypes.RuntimeProfile
	ValidateWorkspaceSelection WorkspaceSelectionValidatorFunc
	PublicKey                  giznet.PublicKey
	Source                     StreamSource
	Consumer                   StreamConsumer
	OnConsumerError            func(context.Context, string, error)
	Logger                     *slog.Logger
	Now                        func() time.Time

	mu      sync.Mutex
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

	selection, err := s.PeerRun.ResolveRunAgent(ctx, s.PublicKey)
	if err != nil {
		return s.setErrorStatus("", err), err
	}
	if s.ValidateWorkspaceSelection != nil {
		canonicalName, err := s.ValidateWorkspaceSelection(ctx, selection.WorkspaceName)
		if err != nil {
			return s.setErrorStatus(selection.WorkspaceName, err), err
		}
		selection.WorkspaceName = canonicalName
	}
	s.setStatus(apitypes.PeerRunStatusStateStarting, selection.WorkspaceName, nil, nil)
	previous := s.swap(nil)
	if err := previous.stop(ctx); err != nil {
		return s.setErrorStatus(selection.WorkspaceName, fmt.Errorf("agenthost: stop previous runtime: %w", err)), err
	}

	input, err := s.Source.OpenAgentInput(ctx)
	if err != nil {
		return s.setErrorStatus(selection.WorkspaceName, fmt.Errorf("agenthost: open input stream: %w", err)), err
	}
	if input == nil {
		err := errors.New("agenthost: input stream is required")
		return s.setErrorStatus(selection.WorkspaceName, err), err
	}
	var profileToolIDs []string
	profileWorkflowBindings := map[string]string{}
	if s.RuntimeProfile != nil {
		if profile := s.RuntimeProfile(); profile != nil {
			if profile.Spec.Resources.Tools != nil {
				for _, toolID := range *profile.Spec.Resources.Tools {
					profileToolIDs = append(profileToolIDs, toolID)
				}
			}
			if profile.Spec.Resources.Workflows != nil {
				for alias, name := range *profile.Spec.Resources.Workflows {
					profileWorkflowBindings[alias] = name
				}
			}
		}
	}
	baseCtx := WithResourceAccess(withHistoryGearID(context.WithoutCancel(ctx), s.PublicKey.String()), s.PublicKey.String(), profileToolIDs, profileWorkflowBindings)
	runCtx, cancel := context.WithCancel(baseCtx)
	pattern := workspacePattern(selection.WorkspaceName)
	agent, release, output, err := s.openAgentOutput(runCtx, pattern, input)
	if err != nil {
		cancel()
		_ = input.CloseWithError(err)
		return s.setErrorStatus(selection.WorkspaceName, err), err
	}
	if output == nil {
		cancel()
		if release != nil {
			release()
		}
		_ = input.Close()
		err := errors.New("agenthost: output stream is required")
		return s.setErrorStatus(selection.WorkspaceName, err), err
	}
	if _, err := s.PeerRun.ActivateRunAgent(ctx, s.PublicKey, selection); err != nil {
		cancel()
		if release != nil {
			release()
		}
		_ = errors.Join(output.CloseWithError(err), input.CloseWithError(err))
		return s.setErrorStatus(selection.WorkspaceName, err), err
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
	_ = s.swap(next)
	status := s.setStatus(apitypes.PeerRunStatusStateRunning, selection.WorkspaceName, nil, &now)
	go s.consume(runCtx, next)
	return status, nil
}

type agentOpener interface {
	OpenAgent(context.Context, string) (Agent, func(), error)
}

func (s *Service) openAgentOutput(ctx context.Context, pattern string, input genx.Stream) (Agent, func(), genx.Stream, error) {
	if opener, ok := s.Host.(agentOpener); ok {
		agent, release, err := opener.OpenAgent(ctx, pattern)
		if err != nil {
			return nil, nil, nil, err
		}
		output, err := agent.Transform(ctx, pattern, input)
		if err != nil {
			if release != nil {
				release()
			}
			return nil, nil, nil, err
		}
		return agent, release, output, nil
	}
	output, err := s.Host.Transform(ctx, pattern, input)
	if err != nil {
		return nil, nil, nil, err
	}
	return asAgent(s.Host), nil, output, nil
}

func (s *Service) Status(context.Context) (apitypes.PeerRunStatus, error) {
	if s == nil {
		return apitypes.PeerRunStatus{}, ErrNilService
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime != nil {
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
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unavailable", Message: &message}, nil
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
		if rt.input != nil {
			_ = rt.input.Close()
		}
	}()
	err := s.Consumer.ConsumeAgentOutput(ctx, rt.output)
	if err != nil && ctx.Err() == nil {
		s.logger().Error("agenthost: output consumer failed", "error", err)
		if s.OnConsumerError != nil {
			s.OnConsumerError(context.WithoutCancel(ctx), rt.workspace, err)
		}
		s.mu.Lock()
		if s.runtime == rt {
			s.runtime = nil
		}
		s.mu.Unlock()
		s.setErrorStatus(rt.workspace, err)
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
	done      chan struct{}
	workspace string
	startedAt time.Time
}

func (r *runtime) stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.cancel()
	err := errors.Join(r.output.Close(), r.input.Close())
	select {
	case <-r.done:
		r.releaseOnce()
		return err
	case <-ctx.Done():
		r.releaseOnce()
		return errors.Join(err, ctx.Err())
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
