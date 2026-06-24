package workspace

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

type Runtime struct {
	ObjectPrefix string
	LocalDir     string
	History      *HistoryStore
}

type RuntimeStore interface {
	PrepareWorkspace(context.Context, string) (Runtime, error)
	GetWorkspaceRuntime(context.Context, string) (Runtime, error)
	DeleteWorkspaceRuntime(context.Context, string) error
}

type ObjectRuntimeStore struct {
	Objects objectstore.ObjectStore
}

func NewObjectRuntimeStore(objects objectstore.ObjectStore) ObjectRuntimeStore {
	return ObjectRuntimeStore{Objects: objects}
}

func (s ObjectRuntimeStore) PrepareWorkspace(ctx context.Context, workspace string) (Runtime, error) {
	rt, err := s.GetWorkspaceRuntime(ctx, workspace)
	if err != nil {
		return Runtime{}, err
	}
	if rt.LocalDir != "" {
		if err := os.MkdirAll(rt.LocalDir, 0o755); err != nil {
			return Runtime{}, fmt.Errorf("workspace: create runtime dir: %w", err)
		}
	}
	return rt, nil
}

func (s ObjectRuntimeStore) GetWorkspaceRuntime(_ context.Context, workspace string) (Runtime, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return Runtime{}, fmt.Errorf("workspace: name is required")
	}
	if s.Objects == nil {
		return Runtime{}, fmt.Errorf("workspace: runtime store is required")
	}
	objectPrefix := ObjectPrefix(workspace)
	rt := Runtime{
		ObjectPrefix: objectPrefix,
		History: &HistoryStore{
			Objects:        s.Objects,
			Workspace:      workspace,
			ObjectPrefix:   objectPrefix,
			AssetRetention: defaultHistoryAssetTTL,
		},
	}
	if provider, ok := s.Objects.(objectstore.LocalDirProvider); ok {
		root, ok := provider.LocalDir()
		if ok && strings.TrimSpace(root) != "" {
			rt.LocalDir = filepath.Join(root, filepath.FromSlash(objectPrefix))
		}
	}
	return rt, nil
}

func (s ObjectRuntimeStore) DeleteWorkspaceRuntime(_ context.Context, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("workspace: name is required")
	}
	if s.Objects == nil {
		return fmt.Errorf("workspace: runtime store is required")
	}
	if err := s.Objects.DeletePrefix(ObjectPrefix(workspace)); err != nil {
		return fmt.Errorf("workspace: delete runtime prefix: %w", err)
	}
	return nil
}

func ObjectPrefix(workspace string) string {
	return "workspaces/" + url.PathEscape(workspace)
}
