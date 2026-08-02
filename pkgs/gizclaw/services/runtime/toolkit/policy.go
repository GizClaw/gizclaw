package toolkit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// NormalizePolicy validates and returns a copy of a ToolKit exposure policy.
func NormalizePolicy(policy *apitypes.ToolkitPolicy) (*apitypes.ToolkitPolicy, error) {
	if policy == nil {
		return nil, nil
	}
	out := *policy
	if policy.ToolIds == nil {
		return &out, nil
	}
	seen := make(map[string]bool, len(*policy.ToolIds))
	ids := make([]string, 0, len(*policy.ToolIds))
	for _, raw := range *policy.ToolIds {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, fmt.Errorf("%w: tool_ids contains an empty Tool ID", ErrInvalidTool)
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
