package deepseektenantscmd

import "testing"

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

const cobraAnnotationBashCompOneRequiredFlag = "cobra_annotation_bash_completion_one_required_flag"
