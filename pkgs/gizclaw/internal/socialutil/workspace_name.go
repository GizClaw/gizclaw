package socialutil

import "strings"

const (
	directWorkspaceNamePrefix = "social-direct-"
	groupWorkspaceNamePrefix  = "social-group-"
	workspaceNameHashLength   = 20
)

// IsSFUWorkspaceName reports whether name has the canonical generated Friend or
// Friend Group Workspace shape. It selects a lookup path, not an authorization:
// callers must still resolve the authoritative Social binding and membership.
func IsSFUWorkspaceName(name string) bool {
	hash, ok := strings.CutPrefix(name, directWorkspaceNamePrefix)
	if !ok {
		hash, ok = strings.CutPrefix(name, groupWorkspaceNamePrefix)
	}
	if !ok || len(hash) != workspaceNameHashLength {
		return false
	}
	for _, c := range hash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
