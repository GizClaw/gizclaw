package agenthost

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const unsupportedMessage = "workspace runtime feature is not supported by this agent"
const agentReloadFailedCode = "AGENT_RELOAD_FAILED"
const agentReloadFailedMessage = "workspace Agent reload failed"

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
		Accepted:    false,
		HistoryName: req.HistoryName,
		State:       "unsupported",
		Message:     &message,
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

type reloadErrorAgent struct {
	transformerAgent
}

func newReloadErrorAgent() Agent {
	transformer := reloadErrorTransformer{}
	return reloadErrorAgent{
		transformerAgent: transformerAgent{Transformer: transformer},
	}
}

func (a reloadErrorAgent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	available := false
	message := agentReloadFailedMessage
	return apitypes.PeerRunWorkspaceState{
		RuntimeState:         apitypes.PeerRunStatusStateError,
		HistoryAvailable:     &available,
		MemoryStatsAvailable: &available,
		RecallAvailable:      &available,
		Message:              &message,
	}, nil
}

type reloadErrorTransformer struct{}

func (t reloadErrorTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	go t.forward(ctx, input, output)
	return output.Stream(), nil
}

func (t reloadErrorTransformer) forward(ctx context.Context, input genx.Stream, output *genx.StreamBuilder) {
	seen := make(map[string]struct{})
	order := make([]string, 0, 64)
	for {
		chunk, err := input.Next()
		if err != nil {
			if ctx.Err() != nil {
				_ = output.Abort(ctx.Err())
				return
			}
			_ = output.Abort(err)
			return
		}
		if chunk == nil || !chunk.IsBeginOfStream() || chunk.Ctrl.StreamID == "" {
			continue
		}
		if _, ok := seen[chunk.Ctrl.StreamID]; ok {
			continue
		}
		seen[chunk.Ctrl.StreamID] = struct{}{}
		order = append(order, chunk.Ctrl.StreamID)
		if len(order) > 64 {
			delete(seen, order[0])
			order = order[1:]
		}
		if err := output.Add(reloadFailureEOS(chunk)); err != nil {
			_ = output.Abort(err)
			return
		}
	}
}

func reloadFailureEOS(begin *genx.MessageChunk) *genx.MessageChunk {
	ctrl := *begin.Ctrl
	ctrl.BeginOfStream = false
	ctrl.EndOfStream = true
	ctrl.Error = agentReloadFailedMessage
	ctrl.ErrorCode = agentReloadFailedCode
	ctrl.ErrorRetryable = false
	ctrl.FailureClass = genx.FailureClassTransform

	var part genx.Part
	if blob, ok := begin.Part.(*genx.Blob); ok && blob != nil {
		part = &genx.Blob{MIMEType: blob.MIMEType}
	}
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: begin.Name,
		Part: part,
		Ctrl: &ctrl,
	}
}
