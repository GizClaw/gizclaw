//go:build gizclaw_e2e

package admin_test

import (
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const firmwarePackageSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestAdminAPIFirmwaresListGetAndConfigurePackages(t *testing.T) {
	env := newAdminAPIHarness(t)

	all := collectAdminPages(t, 20, func(cursor *string, limit int32) ([]apitypes.Firmware, bool, *string) {
		resp, err := env.api.ListFirmwaresWithResponse(env.ctx, &adminhttp.ListFirmwaresParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list firmwares: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list firmwares missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	seed := requireName(t, all, "devkit-firmware-main", func(item apitypes.Firmware) string { return item.Name })

	get, err := env.api.GetFirmwareWithResponse(env.ctx, seed.Id)
	if err != nil {
		t.Fatalf("get firmware: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Slots.Stable.Package == nil || get.JSON200.Slots.Stable.Package.Url != "https://firmware.example.invalid/devkit/stable.tar.zlib" {
		t.Fatalf("get firmware = %#v", get.JSON200)
	}

	name := mutationName("firmware")
	upsert := adminhttp.FirmwareUpsert{
		Name:        name,
		Description: ptr("Admin API mutation firmware"),
		Slots: apitypes.FirmwareSlots{
			Stable: apitypes.FirmwareSlot{
				Description: ptr("stable package"),
				Package: &apitypes.FirmwarePackage{
					Url:    "https://downloads.example.com/firmware/stable.tar.zlib",
					Sha256: firmwarePackageSHA256,
					Size:   4096,
				},
			},
			Beta: apitypes.FirmwareSlot{
				Description: ptr("beta package"),
				Package: &apitypes.FirmwarePackage{
					Url:    "https://downloads.example.com/firmware/beta.tar.zlib",
					Sha256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
					Size:   6144,
				},
			},
			Develop: apitypes.FirmwareSlot{
				Package: &apitypes.FirmwarePackage{
					Url:    "https://downloads.example.com/firmware/develop.tar.zlib",
					Sha256: "123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0",
					Size:   7168,
				},
			},
			Pending: apitypes.FirmwareSlot{
				Description: ptr("pending package"),
				Package: &apitypes.FirmwarePackage{
					Url:    "https://downloads.example.com/firmware/pending.tar.zlib?build=2",
					Sha256: firmwarePackageSHA256,
					Size:   8192,
				},
			},
		},
	}
	created, err := env.api.CreateFirmwareWithResponse(env.ctx, upsert)
	if err != nil {
		t.Fatalf("create firmware: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil {
		t.Fatalf("created firmware = %#v", created.JSON200)
	}
	requireFirmwareSlots(t, created.JSON200.Slots, upsert.Slots)
	firmwareID := created.JSON200.Id
	deleted := false
	t.Cleanup(func() {
		if !deleted {
			_, _ = env.api.DeleteFirmwareWithResponse(env.ctx, firmwareID)
		}
	})

	upsert.Description = ptr("Admin API updated firmware")
	put, err := env.api.PutFirmwareWithResponse(env.ctx, firmwareID, upsert)
	if err != nil {
		t.Fatalf("put firmware: %v", err)
	}
	requireStatusOK(t, put, put.Body)
	if put.JSON200 == nil || put.JSON200.Description == nil || *put.JSON200.Description != "Admin API updated firmware" {
		t.Fatalf("put firmware = %#v", put.JSON200)
	}
	requireFirmwareSlots(t, put.JSON200.Slots, upsert.Slots)

	gotUpdated, err := env.api.GetFirmwareWithResponse(env.ctx, firmwareID)
	if err != nil {
		t.Fatalf("get updated firmware: %v", err)
	}
	requireStatusOK(t, gotUpdated, gotUpdated.Body)
	if gotUpdated.JSON200 == nil {
		t.Fatalf("get updated firmware = %#v", gotUpdated.JSON200)
	}
	requireFirmwareSlots(t, gotUpdated.JSON200.Slots, upsert.Slots)

	listed := collectAdminPages(t, 20, func(cursor *string, limit int32) ([]apitypes.Firmware, bool, *string) {
		resp, err := env.api.ListFirmwaresWithResponse(env.ctx, &adminhttp.ListFirmwaresParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list updated firmwares: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatal("list updated firmwares missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	listedFirmware := requireName(t, listed, name, func(item apitypes.Firmware) string { return item.Name })
	requireFirmwareSlots(t, listedFirmware.Slots, upsert.Slots)

	released, err := env.api.ReleaseFirmwareWithResponse(env.ctx, created.JSON200.Id)
	if err != nil {
		t.Fatalf("release firmware: %v", err)
	}
	requireStatusOK(t, released, released.Body)
	if released.JSON200 == nil || released.JSON200.Slots.Stable.Package == nil || released.JSON200.Slots.Stable.Package.Size != 8192 || released.JSON200.Slots.Beta.Package == nil || released.JSON200.Slots.Beta.Package.Size != 4096 {
		t.Fatalf("release firmware = %#v", released.JSON200)
	}

	rolledBack, err := env.api.RollbackFirmwareWithResponse(env.ctx, created.JSON200.Id)
	if err != nil {
		t.Fatalf("rollback firmware: %v", err)
	}
	requireStatusOK(t, rolledBack, rolledBack.Body)
	if rolledBack.JSON200 == nil || rolledBack.JSON200.Slots.Stable.Package == nil || rolledBack.JSON200.Slots.Stable.Package.Size != 4096 {
		t.Fatalf("rollback firmware = %#v", rolledBack.JSON200)
	}

	invalidName := mutationName("firmware-invalid")
	invalid, err := env.api.CreateFirmwareWithResponse(env.ctx, adminhttp.FirmwareUpsert{
		Name: invalidName,
		Slots: apitypes.FirmwareSlots{Stable: apitypes.FirmwareSlot{Package: &apitypes.FirmwarePackage{
			Url: "https://downloads.example.com:0/firmware/stable.tar.zlib", Sha256: firmwarePackageSHA256, Size: 1,
		}}},
	})
	if err != nil {
		t.Fatalf("create invalid firmware request: %v", err)
	}
	if invalid.StatusCode() < 400 {
		t.Fatalf("create invalid firmware status = %d, body = %s", invalid.StatusCode(), invalid.Body)
	}

	removed, err := env.api.DeleteFirmwareWithResponse(env.ctx, firmwareID)
	if err != nil {
		t.Fatalf("delete firmware: %v", err)
	}
	requireStatusOK(t, removed, removed.Body)
	deleted = true
}

func requireFirmwareSlots(t *testing.T, got, want apitypes.FirmwareSlots) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("firmware slots = %#v, want %#v", got, want)
	}
}
