package bridge

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
)

type ContextSummary struct {
	Current        bool   `json:"current"`
	Description    string `json:"description,omitempty"`
	Endpoint       string `json:"endpoint"`
	LocalPublicKey string `json:"local_public_key,omitempty"`
	Name           string `json:"name"`
}

type RuntimeContext struct {
	Context          *ContextSummary `json:"context,omitempty"`
	PrivateKeyBase64 string          `json:"private_key_base64,omitempty"`
}

type CreateContextRequest struct {
	Description string `json:"description,omitempty"`
	Endpoint    string `json:"endpoint"`
	Name        string `json:"name"`
}

type ContextBridge struct {
	Store *contextstore.Store
	State appconfig.StateStore
}

func NewContextBridge(store *contextstore.Store, state appconfig.StateStore) *ContextBridge {
	return &ContextBridge{Store: store, State: state}
}

func (b *ContextBridge) ListContexts(context.Context) ([]ContextSummary, error) {
	if b == nil || b.Store == nil {
		return nil, fmt.Errorf("desktop bridge: context store is not configured")
	}
	summaries, err := b.Store.ListSummaries()
	if err != nil {
		return nil, err
	}
	out := make([]ContextSummary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, contextSummary(summary))
	}
	return out, nil
}

func (b *ContextBridge) SelectContext(_ context.Context, name string) (ContextSummary, error) {
	if b == nil || b.Store == nil {
		return ContextSummary{}, fmt.Errorf("desktop bridge: context store is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ContextSummary{}, fmt.Errorf("desktop bridge: context name is required")
	}
	if err := b.Store.Use(name); err != nil {
		return ContextSummary{}, err
	}
	state, err := b.State.Load()
	if err != nil {
		return ContextSummary{}, err
	}
	state.LastContext = name
	if err := b.State.Save(state); err != nil {
		return ContextSummary{}, err
	}
	current, err := b.Store.Current()
	if err != nil {
		return ContextSummary{}, err
	}
	summary, err := contextstore.LoadSummary(current.Dir)
	if err != nil {
		return ContextSummary{}, err
	}
	summary.Current = true
	return contextSummary(summary), nil
}

func (b *ContextBridge) CreateContext(_ context.Context, req CreateContextRequest) (ContextSummary, error) {
	if b == nil || b.Store == nil {
		return ContextSummary{}, fmt.Errorf("desktop bridge: context store is not configured")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ContextSummary{}, fmt.Errorf("desktop bridge: context name is required")
	}
	if err := b.Store.CreateWithOptions(name, strings.TrimSpace(req.Endpoint), contextstore.CreateOptions{
		Description: strings.TrimSpace(req.Description),
	}); err != nil {
		return ContextSummary{}, err
	}
	return b.SelectContext(context.Background(), name)
}

func (b *ContextBridge) RuntimeContext(context.Context) (RuntimeContext, error) {
	if b == nil || b.Store == nil {
		return RuntimeContext{}, fmt.Errorf("desktop bridge: context store is not configured")
	}
	current, err := b.Store.Current()
	if err != nil {
		return RuntimeContext{}, err
	}
	if current == nil {
		return RuntimeContext{}, nil
	}
	summary, err := contextstore.LoadSummary(current.Dir)
	if err != nil {
		return RuntimeContext{}, err
	}
	summary.Current = true
	dto := contextSummary(summary)
	return RuntimeContext{
		Context:          &dto,
		PrivateKeyBase64: base64.StdEncoding.EncodeToString(current.KeyPair.Private[:]),
	}, nil
}

func contextSummary(summary contextstore.Summary) ContextSummary {
	return ContextSummary{
		Current:        summary.Current,
		Description:    summary.Description,
		Endpoint:       summary.Endpoint,
		LocalPublicKey: summary.LocalPublicKey.String(),
		Name:           summary.Name,
	}
}
