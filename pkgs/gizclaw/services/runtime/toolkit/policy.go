package toolkit

import (
	"fmt"
	"slices"
	"sort"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
)

// NormalizePolicy validates and returns a copy of a ToolKit exposure policy.
func NormalizePolicy(policy *apitypes.ToolkitPolicy) (*apitypes.ToolkitPolicy, error) {
	if policy == nil {
		return nil, nil
	}
	out := *policy
	if policy.ToolIds != nil && policy.ToolNames != nil {
		return nil, fmt.Errorf("%w: tool_ids and tool_names cannot be combined", ErrInvalidTool)
	}
	if policy.ToolNames != nil {
		names := append([]string{}, (*policy.ToolNames)...)
		for _, name := range names {
			if err := runtimealias.Validate("tool", name); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidTool, err)
			}
		}
		sort.Strings(names)
		names = slices.Compact(names)
		out.ToolNames = &names
	}
	if policy.ToolIds == nil {
		return &out, nil
	}
	seen := make(map[string]bool, len(*policy.ToolIds))
	ids := make([]string, 0, len(*policy.ToolIds))
	for _, id := range *policy.ToolIds {
		if err := customid.ValidateResourceID(id); err != nil {
			return nil, fmt.Errorf("%w: tool_ids: %v", ErrInvalidTool, err)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out.ToolIds = &ids
	return &out, nil
}
