package memorystore

import (
	"strings"
	"testing"
)

func TestOpenSharedFlowcraftAcceptsCanonicalLayoutID(t *testing.T) {
	request := managedTestRequest(t)

	backend, err := openSharedFlowcraft(t.Context(), request)
	if err != nil {
		t.Fatalf("openSharedFlowcraft() error = %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
}

func TestOpenSharedFlowcraftRejectsMismatchedCanonicalLayoutID(t *testing.T) {
	request := managedTestRequest(t)
	request.Layout.Id = "different-layout-id"

	_, err := openSharedFlowcraft(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "different-layout-id" does not match binding layout_id "layout-id"`) {
		t.Fatalf("openSharedFlowcraft() error = %v", err)
	}
}
