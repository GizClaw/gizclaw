package resourcemanager

import (
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
)

func TestResourceDisplayAssetRefsIgnoreUnknownRawFields(t *testing.T) {
	ref, err := asset.ParseRef("asset://7d9c87aa1a224de6b93082026f30c77e")
	if err != nil {
		t.Fatal(err)
	}
	resource := mustResource(t, fmt.Sprintf(`{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Workflow",
		"metadata":{"name":"asset-workflow"},
		"displays":{"icon":{"png":%q}},
		"spec":{"driver":"flowcraft","flowcraft":{"prompt":"asset test"}}
	}`, ref.String()))

	refs, err := resourceDisplayAssetRefs(resource)
	if err != nil {
		t.Fatalf("resourceDisplayAssetRefs() error = %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("resourceDisplayAssetRefs() = %v, want no refs", refs)
	}
}
