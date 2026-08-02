package toolkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
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
	ids := orderedToolIDs(req.ProfileTools)
	tools := make([]Tool, 0, len(ids))
	for _, id := range ids {
		tool, err := b.Tools.GetToolByID(ctx, id)
		if errors.Is(err, ErrToolNotFound) {
			return ToolKit{}, fmt.Errorf("%w: RuntimeProfile references Tool ID %q", ErrToolNotFound, id)
		}
		if err != nil {
			return ToolKit{}, err
		}
		tools = append(tools, tool)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	allowedPolicy := toolIDSet(req.AllowedTools, req.RestrictTools || len(req.AllowedTools) > 0)
	out := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if !tool.Enabled {
			continue
		}
		if allowedPolicy != nil && !allowedPolicy[tool.ID] {
			continue
		}
		out = append(out, tool)
	}
	return ToolKit{Tools: cloneTools(out)}, nil
}

func orderedToolIDs(profile []string) []string {
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

func toolIDSet(ids []string, restrict bool) map[string]bool {
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
