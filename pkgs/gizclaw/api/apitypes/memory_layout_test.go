package apitypes

import (
	"encoding/json"
	"testing"
)

func TestMemoryLayoutRejectsRetiredPolicy(t *testing.T) {
	var spec MemoryLayoutSpec
	if err := json.Unmarshal([]byte(`{"flowcraft":{},"mem0":{},"volc_mem0":{}}`), &spec); err == nil {
		t.Fatal("retired policy was silently accepted")
	}
}
