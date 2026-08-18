package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

type Runtime struct {
	ObjectPrefix string
	LocalDir     string
	History      *HistoryStore
	OpenAI       *OpenAIStateStore
}

type RuntimeStore interface {
	// All Workspace arguments are canonical Workspace IDs.
	PrepareWorkspace(context.Context, string) (Runtime, error)
	GetWorkspaceRuntime(context.Context, string) (Runtime, error)
	DeleteWorkspaceRuntime(context.Context, string) error
}

// RuntimeCleanupStore adds physical absence verification for asynchronous
// Workspace retirement without widening the ordinary runtime surface.
type RuntimeCleanupStore interface {
	RuntimeStore
	WorkspaceRuntimeAbsent(context.Context, string) (bool, error)
}

type ObjectRuntimeStore struct {
	Objects objectstore.ObjectStore
}

func NewObjectRuntimeStore(objects objectstore.ObjectStore) ObjectRuntimeStore {
	return ObjectRuntimeStore{Objects: objects}
}

func (s ObjectRuntimeStore) PrepareWorkspace(ctx context.Context, workspaceID string) (Runtime, error) {
	rt, err := s.GetWorkspaceRuntime(ctx, workspaceID)
	if err != nil {
		return Runtime{}, err
	}
	if err := ctx.Err(); err != nil {
		return Runtime{}, err
	}
	if rt.LocalDir != "" {
		if err := os.MkdirAll(rt.LocalDir, 0o755); err != nil {
			return Runtime{}, fmt.Errorf("workspace: create runtime dir: %w", err)
		}
	}
	return rt, nil
}

func (s ObjectRuntimeStore) GetWorkspaceRuntime(_ context.Context, workspaceID string) (Runtime, error) {
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return Runtime{}, fmt.Errorf("workspace: invalid id: %w", err)
	}
	if s.Objects == nil {
		return Runtime{}, fmt.Errorf("workspace: runtime store is required")
	}
	objectPrefix := ObjectPrefix(workspaceID)
	rt := Runtime{
		ObjectPrefix: objectPrefix,
		History: &HistoryStore{
			Objects:        s.Objects,
			Workspace:      workspaceID,
			ObjectPrefix:   objectPrefix,
			AssetRetention: defaultHistoryAssetTTL,
		},
		OpenAI: NewOpenAIStateStore(s.Objects, objectPrefix),
	}
	if provider, ok := s.Objects.(objectstore.LocalDirProvider); ok {
		root, ok := provider.LocalDir()
		if ok && strings.TrimSpace(root) != "" {
			rt.LocalDir = filepath.Join(root, filepath.FromSlash(objectPrefix))
		}
	}
	return rt, nil
}

func (s ObjectRuntimeStore) DeleteWorkspaceRuntime(_ context.Context, workspaceID string) error {
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return fmt.Errorf("workspace: invalid id: %w", err)
	}
	if s.Objects == nil {
		return fmt.Errorf("workspace: runtime store is required")
	}
	if err := s.Objects.DeletePrefix(ObjectPrefix(workspaceID)); err != nil {
		return fmt.Errorf("workspace: delete runtime prefix: %w", err)
	}
	return nil
}

func (s ObjectRuntimeStore) WorkspaceRuntimeAbsent(_ context.Context, workspaceID string) (bool, error) {
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return false, fmt.Errorf("workspace: invalid id: %w", err)
	}
	if s.Objects == nil {
		return false, fmt.Errorf("workspace: runtime store is required")
	}
	objects, err := s.Objects.List(ObjectPrefix(workspaceID))
	if err != nil {
		return false, fmt.Errorf("workspace: list runtime prefix: %w", err)
	}
	return len(objects) == 0, nil
}

func ObjectPrefix(workspaceID string) string {
	return "workspaces/" + customid.OpaquePathSegment(workspaceID)
}
