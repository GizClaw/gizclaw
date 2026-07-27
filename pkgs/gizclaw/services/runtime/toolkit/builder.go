package toolkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type BuildRequest struct {
	CallerPublicKey string
	ProfileTools    []string
	AllowedTools    []string
	RestrictTools   bool
}

type Builder struct {
	Tools *Server
}

func (b *Builder) Build(ctx context.Context, req BuildRequest) (ToolKit, error) {
	if b == nil || b.Tools == nil {
		return ToolKit{}, ErrNotConfigured
	}
	names := orderedToolNames(req.ProfileTools)
	tools := make([]Tool, 0, len(names))
	for _, name := range names {
		tool, err := b.Tools.GetTool(ctx, name)
		if errors.Is(err, ErrToolNotFound) {
			return ToolKit{}, fmt.Errorf("%w: RuntimeProfile references %q", ErrToolNotFound, name)
		}
		if err != nil {
			return ToolKit{}, err
		}
		tools = append(tools, tool)
	}
	allowedPolicy := toolNameSet(req.AllowedTools, req.RestrictTools || len(req.AllowedTools) > 0)
	out := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if !tool.Enabled {
			continue
		}
		if allowedPolicy != nil && !allowedPolicy[tool.Name] {
			continue
		}
		out = append(out, tool)
	}
	return ToolKit{Tools: cloneTools(out)}, nil
}

func orderedToolNames(profile []string) []string {
	seen := make(map[string]struct{}, len(profile))
	out := make([]string, 0, len(profile))
	for _, id := range profile {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func toolNameSet(ids []string, restrict bool) map[string]bool {
	if !restrict {
		return nil
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func (tk ToolKit) Find(name string) (Tool, bool) {
	idx := slices.IndexFunc(tk.Tools, func(tool Tool) bool {
		return tool.Name == name
	})
	if idx < 0 {
		return Tool{}, false
	}
	return cloneTool(tk.Tools[idx]), true
}
