package memorystore

import (
	"strings"
	"testing"
)

func TestOpenSharedFlowcraftRejectsMismatchedCanonicalLayoutID(t *testing.T) {
	request := supportedFlowcraftTestRequest(t)
	request.Layout.Id = "different-layout-id"

	_, err := openSharedFlowcraft(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "different-layout-id" does not match binding layout_id "layout-id"`) {
		t.Fatalf("openSharedFlowcraft() error = %v", err)
	}
}
