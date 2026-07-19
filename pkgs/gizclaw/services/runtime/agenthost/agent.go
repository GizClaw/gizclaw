package agenthost

import (
	"context"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const unsupportedMessage = "workspace runtime feature is not supported by this agent"

// Agent is the active workspace runtime surface.
type Agent interface {
	commonagent.Agent
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
