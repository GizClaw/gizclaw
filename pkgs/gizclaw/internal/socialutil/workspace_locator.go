package socialutil

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// WorkspaceBindingLocator identifies the exact Social resource incarnation
// that created a Workspace. Locators survive retirement so an old Workspace
// cannot resolve to a later binding of the same relationship or group.
type WorkspaceBindingLocator struct {
	ResourceID    string `json:"resource_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
}

// Validate checks the locator's required identity fields.
func (l WorkspaceBindingLocator) Validate() error {
	for _, field := range []string{l.ResourceID, l.WorkspaceID, l.WorkspaceName} {
		if field == "" || field != strings.TrimSpace(field) {
			return errors.New("social: invalid workspace binding locator")
		}
	}
	return nil
}

// WorkspaceLocatorIDKey is an exact lookup key within one Social store.
func WorkspaceLocatorIDKey(id string) kv.Key {
	return kv.Key{"workspace-locators", "by-id", EscapeStoreSegment(id)}
}

// WorkspaceLocatorNameKey addresses the generated canonical Social Workspace
// name, not a peer's editable nickname or arbitrary key prefix.
func WorkspaceLocatorNameKey(name string) kv.Key {
	return kv.Key{"workspace-locators", "by-name", EscapeStoreSegment(name)}
}

// WorkspaceLocatorEntries must be committed atomically with the binding.
func WorkspaceLocatorEntries(locator WorkspaceBindingLocator) ([]kv.Entry, error) {
	if err := locator.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(locator)
	if err != nil {
		return nil, err
	}
	return []kv.Entry{{Key: WorkspaceLocatorIDKey(locator.WorkspaceID), Value: data}, {Key: WorkspaceLocatorNameKey(locator.WorkspaceName), Value: data}}, nil
}
