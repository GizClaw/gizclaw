package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var toolsRoot = kv.Key{"tools", "by-id"}

type Server struct {
	Store kv.Store
	Now   func() time.Time
}

func (s *Server) GetTool(ctx context.Context, id string) (Tool, error) {
	store, err := s.store()
	if err != nil {
		return Tool{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Tool{}, fmt.Errorf("%w: id is required", ErrInvalidTool)
	}
	data, err := store.Get(ctx, toolKey(id))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return Tool{}, ErrToolNotFound
		}
		return Tool{}, fmt.Errorf("toolkit: get tool %q: %w", id, err)
	}
	tool, err := decodeTool(data)
	if err != nil {
		return Tool{}, fmt.Errorf("toolkit: decode tool %q: %w", id, err)
	}
	return tool, nil
}

func (s *Server) ListTools(ctx context.Context) ([]Tool, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	var tools []Tool
	for entry, err := range store.List(ctx, toolsRoot) {
		if err != nil {
			return nil, fmt.Errorf("toolkit: list tools: %w", err)
		}
		tool, err := decodeTool(entry.Value)
		if err != nil {
			return nil, fmt.Errorf("toolkit: decode tool at %s: %w", entry.Key.String(), err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func (s *Server) PutTool(ctx context.Context, tool Tool) (Tool, error) {
	store, err := s.store()
	if err != nil {
		return Tool{}, err
	}
	tool, err = NormalizeTool(tool)
	if err != nil {
		return Tool{}, err
	}
	now := s.now()
	if existing, err := s.GetTool(ctx, tool.ID); err == nil {
		tool.CreatedAt = existing.CreatedAt
		tool.SyncedAt = cloneTimePtr(existing.SyncedAt)
	} else if !errors.Is(err, ErrToolNotFound) {
		return Tool{}, err
	} else {
		tool.CreatedAt = now
	}
	tool.UpdatedAt = now
	data, err := json.Marshal(tool)
	if err != nil {
		return Tool{}, fmt.Errorf("toolkit: encode tool %q: %w", tool.ID, err)
	}
	if err := store.Set(ctx, toolKey(tool.ID), data); err != nil {
		return Tool{}, fmt.Errorf("toolkit: put tool %q: %w", tool.ID, err)
	}
	return cloneTool(tool), nil
}

func (s *Server) DeleteTool(ctx context.Context, id string) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidTool)
	}
	if err := store.Delete(ctx, toolKey(id)); err != nil {
		return fmt.Errorf("toolkit: delete tool %q: %w", id, err)
	}
	return nil
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, ErrNotConfigured
	}
	return s.Store, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func decodeTool(data []byte) (Tool, error) {
	var tool Tool
	if err := json.Unmarshal(data, &tool); err != nil {
		return Tool{}, err
	}
	return NormalizeTool(tool)
}

func toolKey(id string) kv.Key {
	return append(append(kv.Key{}, toolsRoot...), url.PathEscape(id))
}
