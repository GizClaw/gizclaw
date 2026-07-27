package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var toolsRoot = kv.Key{"by-name"}

type Server struct {
	Store kv.Store
	Now   func() time.Time
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
	data, err := store.Get(ctx, toolKey(name))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return Tool{}, ErrToolNotFound
		}
		return Tool{}, fmt.Errorf("toolkit: get tool %q: %w", name, err)
	}
	tool, err := decodeTool(data)
	if err != nil {
		return Tool{}, fmt.Errorf("toolkit: decode tool %q: %w", name, err)
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
	tool, err = normalizeToolDeclaration(tool)
	if err != nil {
		return Tool{}, err
	}
	now := s.now()
	if existing, err := s.GetTool(ctx, tool.Name); err == nil {
		tool.CreatedAt = existing.CreatedAt
		retainDirectSecret(&tool, existing)
	} else if !errors.Is(err, ErrToolNotFound) {
		return Tool{}, err
	} else {
		tool.CreatedAt = now
	}
	tool, err = NormalizeTool(tool)
	if err != nil {
		return Tool{}, err
	}
	tool.UpdatedAt = now
	data, err := json.Marshal(tool)
	if err != nil {
		return Tool{}, fmt.Errorf("toolkit: encode tool %q: %w", tool.Name, err)
	}
	if err := store.Set(ctx, toolKey(tool.Name), data); err != nil {
		return Tool{}, fmt.Errorf("toolkit: put tool %q: %w", tool.Name, err)
	}
	return cloneTool(tool), nil
}

func (s *Server) DeleteTool(ctx context.Context, name string) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	name, err = normalizeToolName(name)
	if err != nil {
		return err
	}
	if err := store.Delete(ctx, toolKey(name)); err != nil {
		return fmt.Errorf("toolkit: delete tool %q: %w", name, err)
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

func toolKey(name string) kv.Key {
	return append(append(kv.Key{}, toolsRoot...), url.PathEscape(name))
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
