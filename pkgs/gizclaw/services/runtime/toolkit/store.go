package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	toolsRoot       = kv.Key{"by-id"}
	toolsByNameRoot = kv.Key{"by-name"}
)

type Server struct {
	Store kv.Store
	Now   func() time.Time
	NewID func() string
}

func (s *Server) GetTool(ctx context.Context, name string) (Tool, error) {
	store, err := s.store()
	if err != nil {
		return Tool{}, err
	}
	name, err = normalizeToolName(name)
	if err != nil {
		return Tool{}, err
	}
	id, err := store.Get(ctx, toolNameKey(name))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return Tool{}, ErrToolNotFound
		}
		return Tool{}, fmt.Errorf("toolkit: get tool %q: %w", name, err)
	}
	return s.GetToolByID(ctx, string(id))
}

func (s *Server) GetToolByID(ctx context.Context, id string) (Tool, error) {
	store, err := s.store()
	if err != nil {
		return Tool{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Tool{}, ErrToolNotFound
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

func (s *Server) CreateTool(ctx context.Context, tool Tool) (Tool, error) {
	store, err := s.store()
	if err != nil {
		return Tool{}, err
	}
	tool, err = normalizeToolDeclaration(tool)
	if err != nil {
		return Tool{}, err
	}
	tool.ID = s.newID()
	now := s.now()
	tool.CreatedAt = now
	tool, err = NormalizeTool(tool)
	if err != nil {
		return Tool{}, err
	}
	tool.UpdatedAt = now
	data, err := json.Marshal(tool)
	if err != nil {
		return Tool{}, fmt.Errorf("toolkit: encode tool %q: %w", tool.Name, err)
	}
	_, created, err := kv.CreateIfAbsent(ctx, store,
		kv.Entry{Key: toolNameKey(tool.Name), Value: []byte(tool.ID)},
		[]kv.Entry{{Key: toolKey(tool.ID), Value: data}},
	)
	if err != nil {
		return Tool{}, fmt.Errorf("toolkit: put tool %q: %w", tool.Name, err)
	}
	if !created {
		return Tool{}, fmt.Errorf("%w: tool name %q already exists", ErrInvalidTool, tool.Name)
	}
	return cloneTool(tool), nil
}

func (s *Server) PutTool(ctx context.Context, id string, tool Tool) (Tool, error) {
	store, err := s.store()
	if err != nil {
		return Tool{}, err
	}
	existing, err := s.GetToolByID(ctx, id)
	if err != nil {
		return Tool{}, err
	}
	tool, err = normalizeToolDeclaration(tool)
	if err != nil {
		return Tool{}, err
	}
	if tool.Name != existing.Name {
		return Tool{}, fmt.Errorf("%w: name %q must match immutable name %q", ErrInvalidTool, tool.Name, existing.Name)
	}
	tool.ID = existing.ID
	tool.CreatedAt = existing.CreatedAt
	tool.UpdatedAt = s.now()
	retainDirectSecret(&tool, existing)
	tool, err = NormalizeTool(tool)
	if err != nil {
		return Tool{}, err
	}
	data, err := json.Marshal(tool)
	if err != nil {
		return Tool{}, fmt.Errorf("toolkit: encode tool %q: %w", tool.Name, err)
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
	tool, err := s.GetToolByID(ctx, id)
	if err != nil {
		return err
	}
	if err := store.BatchDelete(ctx, []kv.Key{toolKey(tool.ID), toolNameKey(tool.Name)}); err != nil {
		return fmt.Errorf("toolkit: delete tool %q: %w", tool.ID, err)
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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&tool); err != nil {
		return Tool{}, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return Tool{}, errors.New("multiple JSON values")
		}
		return Tool{}, err
	}
	return NormalizeTool(tool)
}

func toolKey(id string) kv.Key {
	return append(append(kv.Key{}, toolsRoot...), url.PathEscape(id))
}

func toolNameKey(name string) kv.Key {
	return append(append(kv.Key{}, toolsByNameRoot...), url.PathEscape(name))
}

func (s *Server) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return socialutil.NewID()
}

func retainDirectSecret(desired *Tool, existing Tool) {
	if desired.HTTP == nil || existing.HTTP == nil || desired.HTTP.Auth.Method != existing.HTTP.Auth.Method {
		return
	}
	switch desired.HTTP.Auth.Method {
	case "bearer":
		if desired.HTTP.Auth.BearerToken == nil {
			desired.HTTP.Auth.BearerToken = cloneStringPtr(existing.HTTP.Auth.BearerToken)
		}
	case "header_api_key":
		if desired.HTTP.Auth.APIKey == nil {
			desired.HTTP.Auth.APIKey = cloneStringPtr(existing.HTTP.Auth.APIKey)
		}
	}
}

// MergeDirectSecrets retains omitted direct secrets only when the auth method
// is unchanged. It returns an independently owned, executable declaration.
func MergeDirectSecrets(desired, existing Tool) (Tool, error) {
	desired = cloneTool(desired)
	retainDirectSecret(&desired, existing)
	return NormalizeTool(desired)
}
