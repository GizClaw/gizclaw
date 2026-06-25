//go:build gizclaw_e2e

package admin_test

import (
	"bytes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestAdminAPIFirmwaresListGetPaginationAndUpload(t *testing.T) {
	env := newAdminAPIHarness(t)

	all := collectAdminPages(t, 20, func(cursor *string, limit int32) ([]apitypes.Firmware, bool, *string) {
		resp, err := env.api.ListFirmwaresWithResponse(env.ctx, &adminservice.ListFirmwaresParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list firmwares: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list firmwares missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, all, "devkit-firmware-main", func(item apitypes.Firmware) string { return item.Name })
	requirePrefixCount(t, all, "devkit-firmware-", 70, func(item apitypes.Firmware) string { return item.Name })

	get, err := env.api.GetFirmwareWithResponse(env.ctx, "devkit-firmware-main")
	if err != nil {
		t.Fatalf("get firmware: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Slots.Stable.Version == nil || *get.JSON200.Slots.Stable.Version != "9.9.0" {
		t.Fatalf("get firmware = %#v", get.JSON200)
	}
	if get.JSON200.Slots.Stable.Artifacts == nil || len(*get.JSON200.Slots.Stable.Artifacts) != 1 || (*get.JSON200.Slots.Stable.Artifacts)[0].Path == nil {
		t.Fatalf("firmware stable artifact missing uploaded path: %#v", get.JSON200.Slots.Stable.Artifacts)
	}

	name := mutationName("firmware")
	_, _ = env.api.DeleteFirmwareWithResponse(env.ctx, name)
	created, err := env.api.CreateFirmwareWithResponse(env.ctx, adminservice.FirmwareUpsert{
		Name:        name,
		Description: ptr("Admin API mutation firmware"),
		Slots:       firmwareSlots("0.0.1", "main"),
	})
	if err != nil {
		t.Fatalf("create firmware: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	t.Cleanup(func() { _, _ = env.api.DeleteFirmwareWithResponse(env.ctx, name) })

	upload, err := env.api.UploadFirmwareBinWithBodyWithResponse(env.ctx, name, adminservice.Stable, "main", "application/octet-stream", bytes.NewReader([]byte("admin api firmware payload")))
	if err != nil {
		t.Fatalf("upload firmware bin: %v", err)
	}
	requireStatusOK(t, upload, upload.Body)
	if upload.JSON200 == nil || upload.JSON200.Slots.Stable.Artifacts == nil {
		t.Fatalf("upload firmware bin = %#v", upload.JSON200)
	}
}
