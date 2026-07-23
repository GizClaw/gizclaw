package agenthost

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const unsupportedMessage = "workspace runtime feature is not supported by this agent"

type boardInputsContextKey struct{}

// BoardInputsFromContext returns product-owned transient inputs injected for
// the current turn. Drivers that support contextual execution can consume
// these values without persisting them in a Workflow or Workspace.
func BoardInputsFromContext(ctx context.Context) (map[string]any, bool) {
	inputs, ok := ctx.Value(boardInputsContextKey{}).(map[string]any)
	return inputs, ok
}

// Agent is the active workspace runtime surface.
type Agent interface {
	genx.Transformer
	Status(context.Context) (apitypes.PeerRunWorkspaceState, error)
	ListHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error)
	PlayHistory(context.Context, apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error)
	MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error)
	Recall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error)
}

func asAgent(transformer genx.Transformer) Agent {
	if transformer == nil {
		return nil
	}
	if agent, ok := transformer.(Agent); ok {
		return agent
	}
	return transformerAgent{Transformer: transformer}
}

func NewTransformerAgent(transformer genx.Transformer) Agent {
	return asAgent(transformer)
}

// NewBoardInputsAgent resolves transient product context for every Transform
// call and makes it available to any nested driver through the turn context.
func NewBoardInputsAgent(agent Agent, provider func(context.Context) (map[string]any, error)) Agent {
	if agent == nil || provider == nil {
		return agent
	}
	return boardInputsAgent{Agent: agent, provider: provider}
}

type boardInputsAgent struct {
	Agent
	provider func(context.Context) (map[string]any, error)
}

func (a boardInputsAgent) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	inputs, err := a.provider(ctx)
	if err != nil {
		return nil, err
	}
	return a.Agent.Transform(context.WithValue(ctx, boardInputsContextKey{}, inputs), input)
}

type transformerAgent struct {
	genx.Transformer
}

func (a transformerAgent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	available := false
	return apitypes.PeerRunWorkspaceState{
		RuntimeState:         apitypes.PeerRunStatusStateRunning,
		HistoryAvailable:     &available,
		MemoryStatsAvailable: &available,
		RecallAvailable:      &available,
	}, nil
}

func (a transformerAgent) ListHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	message := unsupportedMessage
	return apitypes.PeerRunHistoryListResponse{
		Available: false,
		Items:     []apitypes.PeerRunHistoryEntry{},
		HasNext:   false,
		Message:   &message,
	}, nil
}

func (a transformerAgent) PlayHistory(_ context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	message := unsupportedMessage
	return apitypes.PeerRunHistoryPlayResponse{
		Accepted:  false,
		HistoryId: req.HistoryId,
		State:     "unsupported",
		Message:   &message,
	}, nil
}

func (a transformerAgent) MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	message := unsupportedMessage
	return apitypes.PeerRunMemoryStatsResponse{
		Available:    false,
		Enabled:      false,
		ItemCount:    0,
		StorageBytes: 0,
		Message:      &message,
	}, nil
}

func (a transformerAgent) Recall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	message := unsupportedMessage
	return apitypes.PeerRunRecallResponse{
		Available: false,
		Hits:      []apitypes.PeerRunRecallHit{},
		Message:   &message,
	}, nil
}
