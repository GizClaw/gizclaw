//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	flowworkspace "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	"github.com/GizClaw/flowcraft/memory/retrieval"
	"github.com/GizClaw/flowcraft/memory/retrieval/bbh"
	"github.com/GizClaw/flowcraft/sdk/workspace"
)

type flowcraftResources struct {
	backend *flowworkspace.Backend
	index   retrieval.Index
}

func newFlowcraftResources(t *testing.T, profile string) flowcraftResources {
	t.Helper()
	root := filepath.Join(t.TempDir(), profile)
	metadataWorkspace, err := workspace.NewLocalWorkspace(filepath.Join(root, "metadata"))
	if err != nil {
		t.Fatal(err)
	}
	backend, err := flowworkspace.New(metadataWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	retrievalWorkspace, err := workspace.NewLocalWorkspace(filepath.Join(root, "retrieval"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := bbh.New(retrievalWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	return flowcraftResources{backend: backend, index: index}
}

func (r flowcraftResources) closer(store io.Closer) io.Closer {
	return closeGroup{store, r.index, r.backend}
}

type closeGroup []io.Closer

func (group closeGroup) Close() error {
	var err error
	for _, closer := range group {
		if closer != nil {
			err = errors.Join(err, closer.Close())
		}
	}
	return err
}
