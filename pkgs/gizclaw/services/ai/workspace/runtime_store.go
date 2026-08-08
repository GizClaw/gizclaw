package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	DialogID     string
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

type runtimeMetadata struct {
	DialogID string `json:"dialog_id,omitempty"`
}

func NewObjectRuntimeStore(objects objectstore.ObjectStore) ObjectRuntimeStore {
	return ObjectRuntimeStore{Objects: objects}
}

func (s ObjectRuntimeStore) PrepareWorkspace(ctx context.Context, workspaceID string) (Runtime, error) {
	rt, err := s.GetWorkspaceRuntime(ctx, workspaceID)
	if err != nil {
		return Runtime{}, err
	}
	if err := s.ensureRuntimeMetadata(ctx, &rt); err != nil {
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
	}
	if provider, ok := s.Objects.(objectstore.LocalDirProvider); ok {
		root, ok := provider.LocalDir()
		if ok && strings.TrimSpace(root) != "" {
			rt.LocalDir = filepath.Join(root, filepath.FromSlash(objectPrefix))
		}
	}
	metadata, err := s.readRuntimeMetadata(rt.ObjectPrefix)
	if err != nil {
		return Runtime{}, err
	}
	rt.DialogID = metadata.DialogID
	return rt, nil
}

func (s ObjectRuntimeStore) ensureRuntimeMetadata(ctx context.Context, rt *Runtime) error {
	if rt == nil {
		return fmt.Errorf("workspace: runtime is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(rt.DialogID) != "" {
		return nil
	}
	dialogID, err := newRuntimeDialogID()
	if err != nil {
		return err
	}
	metadata := runtimeMetadata{DialogID: dialogID}
	if err := s.writeRuntimeMetadata(rt.ObjectPrefix, metadata); err != nil {
		return err
	}
	rt.DialogID = dialogID
	return nil
}

func (s ObjectRuntimeStore) readRuntimeMetadata(objectPrefix string) (runtimeMetadata, error) {
	reader, err := s.Objects.Get(runtimeMetadataObject(objectPrefix))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return runtimeMetadata{}, nil
		}
		return runtimeMetadata{}, fmt.Errorf("workspace: read runtime metadata: %w", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return runtimeMetadata{}, fmt.Errorf("workspace: read runtime metadata: %w", err)
	}
	var metadata runtimeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return runtimeMetadata{}, fmt.Errorf("workspace: decode runtime metadata: %w", err)
	}
	return metadata, nil
}

func (s ObjectRuntimeStore) writeRuntimeMetadata(objectPrefix string, metadata runtimeMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("workspace: encode runtime metadata: %w", err)
	}
	if err := s.Objects.Put(runtimeMetadataObject(objectPrefix), bytes.NewReader(data)); err != nil {
		return fmt.Errorf("workspace: write runtime metadata: %w", err)
	}
	return nil
}

func runtimeMetadataObject(objectPrefix string) string {
	return strings.TrimRight(objectPrefix, "/") + "/runtime.json"
}

func newRuntimeDialogID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("workspace: generate runtime dialog id: %w", err)
	}
	return "dialog-" + hex.EncodeToString(random[:]), nil
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
