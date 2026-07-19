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

var (
	toolsRoot        = kv.Key{"by-id"}
	toolsByOwnerRoot = kv.Key{"by-owner"}
)

type Server struct {
	Store kv.Store
	Now   func() time.Time
}

func (s *Server) GetTool(ctx context.Context, id string) (Tool, error) {
	store, err := s.store()
	if err != nil {
		return Tool{}, err
	}
	id, err = normalizeToolID(id)
	if err != nil {
		return Tool{}, err
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

// ListToolsByOwner reads the immutable owner index used by Peer RPC.
func (s *Server) ListToolsByOwner(ctx context.Context, owner string) ([]Tool, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return []Tool{}, nil
	}
	prefix := toolByOwnerPrefix(owner)
	tools := make([]Tool, 0)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, fmt.Errorf("toolkit: list owner %q: %w", owner, err)
		}
		if len(entry.Key) == 0 {
			continue
		}
		id, err := url.PathUnescape(entry.Key[len(entry.Key)-1])
		if err != nil {
			return nil, fmt.Errorf("toolkit: decode owner index %q: %w", entry.Key.String(), err)
		}
		tool, err := s.GetTool(ctx, id)
		if errors.Is(err, ErrToolNotFound) {
			continue
		}
		if err != nil {
			return nil, err
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
	var previous *Tool
	if existing, err := s.GetTool(ctx, tool.ID); err == nil {
		tool.CreatedAt = existing.CreatedAt
		tool.OwnerPeer = cloneStringPtr(existing.OwnerPeer)
		tool.OwnerPublicKey = cloneStringPtr(existing.OwnerPublicKey)
		previous = &existing
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
	entries := []kv.Entry{{Key: toolKey(tool.ID), Value: data}}
	if tool.OwnerPublicKey != nil {
		entries = append(entries, kv.Entry{Key: toolByOwnerKey(*tool.OwnerPublicKey, tool.ID), Value: []byte{}})
	}
	if previous != nil && previous.OwnerPublicKey != nil && (tool.OwnerPublicKey == nil || *previous.OwnerPublicKey != *tool.OwnerPublicKey) {
		if err := store.Delete(ctx, toolByOwnerKey(*previous.OwnerPublicKey, tool.ID)); err != nil {
			return Tool{}, fmt.Errorf("toolkit: delete stale owner index %q: %w", tool.ID, err)
		}
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return Tool{}, fmt.Errorf("toolkit: put tool %q: %w", tool.ID, err)
	}
	return cloneTool(tool), nil
}

func (s *Server) DeleteTool(ctx context.Context, id string) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	id, err = normalizeToolID(id)
	if err != nil {
		return err
	}
	tool, err := s.GetTool(ctx, id)
	if err != nil && !errors.Is(err, ErrToolNotFound) {
		return err
	}
	keys := []kv.Key{toolKey(id)}
	if tool.OwnerPublicKey != nil {
		keys = append(keys, toolByOwnerKey(*tool.OwnerPublicKey, id))
	}
	if err := store.BatchDelete(ctx, keys); err != nil {
		return fmt.Errorf("toolkit: delete tool %q: %w", id, err)
	}
	return nil
}

func toolByOwnerKey(owner, id string) kv.Key {
	return append(toolByOwnerPrefix(owner), escapeToolSegment(id))
}

func toolByOwnerPrefix(owner string) kv.Key {
	return append(append(kv.Key{}, toolsByOwnerRoot...), escapeToolSegment(owner))
}

func escapeToolSegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
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
