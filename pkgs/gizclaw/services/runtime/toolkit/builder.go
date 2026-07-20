package toolkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type AvailabilityChecker interface {
	ToolAvailable(context.Context, Tool) (bool, error)
}

type BuildRequest struct {
	CallerPublicKey string
	ProfileToolIDs  []string
	AllowedToolIDs  []string
	RestrictToolIDs bool
}

type Builder struct {
	Tools        *Server
	Availability AvailabilityChecker
}

func (b *Builder) Build(ctx context.Context, req BuildRequest) (ToolKit, error) {
	if b == nil || b.Tools == nil {
		return ToolKit{}, ErrNotConfigured
	}
	toolIDs := orderedToolIDs(req.ProfileToolIDs)
	tools := make([]Tool, 0, len(toolIDs))
	for _, id := range toolIDs {
		tool, err := b.Tools.GetTool(ctx, id)
		if errors.Is(err, ErrToolNotFound) {
			continue
		}
		if err != nil {
			return ToolKit{}, err
		}
		tools = append(tools, tool)
	}
	allowedPolicy := toolIDSet(req.AllowedToolIDs, req.RestrictToolIDs || len(req.AllowedToolIDs) > 0)
	out := make([]Tool, 0, len(tools))
	advertised := make(map[string]string)
	for _, tool := range tools {
		if !tool.Enabled {
			continue
		}
		if allowedPolicy != nil && !allowedPolicy[tool.ID] {
			continue
		}
		available, err := b.available(ctx, tool)
		if err != nil {
			return ToolKit{}, err
		}
		if !available {
			continue
		}
		name := effectiveToolName(tool)
		if previous, exists := advertised[name]; exists {
			return ToolKit{}, fmt.Errorf("%w: name %q conflicts between %q and %q", ErrDuplicateToolName, name, previous, tool.ID)
		}
		advertised[name] = tool.ID
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

func (b *Builder) available(ctx context.Context, tool Tool) (bool, error) {
	if b.Availability == nil {
		return true, nil
	}
	return b.Availability.ToolAvailable(ctx, tool)
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

func (tk ToolKit) Find(id string) (Tool, bool) {
	idx := slices.IndexFunc(tk.Tools, func(tool Tool) bool {
		return tool.ID == id
	})
	if idx < 0 {
		return Tool{}, false
	}
	return cloneTool(tk.Tools[idx]), true
}

func (tk ToolKit) FindByName(name string) (Tool, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tool{}, false
	}
	idx := slices.IndexFunc(tk.Tools, func(tool Tool) bool {
		return effectiveToolName(tool) == name
	})
	if idx < 0 {
		return Tool{}, false
	}
	return cloneTool(tk.Tools[idx]), true
}

func effectiveToolName(tool Tool) string {
	if name := trimPtr(tool.Name); name != "" {
		return name
	}
	return tool.ID
}
