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
	if get.JSON200 == nil || get.JSON200.Slots.Stable.Artifact == nil || get.JSON200.Slots.Stable.Artifact.TarPath == "" {
		t.Fatalf("get firmware = %#v", get.JSON200)
	}

	name := mutationName("firmware")
	_, _ = env.api.DeleteFirmwareWithResponse(env.ctx, name)
	created, err := env.api.CreateFirmwareWithResponse(env.ctx, adminservice.FirmwareUpsert{
		Name:        name,
		Description: ptr("Admin API mutation firmware"),
		Slots:       firmwareSlots("Admin API stable firmware"),
	})
	if err != nil {
		t.Fatalf("create firmware: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	t.Cleanup(func() { _, _ = env.api.DeleteFirmwareWithResponse(env.ctx, name) })

	payload := adminFirmwareTarPayload(t, map[string]string{"firmware.bin": "admin api firmware payload"})
	upload, err := env.api.UploadFirmwareArtifactWithBodyWithResponse(env.ctx, name, adminservice.UploadFirmwareArtifactParamsChannelStable, "application/x-tar", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("upload firmware artifact: %v", err)
	}
	requireStatusOK(t, upload, upload.Body)
	if upload.JSON200 == nil || upload.JSON200.Slots.Stable.Artifact == nil {
		t.Fatalf("upload firmware artifact = %#v", upload.JSON200)
	}
	list, err := env.api.ListFirmwareArtifactEntriesWithResponse(env.ctx, name, adminservice.ListFirmwareArtifactEntriesParamsChannelStable, nil)
	if err != nil {
		t.Fatalf("list firmware artifact entries: %v", err)
	}
	requireStatusOK(t, list, list.Body)
	if list.JSON200 == nil || len(list.JSON200.Items) != 1 || list.JSON200.Items[0].Path != "firmware.bin" {
		t.Fatalf("artifact list = %#v", list.JSON200)
	}
}
