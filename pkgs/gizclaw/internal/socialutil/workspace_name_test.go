package socialutil

import (
	"strings"
	"testing"
)

func TestIsSFUWorkspaceName(t *testing.T) {
	for _, name := range []string{
		DirectWorkspaceName("peer-a:peer-b"),
		DirectWorkspaceIncarnationName("peer-a:peer-b", "incarnation"),
		GroupWorkspaceName("group-a"),
	} {
		if !IsSFUWorkspaceName(name) {
			t.Errorf("generated SFU name %q was not recognized", name)
		}
	}
	for _, name := range []string{
		"", "h106-chat", "social-direct-", "social-group-",
		"social-group-" + strings.Repeat("a", 19),
		"social-direct-" + strings.Repeat("a", 21),
		"social-group-" + strings.Repeat("g", 20),
		"social-direct-" + strings.Repeat("A", 20),
		"social-other-" + strings.Repeat("a", 20),
		" " + GroupWorkspaceName("group-a"),
		GroupWorkspaceName("group-a") + " ",
	} {
		if IsSFUWorkspaceName(name) {
			t.Errorf("noncanonical name %q was recognized as an SFU name", name)
		}
	}
}
