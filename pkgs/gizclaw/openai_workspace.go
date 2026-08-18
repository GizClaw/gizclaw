package gizclaw

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
)

type openAIWorkspaceAdapter struct {
	caller    giznet.PublicKey
	manager   *Manager
	resources *peerresource.Server
}

type openAIWorkspaceStore interface {
	GetWorkspaceRuntimeByID(context.Context, string) (workspace.Runtime, error)
	AppendWorkspaceHistoryByID(context.Context, string, workspace.AppendHistoryRequest) (workspace.HistoryEntry, error)
}

func (a openAIWorkspaceAdapter) CreateConversationWorkspace(ctx context.Context, request openaiapi.ConversationWorkspaceRequest) (apitypes.Workspace, error) {
	if a.resources == nil {
		return apitypes.Workspace{}, errors.New("OpenAI Workspace resources are unavailable")
	}
	created, err := a.resources.CreateWorkspace(ctx, peerresource.WorkspaceCreateRequest{
		Name: request.Name, Collection: request.Collection, WorkflowName: request.WorkflowName,
		Labels: map[string]string{"openai.conversation": "true"}, Initialize: request.Initialize,
	})
	return created.Workspace, err
}

func (a openAIWorkspaceAdapter) GetConversationWorkspace(ctx context.Context, name string) (apitypes.Workspace, error) {
	if a.resources == nil {
		return apitypes.Workspace{}, errors.New("OpenAI Workspace resources are unavailable")
	}
	item, rpcErr := a.resources.ResolveRunWorkspaceSelection(ctx, name)
	if rpcErr != nil {
		return apitypes.Workspace{}, errors.New(rpcErr.Message)
	}
	return item, nil
}

func (a openAIWorkspaceAdapter) store() (openAIWorkspaceStore, error) {
	if a.manager == nil {
		return nil, errors.New("OpenAI Workspace manager is unavailable")
	}
	store, ok := a.manager.Workspaces.(openAIWorkspaceStore)
	if !ok {
		return nil, errors.New("OpenAI Workspace store is unavailable")
	}
	return store, nil
}

func (a openAIWorkspaceAdapter) GetConversationRuntime(ctx context.Context, id string) (workspace.Runtime, error) {
	store, err := a.store()
	if err != nil {
		return workspace.Runtime{}, err
	}
	return store.GetWorkspaceRuntimeByID(a.ownerContext(ctx), id)
}

func (a openAIWorkspaceAdapter) AppendConversationHistory(ctx context.Context, id string, request workspace.AppendHistoryRequest) (workspace.HistoryEntry, error) {
	store, err := a.store()
	if err != nil {
		return workspace.HistoryEntry{}, err
	}
	return store.AppendWorkspaceHistoryByID(a.ownerContext(ctx), id, request)
}

func (a openAIWorkspaceAdapter) ExecuteWorkspaceText(ctx context.Context, item apitypes.Workspace, text string, delta func(string) error) ([]workspace.HistoryEntry, error) {
	if a.manager == nil || a.manager.AgentHost == nil {
		return nil, errors.New("OpenAI Workspace Agent is unavailable")
	}
	host := newPeerAgentHost(
		a.manager.AgentHost, nil, nil, a.manager.ownerGenX, a.manager.Gameplay,
		a.manager.FlowcraftHistory, a.manager.FlowcraftState, a.manager.MemoryRoot, a.manager.MemoryStores,
	)
	resolver, ok := host.Resolver.(canonicalAgentResolver)
	if !ok {
		return nil, errors.New("OpenAI canonical Workspace resolver is unavailable")
	}
	host.Resolver = openAICanonicalResolver{resolver: resolver}
	ctx = a.ownerContext(ctx)
	ctx, err := agenthost.WithToolExecution(ctx, nil, unavailableOpenAIClientTools{})
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	var observed []workspace.HistoryEntry
	ctx = agenthost.WithWorkspaceHistoryObserver(ctx, func(_ context.Context, _ string, entry workspace.HistoryEntry) {
		if entry.Type != "agent" || strings.TrimSpace(entry.Text) == "" {
			return
		}
		mu.Lock()
		observed = append(observed, entry)
		mu.Unlock()
	})
	input := &openAITextStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: genx.Text(text)},
		{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{EndOfStream: true}},
	}}
	output, err := host.Transform(ctx, item.Id, input)
	if err != nil {
		return nil, err
	}
	defer output.Close()
	firstTextRoute := ""
	firstTextRouteSet := false
	for {
		chunk, nextErr := output.Next()
		if chunk != nil && chunk.Role != genx.RoleUser {
			if value, ok := chunk.Part.(genx.Text); ok && value != "" && delta != nil {
				route := chunk.Name
				if chunk.Ctrl != nil {
					route = chunk.Ctrl.StreamID + "\x00" + chunk.Ctrl.Label + "\x00" + route
				}
				if !firstTextRouteSet {
					firstTextRoute = route
					firstTextRouteSet = true
				}
				if route == firstTextRoute {
					if err := delta(string(value)); err != nil {
						_ = output.CloseWithError(err)
						return nil, err
					}
				}
			}
		}
		if errors.Is(nextErr, genx.ErrDone) {
			break
		}
		if nextErr != nil {
			return nil, nextErr
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(observed) == 0 {
		return nil, errors.New("Workspace Agent produced no persisted text output")
	}
	return append([]workspace.HistoryEntry(nil), observed...), nil
}

func (a openAIWorkspaceAdapter) ownerContext(ctx context.Context) context.Context {
	return ownership.WithOwner(ctx, a.caller.String())
}

type openAICanonicalResolver struct{ resolver canonicalAgentResolver }

func (r openAICanonicalResolver) Resolve(ctx context.Context, pattern string) (agenthost.Spec, error) {
	return r.resolver.ResolveByID(ctx, strings.TrimSpace(pattern))
}

type unavailableOpenAIClientTools struct{}

func (unavailableOpenAIClientTools) InvokeClientTool(context.Context, string, []byte) ([]byte, error) {
	return nil, giztools.ErrClientToolUnavailable
}

type openAITextStream struct{ chunks []*genx.MessageChunk }

func (s *openAITextStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, genx.ErrDone
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}
func (*openAITextStream) Close() error               { return nil }
func (*openAITextStream) CloseWithError(error) error { return nil }

var _ openaiapi.ConversationWorkspaces = openAIWorkspaceAdapter{}
var _ openaiapi.WorkspaceExecutor = openAIWorkspaceAdapter{}
