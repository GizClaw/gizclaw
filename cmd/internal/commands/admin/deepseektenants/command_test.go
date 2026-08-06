package deepseektenantscmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewCmdExposesCompleteCRUD(t *testing.T) {
	cmd := NewCmd()
	for _, name := range []string{"list", "get", "create", "update", "delete"} {
		child, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("Find(%q) error = %v", name, err)
		}
		if child == nil || child.Name() != name {
			t.Fatalf("Find(%q) = %#v", name, child)
		}
	}
	for _, name := range []string{"create", "update"} {
		child, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("Find(%q) error = %v", name, err)
		}
		flag := child.Flag("credential-id")
		if flag == nil || flag.Annotations[cobraAnnotationBashCompOneRequiredFlag] == nil {
			t.Fatalf("%s --credential-id is not required", name)
		}
	}
}

func TestTenantCommandsRejectInvalidID(t *testing.T) {
	ctxName := ""
	commands := map[string]func(*string) *cobra.Command{
		"create": newCreateCmd,
		"update": newUpdateCmd,
		"delete": newDeleteCmd,
		"get":    newGetCmd,
	}
	for name, newCommand := range commands {
		t.Run(name, func(t *testing.T) {
			err := newCommand(&ctxName).RunE(nil, []string{" \t"})
			if err == nil || !strings.Contains(err.Error(), "id") {
				t.Fatalf("RunE() error = %v, want tenant ID validation error", err)
			}
		})
	}
}

func TestTenantIDRejectsWhitespace(t *testing.T) {
	if _, err := tenantID("  example  "); err == nil {
		t.Fatal("tenantID() error = nil")
	}
}

const cobraAnnotationBashCompOneRequiredFlag = "cobra_annotation_bash_completion_one_required_flag"
