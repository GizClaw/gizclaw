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
		flag := child.Flag("credential-name")
		if flag == nil || flag.Annotations[cobraAnnotationBashCompOneRequiredFlag] == nil {
			t.Fatalf("%s --credential-name is not required", name)
		}
	}
}

func TestTenantCommandsRejectBlankName(t *testing.T) {
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
			if err == nil || !strings.Contains(err.Error(), "tenant name") {
				t.Fatalf("RunE() error = %v, want tenant name validation error", err)
			}
		})
	}
}

func TestTenantNameTrimsWhitespace(t *testing.T) {
	got, err := tenantName("  example  ")
	if err != nil {
		t.Fatalf("tenantName() error = %v", err)
	}
	if got != "example" {
		t.Fatalf("tenantName() = %q, want example", got)
	}
}

const cobraAnnotationBashCompOneRequiredFlag = "cobra_annotation_bash_completion_one_required_flag"
