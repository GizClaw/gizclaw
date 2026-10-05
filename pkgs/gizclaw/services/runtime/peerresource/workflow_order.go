package peerresource

import (
	"cmp"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func compareWorkflowAliases(bindings apitypes.RuntimeProfileWorkflows, left, right string) int {
	var leftOrder, rightOrder int32
	if order := bindings[left].SortOrder; order != nil {
		leftOrder = *order
	}
	if order := bindings[right].SortOrder; order != nil {
		rightOrder = *order
	}
	if result := cmp.Compare(leftOrder, rightOrder); result != 0 {
		return result
	}
	return strings.Compare(left, right)
}

func sortWorkflowAliases(aliases []string, bindings apitypes.RuntimeProfileWorkflows) {
	slices.SortFunc(aliases, func(left, right string) int {
		return compareWorkflowAliases(bindings, left, right)
	})
}
