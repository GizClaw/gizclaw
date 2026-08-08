//go:build gizclaw_e2e

package admin_test

import (
	"fmt"
	"reflect"
	"strings"
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
	seed := requireName(t, all, "devkit-firmware-main", func(item apitypes.Firmware) string { return item.Id })

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
		Id:          name,
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
	if created.JSON200.Id != name {
		t.Fatalf("created firmware ID = %q, want caller ID %q", created.JSON200.Id, name)
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
	listedFirmware := requireName(t, listed, name, func(item apitypes.Firmware) string { return item.Id })
	requireFirmwareSlots(t, listedFirmware.Slots, upsert.Slots)

	legacyChannel := "pen" + "ding"
	for name, slots := range map[string]string{
		"missing develop": `"stable":{},"beta":{}`,
		"legacy channel":  `"stable":{},"beta":{},"develop":{},"` + legacyChannel + `":{}`,
		"unknown canary":  `"stable":{},"beta":{},"develop":{},"canary":{}`,
	} {
		t.Run(name, func(t *testing.T) {
			invalidID := mutationName("firmware-shape-" + strings.ReplaceAll(name, " ", "-"))
			response, requestErr := env.api.CreateFirmwareWithBodyWithResponse(
				env.ctx,
				"application/json",
				strings.NewReader(fmt.Sprintf(`{"id":%q,"slots":{%s}}`, invalidID, slots)),
			)
			if requestErr != nil {
				t.Fatalf("create invalid Firmware shape: %v", requestErr)
			}
			if response.StatusCode() != 400 {
				t.Fatalf("create invalid Firmware shape status = %d, body = %s", response.StatusCode(), response.Body)
			}
			got, getErr := env.api.GetFirmwareWithResponse(env.ctx, invalidID)
			if getErr != nil {
				t.Fatalf("get rejected Firmware: %v", getErr)
			}
			if got.StatusCode() != 404 {
				t.Fatalf("rejected Firmware was persisted: status=%d body=%s", got.StatusCode(), got.Body)
			}
		})
	}

	invalidName := mutationName("firmware-invalid")
	invalid, err := env.api.CreateFirmwareWithResponse(env.ctx, adminhttp.FirmwareUpsert{
		Id: invalidName,
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
