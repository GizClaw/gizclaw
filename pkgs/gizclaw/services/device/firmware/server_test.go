package firmware

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const stableChannel = "stable"

func TestServerCRUDReleaseRollback(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s := &Server{Store: kv.NewMemory(nil), Now: func() time.Time { return now }}

	create, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsert("devkit", "stable-1", "beta-1", "develop-1", "pending-1"))})
	if err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	}
	if _, ok := create.(adminservice.CreateFirmware200JSONResponse); !ok {
		t.Fatalf("CreateFirmware response = %T", create)
	}

	released, err := s.ReleaseFirmware(ctx, adminservice.ReleaseFirmwareRequestObject{Name: "devkit"})
	if err != nil {
		t.Fatalf("ReleaseFirmware error = %v", err)
	}
	releasedItem := apitypes.Firmware(released.(adminservice.ReleaseFirmware200JSONResponse))
	if got := slotDescription(releasedItem.Slots.Develop); got != "beta-1" {
		t.Fatalf("released develop = %q", got)
	}
	if got := slotDescription(releasedItem.Slots.Beta); got != "stable-1" {
		t.Fatalf("released beta = %q", got)
	}
	if got := slotDescription(releasedItem.Slots.Stable); got != "pending-1" {
		t.Fatalf("released stable = %q", got)
	}
	if slotDescription(releasedItem.Slots.Pending) != "" {
		t.Fatalf("released pending should be empty: %+v", releasedItem.Slots.Pending)
	}

	rolledBack, err := s.RollbackFirmware(ctx, adminservice.RollbackFirmwareRequestObject{Name: "devkit"})
	if err != nil {
		t.Fatalf("RollbackFirmware error = %v", err)
	}
	rolledBackItem := apitypes.Firmware(rolledBack.(adminservice.RollbackFirmware200JSONResponse))
	if got := slotDescription(rolledBackItem.Slots.Stable); got != "stable-1" {
		t.Fatalf("rolled back stable = %q", got)
	}
	if got := slotDescription(rolledBackItem.Slots.Pending); got != "pending-1" {
		t.Fatalf("rolled back pending = %q", got)
	}

	list, err := s.ListFirmwares(ctx, adminservice.ListFirmwaresRequestObject{})
	if err != nil {
		t.Fatalf("ListFirmwares error = %v", err)
	}
	if got := len(adminservice.FirmwareList(list.(adminservice.ListFirmwares200JSONResponse)).Items); got != 1 {
		t.Fatalf("ListFirmwares len = %d", got)
	}
}

func TestServerRejectsOperationLeavingStableEmpty(t *testing.T) {
	ctx := context.Background()
	s := &Server{Store: kv.NewMemory(nil)}
	if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsert("devkit", "stable-1", "", "", ""))}); err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	}
	if response, err := s.ReleaseFirmware(ctx, adminservice.ReleaseFirmwareRequestObject{Name: "devkit"}); err != nil {
		t.Fatalf("ReleaseFirmware error = %v", err)
	} else if _, ok := response.(adminservice.ReleaseFirmware409JSONResponse); !ok {
		t.Fatalf("ReleaseFirmware response = %T, want 409", response)
	}
	if response, err := s.RollbackFirmware(ctx, adminservice.RollbackFirmwareRequestObject{Name: "devkit"}); err != nil {
		t.Fatalf("RollbackFirmware error = %v", err)
	} else if _, ok := response.(adminservice.RollbackFirmware409JSONResponse); !ok {
		t.Fatalf("RollbackFirmware response = %T, want 409", response)
	}
}

func TestServerPutGetDeleteFirmware(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	nextTime := createdAt
	s := &Server{
		Store: kv.NewMemory(nil),
		Now: func() time.Time {
			out := nextTime
			nextTime = updatedAt
			return out
		},
	}

	put, err := s.PutFirmware(ctx, adminservice.PutFirmwareRequestObject{
		Name: "devkit",
		Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0")),
	})
	if err != nil {
		t.Fatalf("PutFirmware error = %v", err)
	}
	putItem := apitypes.Firmware(put.(adminservice.PutFirmware200JSONResponse))
	if putItem.CreatedAt != createdAt || putItem.UpdatedAt != createdAt {
		t.Fatalf("first put timestamps = %s/%s, want %s", putItem.CreatedAt, putItem.UpdatedAt, createdAt)
	}

	update := firmwareUpsertWithArtifact("devkit", "1.1.0")
	description := " updated firmware "
	update.Description = &description
	updated, err := s.PutFirmware(ctx, adminservice.PutFirmwareRequestObject{Name: "devkit", Body: ptr(update)})
	if err != nil {
		t.Fatalf("PutFirmware update error = %v", err)
	}
	updatedItem := apitypes.Firmware(updated.(adminservice.PutFirmware200JSONResponse))
	if updatedItem.CreatedAt != createdAt || updatedItem.UpdatedAt != updatedAt {
		t.Fatalf("updated timestamps = %s/%s, want %s/%s", updatedItem.CreatedAt, updatedItem.UpdatedAt, createdAt, updatedAt)
	}
	if updatedItem.Description == nil || *updatedItem.Description != "updated firmware" {
		t.Fatalf("updated description = %v", updatedItem.Description)
	}

	got, err := s.GetFirmware(ctx, adminservice.GetFirmwareRequestObject{Name: "devkit"})
	if err != nil {
		t.Fatalf("GetFirmware error = %v", err)
	}
	if item := apitypes.Firmware(got.(adminservice.GetFirmware200JSONResponse)); slotDescription(item.Slots.Stable) != "1.1.0" {
		t.Fatalf("GetFirmware stable = %+v", item.Slots.Stable)
	}

	deleted, err := s.DeleteFirmware(ctx, adminservice.DeleteFirmwareRequestObject{Name: "devkit"})
	if err != nil {
		t.Fatalf("DeleteFirmware error = %v", err)
	}
	if item := apitypes.Firmware(deleted.(adminservice.DeleteFirmware200JSONResponse)); item.Name != "devkit" {
		t.Fatalf("DeleteFirmware item = %+v", item)
	}
	if response, err := s.GetFirmware(ctx, adminservice.GetFirmwareRequestObject{Name: "devkit"}); err != nil {
		t.Fatalf("GetFirmware after delete error = %v", err)
	} else if _, ok := response.(adminservice.GetFirmware404JSONResponse); !ok {
		t.Fatalf("GetFirmware after delete response = %T, want 404", response)
	}
}

func TestServerCreateAndPutValidation(t *testing.T) {
	ctx := context.Background()
	s := &Server{Store: kv.NewMemory(nil)}

	if response, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{}); err != nil {
		t.Fatalf("CreateFirmware nil body error = %v", err)
	} else if _, ok := response.(adminservice.CreateFirmware400JSONResponse); !ok {
		t.Fatalf("CreateFirmware nil body response = %T, want 400", response)
	}
	if response, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsert("", "", "", "", ""))}); err != nil {
		t.Fatalf("CreateFirmware empty name error = %v", err)
	} else if _, ok := response.(adminservice.CreateFirmware400JSONResponse); !ok {
		t.Fatalf("CreateFirmware empty name response = %T, want 400", response)
	}
	if response, err := s.PutFirmware(ctx, adminservice.PutFirmwareRequestObject{Name: "devkit", Body: ptr(firmwareUpsert("other", "", "", "", ""))}); err != nil {
		t.Fatalf("PutFirmware name mismatch error = %v", err)
	} else if _, ok := response.(adminservice.PutFirmware400JSONResponse); !ok {
		t.Fatalf("PutFirmware name mismatch response = %T, want 400", response)
	}
	if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0"))}); err != nil {
		t.Fatalf("CreateFirmware first error = %v", err)
	}
	if response, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0"))}); err != nil {
		t.Fatalf("CreateFirmware duplicate error = %v", err)
	} else if _, ok := response.(adminservice.CreateFirmware409JSONResponse); !ok {
		t.Fatalf("CreateFirmware duplicate response = %T, want 409", response)
	}
}

func TestServerUploadFirmwareArtifactExtractsTarAndMetadata(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	assets := objectstore.Dir(t.TempDir())
	s := &Server{Store: kv.NewMemory(nil), Assets: assets, Now: func() time.Time { return now }}
	if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0"))}); err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	}

	payload := tarPayload(t, map[string]string{
		"firmware.bin":       "firmware-app",
		"assets/readme.txt":  "hello asset",
		"assets/icons/app":   "icon",
		"assets/icons/.keep": "",
	})
	resp, err := s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
		Body:    bytes.NewReader(payload),
	})
	if err != nil {
		t.Fatalf("UploadFirmwareArtifact error = %v", err)
	}
	item := apitypes.Firmware(resp.(adminservice.UploadFirmwareArtifact200JSONResponse))
	artifact := item.Slots.Stable.Artifact
	if artifact == nil {
		t.Fatalf("stable artifacts = %+v", artifact)
	}
	if artifact.TarPath != "devkit/stable/artifact/artifact.tar" {
		t.Fatalf("tar path = %q", artifact.TarPath)
	}
	if artifact.ManifestPath != "devkit/stable/artifact/manifest.json" {
		t.Fatalf("manifest path = %q", artifact.ManifestPath)
	}
	if artifact.FilesPath != "devkit/stable/artifact/files" {
		t.Fatalf("files path = %q", artifact.FilesPath)
	}
	if artifact.Size != int64(len(payload)) {
		t.Fatalf("size = %v", artifact.Size)
	}
	wantSHA := sha256.Sum256(payload)
	if artifact.Sha256 != hex.EncodeToString(wantSHA[:]) {
		t.Fatalf("sha256 = %v", artifact.Sha256)
	}
	if artifact.ContentType != "application/x-tar" {
		t.Fatalf("content type = %v", artifact.ContentType)
	}
	if !artifact.UploadedAt.Equal(now) {
		t.Fatalf("uploaded at = %v", artifact.UploadedAt)
	}
	reader, err := assets.Get(artifact.TarPath)
	if err != nil {
		t.Fatalf("Get uploaded object: %v", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Read uploaded object: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("uploaded object = %q", data)
	}

	list, err := s.ListFirmwareArtifactEntries(ctx, adminservice.ListFirmwareArtifactEntriesRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
	})
	if err != nil {
		t.Fatalf("ListFirmwareArtifactEntries error = %v", err)
	}
	root := apitypes.FirmwareArtifactList(list.(adminservice.ListFirmwareArtifactEntries200JSONResponse))
	if entryPaths(root.Items) != "assets,firmware.bin" {
		t.Fatalf("root items = %+v", root.Items)
	}

	tree, err := s.TreeFirmwareArtifactEntries(ctx, adminservice.TreeFirmwareArtifactEntriesRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
	})
	if err != nil {
		t.Fatalf("TreeFirmwareArtifactEntries error = %v", err)
	}
	all := apitypes.FirmwareArtifactTree(tree.(adminservice.TreeFirmwareArtifactEntries200JSONResponse))
	if got := entryPaths(all.Items); got != "assets,assets/icons,assets/icons/.keep,assets/icons/app,assets/readme.txt,firmware.bin" {
		t.Fatalf("tree paths = %s", got)
	}

	assetDir := "assets"
	stat, err := s.StatFirmwareArtifactEntry(ctx, adminservice.StatFirmwareArtifactEntryRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
		Params:  adminservice.StatFirmwareArtifactEntryParams{Path: &assetDir},
	})
	if err != nil {
		t.Fatalf("StatFirmwareArtifactEntry error = %v", err)
	}
	stats := apitypes.FirmwareArtifactStats(stat.(adminservice.StatFirmwareArtifactEntry200JSONResponse))
	if stats.FilesCount != 3 || stats.TotalSize != int64(len("hello asset")+len("icon")) {
		t.Fatalf("stats = %+v", stats)
	}

	entry, err := s.DownloadFirmwareArtifactEntry(ctx, adminservice.DownloadFirmwareArtifactEntryRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
		Params:  adminservice.DownloadFirmwareArtifactEntryParams{Path: "assets/readme.txt"},
	})
	if err != nil {
		t.Fatalf("DownloadFirmwareArtifactEntry error = %v", err)
	}
	entryBody, err := io.ReadAll(entry.(adminservice.DownloadFirmwareArtifactEntry200ApplicationoctetStreamResponse).Body)
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if string(entryBody) != "hello asset" {
		t.Fatalf("entry body = %q", entryBody)
	}

	if conflict, err := s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
		Body:    bytes.NewReader(payload),
	}); err != nil {
		t.Fatalf("UploadFirmwareArtifact conflict error = %v", err)
	} else if _, ok := conflict.(adminservice.UploadFirmwareArtifact409JSONResponse); !ok {
		t.Fatalf("UploadFirmwareArtifact conflict response = %T, want 409", conflict)
	}
}

func TestServerDeleteFirmwareArtifactAllowsReupload(t *testing.T) {
	ctx := context.Background()
	assets := objectstore.Dir(t.TempDir())
	s := &Server{Store: kv.NewMemory(nil), Assets: assets}
	if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0"))}); err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	}
	payload := tarPayload(t, map[string]string{"firmware.bin": "payload"})
	if _, err := s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{Name: "devkit", Channel: stableChannel, Body: bytes.NewReader(payload)}); err != nil {
		t.Fatalf("UploadFirmwareArtifact error = %v", err)
	}

	resp, err := s.DeleteFirmwareArtifact(ctx, adminservice.DeleteFirmwareArtifactRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
	})
	if err != nil {
		t.Fatalf("DeleteFirmwareArtifact error = %v", err)
	}
	item := apitypes.Firmware(resp.(adminservice.DeleteFirmwareArtifact200JSONResponse))
	if item.Slots.Stable.Artifact != nil {
		t.Fatalf("artifact after delete = %+v", item.Slots.Stable.Artifact)
	}
	objects, err := assets.List("devkit/stable/artifact")
	if err != nil {
		t.Fatalf("List artifact objects: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("artifact objects after delete = %+v", objects)
	}
	if _, err := s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{Name: "devkit", Channel: stableChannel, Body: bytes.NewReader(payload)}); err != nil {
		t.Fatalf("UploadFirmwareArtifact after delete error = %v", err)
	}
}

func TestServerUploadFirmwareArtifactRejectsUnsafeTarPath(t *testing.T) {
	ctx := context.Background()
	assets := objectstore.Dir(t.TempDir())
	s := &Server{Store: kv.NewMemory(nil), Assets: assets}
	if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0"))}); err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	}
	payload := tarPayload(t, map[string]string{"../bad.bin": "payload"})
	resp, err := s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
		Body:    bytes.NewReader(payload),
	})
	if err != nil {
		t.Fatalf("UploadFirmwareArtifact error = %v", err)
	}
	if _, ok := resp.(adminservice.UploadFirmwareArtifact400JSONResponse); !ok {
		t.Fatalf("UploadFirmwareArtifact response = %T, want 400", resp)
	}
}

func TestServerUploadFirmwareArtifactRejectsTruncatedTarEntry(t *testing.T) {
	ctx := context.Background()
	assets := objectstore.Dir(t.TempDir())
	s := &Server{Store: kv.NewMemory(nil), Assets: assets}
	if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0"))}); err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	}
	resp, err := s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
		Body:    bytes.NewReader(truncatedTarPayload(t)),
	})
	if err != nil {
		t.Fatalf("UploadFirmwareArtifact error = %v", err)
	}
	if _, ok := resp.(adminservice.UploadFirmwareArtifact400JSONResponse); !ok {
		t.Fatalf("UploadFirmwareArtifact response = %T, want 400", resp)
	}
	objects, err := assets.List(firmwareArtifactPrefix("devkit", stableChannel))
	if err != nil {
		t.Fatalf("List artifact prefix after truncated upload: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("artifact objects after truncated upload = %+v", objects)
	}
	validPayload := tarPayload(t, map[string]string{"firmware.bin": "payload"})
	resp, err = s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{
		Name:    "devkit",
		Channel: stableChannel,
		Body:    bytes.NewReader(validPayload),
	})
	if err != nil {
		t.Fatalf("UploadFirmwareArtifact retry error = %v", err)
	}
	if _, ok := resp.(adminservice.UploadFirmwareArtifact200JSONResponse); !ok {
		t.Fatalf("UploadFirmwareArtifact retry response = %T, want 200", resp)
	}
}

func TestServerUploadFirmwareArtifactRejectsTarWithoutFilesAndPathConflicts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{
			name:    "directories only",
			payload: tarPayloadWithEntries(t, []tarTestEntry{{name: "assets", kind: tar.TypeDir}}),
		},
		{
			name: "file then directory conflict",
			payload: tarPayloadWithEntries(t, []tarTestEntry{
				{name: "assets", body: "payload", kind: tar.TypeReg},
				{name: "assets", kind: tar.TypeDir},
			}),
		},
		{
			name: "file then child conflict",
			payload: tarPayloadWithEntries(t, []tarTestEntry{
				{name: "assets", body: "payload", kind: tar.TypeReg},
				{name: "assets/readme.txt", body: "child", kind: tar.TypeReg},
			}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			assets := objectstore.Dir(t.TempDir())
			s := &Server{Store: kv.NewMemory(nil), Assets: assets}
			if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0"))}); err != nil {
				t.Fatalf("CreateFirmware error = %v", err)
			}
			resp, err := s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{
				Name:    "devkit",
				Channel: stableChannel,
				Body:    bytes.NewReader(tc.payload),
			})
			if err != nil {
				t.Fatalf("UploadFirmwareArtifact error = %v", err)
			}
			if _, ok := resp.(adminservice.UploadFirmwareArtifact400JSONResponse); !ok {
				t.Fatalf("UploadFirmwareArtifact response = %T, want 400", resp)
			}
		})
	}
}

func TestServerPutPreservesUploadedArtifact(t *testing.T) {
	ctx := context.Background()
	assets := objectstore.Dir(t.TempDir())
	s := &Server{Store: kv.NewMemory(nil), Assets: assets}
	if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact("devkit", "1.0.0"))}); err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	}
	payload := tarPayload(t, map[string]string{"firmware.bin": "payload"})
	if _, err := s.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{Name: "devkit", Channel: stableChannel, Body: bytes.NewReader(payload)}); err != nil {
		t.Fatalf("UploadFirmwareArtifact error = %v", err)
	}

	update := firmwareUpsertWithArtifact("devkit", "1.1.0")
	updated, err := s.PutFirmware(ctx, adminservice.PutFirmwareRequestObject{Name: "devkit", Body: ptr(update)})
	if err != nil {
		t.Fatalf("PutFirmware preserving metadata error = %v", err)
	}
	item := apitypes.Firmware(updated.(adminservice.PutFirmware200JSONResponse))
	artifact := item.Slots.Stable.Artifact
	if artifact == nil || artifact.TarPath != "devkit/stable/artifact/artifact.tar" {
		t.Fatalf("preserved artifact = %+v", artifact)
	}
}

func TestServerListFirmwaresPagination(t *testing.T) {
	ctx := context.Background()
	s := &Server{Store: kv.NewMemory(nil)}
	for _, name := range []string{"devkit", "p4_func_ev", "waveshare"} {
		if _, err := s.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: ptr(firmwareUpsertWithArtifact(name, "1.0.0"))}); err != nil {
			t.Fatalf("CreateFirmware(%s) error = %v", name, err)
		}
	}

	limit := int32(2)
	first, err := s.ListFirmwares(ctx, adminservice.ListFirmwaresRequestObject{Params: adminservice.ListFirmwaresParams{Limit: &limit}})
	if err != nil {
		t.Fatalf("ListFirmwares first error = %v", err)
	}
	firstPage := adminservice.FirmwareList(first.(adminservice.ListFirmwares200JSONResponse))
	if len(firstPage.Items) != 2 || !firstPage.HasNext || firstPage.NextCursor == nil {
		t.Fatalf("first page = %+v", firstPage)
	}

	second, err := s.ListFirmwares(ctx, adminservice.ListFirmwaresRequestObject{Params: adminservice.ListFirmwaresParams{Cursor: firstPage.NextCursor, Limit: &limit}})
	if err != nil {
		t.Fatalf("ListFirmwares second error = %v", err)
	}
	secondPage := adminservice.FirmwareList(second.(adminservice.ListFirmwares200JSONResponse))
	if len(secondPage.Items) != 1 || secondPage.HasNext || secondPage.NextCursor != nil {
		t.Fatalf("second page = %+v", secondPage)
	}
}

func TestServerStoreNotConfigured(t *testing.T) {
	ctx := context.Background()
	s := &Server{}
	if response, err := s.ListFirmwares(ctx, adminservice.ListFirmwaresRequestObject{}); err != nil {
		t.Fatalf("ListFirmwares error = %v", err)
	} else if _, ok := response.(adminservice.ListFirmwares500JSONResponse); !ok {
		t.Fatalf("ListFirmwares response = %T, want 500", response)
	}
	if response, err := s.GetFirmware(ctx, adminservice.GetFirmwareRequestObject{Name: "devkit"}); err != nil {
		t.Fatalf("GetFirmware error = %v", err)
	} else if _, ok := response.(adminservice.GetFirmware500JSONResponse); !ok {
		t.Fatalf("GetFirmware response = %T, want 500", response)
	}
}

func firmwareUpsert(name, stable, beta, develop, pending string) adminservice.FirmwareUpsert {
	return adminservice.FirmwareUpsert{
		Name: name,
		Slots: apitypes.FirmwareSlots{
			Stable:  firmwareSlot(stable),
			Beta:    firmwareSlot(beta),
			Develop: firmwareSlot(develop),
			Pending: firmwareSlot(pending),
		},
	}
}

func firmwareUpsertWithArtifact(name, stable string) adminservice.FirmwareUpsert {
	return firmwareUpsert(name, stable, "", "", "")
}

func firmwareSlot(version string) apitypes.FirmwareSlot {
	if version == "" {
		return apitypes.FirmwareSlot{}
	}
	return apitypes.FirmwareSlot{Description: &version}
}

func slotDescription(slot apitypes.FirmwareSlot) string {
	if slot.Description == nil {
		return ""
	}
	return *slot.Description
}

func ptr[T any](value T) *T {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func tarPayload(t *testing.T, files map[string]string) []byte {
	t.Helper()
	entries := make([]tarTestEntry, 0, len(files))
	for name, body := range files {
		entries = append(entries, tarTestEntry{name: name, body: body, kind: tar.TypeReg})
	}
	return tarPayloadWithEntries(t, entries)
}

type tarTestEntry struct {
	name string
	body string
	kind byte
}

func tarPayloadWithEntries(t *testing.T, entries []tarTestEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, entry := range entries {
		data := []byte(entry.body)
		header := &tar.Header{Name: entry.name, Mode: 0644, ModTime: modTime, Typeflag: entry.kind}
		if entry.kind == tar.TypeDir {
			header.Mode = 0755
		} else {
			header.Size = int64(len(data))
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%s): %v", entry.name, err)
		}
		if entry.kind != tar.TypeDir {
			if _, err := tw.Write(data); err != nil {
				t.Fatalf("Write(%s): %v", entry.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close tar: %v", err)
	}
	return buf.Bytes()
}

func truncatedTarPayload(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "firmware.bin", Mode: 0644, Size: 8, Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("WriteHeader truncated tar: %v", err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatalf("Write truncated tar body: %v", err)
	}
	return buf.Bytes()
}

func entryPaths(entries []apitypes.FirmwareArtifactEntry) string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	return strings.Join(paths, ",")
}

type failSetStore struct {
	kv.Store
}

func (s failSetStore) Set(context.Context, kv.Key, []byte) error {
	return errors.New("set failed")
}
