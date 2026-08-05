package firmware

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const testSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestServerCRUDReleaseRollback(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	server := &Server{Store: kv.NewMemory(nil), Now: func() time.Time { return now }}

	created := createFirmware(t, server, firmwareUpsert("devkit",
		firmwareSlot("stable-1", "https://firmware.example/stable.tar.zlib", 101),
		firmwareSlot("beta-1", "https://firmware.example/beta.tar.zlib", 102),
		firmwareSlot("develop-1", "https://firmware.example/develop.tar.zlib", 103),
		firmwareSlot("pending-1", "https://firmware.example/pending.tar.zlib", 104),
	))

	releasedResponse, err := server.ReleaseFirmware(ctx, adminhttp.ReleaseFirmwareRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("ReleaseFirmware: %v", err)
	}
	released := apitypes.Firmware(releasedResponse.(adminhttp.ReleaseFirmware200JSONResponse))
	assertPackageURL(t, released.Slots.Develop, "https://firmware.example/beta.tar.zlib")
	assertPackageURL(t, released.Slots.Beta, "https://firmware.example/stable.tar.zlib")
	assertPackageURL(t, released.Slots.Stable, "https://firmware.example/pending.tar.zlib")
	if released.Slots.Pending.Package != nil || released.Slots.Pending.Description != nil {
		t.Fatalf("released pending = %#v, want empty", released.Slots.Pending)
	}

	rolledBackResponse, err := server.RollbackFirmware(ctx, adminhttp.RollbackFirmwareRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("RollbackFirmware: %v", err)
	}
	rolledBack := apitypes.Firmware(rolledBackResponse.(adminhttp.RollbackFirmware200JSONResponse))
	assertPackageURL(t, rolledBack.Slots.Stable, "https://firmware.example/stable.tar.zlib")
	assertPackageURL(t, rolledBack.Slots.Pending, "https://firmware.example/pending.tar.zlib")

	listResponse, err := server.ListFirmwares(ctx, adminhttp.ListFirmwaresRequestObject{})
	if err != nil {
		t.Fatalf("ListFirmwares: %v", err)
	}
	if got := len(adminhttp.FirmwareList(listResponse.(adminhttp.ListFirmwares200JSONResponse)).Items); got != 1 {
		t.Fatalf("ListFirmwares len = %d, want 1", got)
	}
}

func TestServerPutReplacesPackageConfiguration(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	nextTime := createdAt
	server := &Server{
		Store: kv.NewMemory(nil),
		Now: func() time.Time {
			current := nextTime
			nextTime = updatedAt
			return current
		},
	}
	created := createFirmware(t, server, firmwareUpsert("devkit", firmwareSlot("1.0.0", "https://firmware.example/1.0.0.tar.zlib", 100), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}))

	description := " updated firmware "
	update := firmwareUpsert("devkit", firmwareSlot("1.1.0", "https://firmware.example/1.1.0.tar.zlib?release=1", 200), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
	update.Description = &description
	response, err := server.PutFirmware(ctx, adminhttp.PutFirmwareRequestObject{Id: created.Id, Body: &update})
	if err != nil {
		t.Fatalf("PutFirmware: %v", err)
	}
	updated := apitypes.Firmware(response.(adminhttp.PutFirmware200JSONResponse))
	if updated.CreatedAt != createdAt || updated.UpdatedAt != updatedAt {
		t.Fatalf("timestamps = %s/%s, want %s/%s", updated.CreatedAt, updated.UpdatedAt, createdAt, updatedAt)
	}
	if updated.Description == nil || *updated.Description != "updated firmware" {
		t.Fatalf("description = %v", updated.Description)
	}
	assertPackageURL(t, updated.Slots.Stable, "https://firmware.example/1.1.0.tar.zlib?release=1")
	if updated.Slots.Stable.Package.Size != 200 {
		t.Fatalf("size = %d, want 200", updated.Slots.Stable.Package.Size)
	}

	getResponse, err := server.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("GetFirmware: %v", err)
	}
	got := apitypes.Firmware(getResponse.(adminhttp.GetFirmware200JSONResponse))
	assertPackageURL(t, got.Slots.Stable, "https://firmware.example/1.1.0.tar.zlib?release=1")

	deleteResponse, err := server.DeleteFirmware(ctx, adminhttp.DeleteFirmwareRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteFirmware: %v", err)
	}
	if item := apitypes.Firmware(deleteResponse.(adminhttp.DeleteFirmware200JSONResponse)); item.Name != "devkit" {
		t.Fatalf("deleted firmware = %#v", item)
	}
}

func TestServerValidatesPackageConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		pkg     apitypes.FirmwarePackage
		message string
	}{
		{name: "http", pkg: testPackage("http://firmware.example/fw.tar.zlib", testSHA256, 1), message: "absolute HTTPS"},
		{name: "relative", pkg: testPackage("firmware.tar.zlib", testSHA256, 1), message: "absolute HTTPS"},
		{name: "userinfo", pkg: testPackage("https://user@firmware.example/fw.tar.zlib", testSHA256, 1), message: "userinfo"},
		{name: "fragment", pkg: testPackage("https://firmware.example/fw.tar.zlib#secret", testSHA256, 1), message: "fragment"},
		{name: "invalid port", pkg: testPackage("https://firmware.example:not-a-port/fw.tar.zlib", testSHA256, 1), message: "absolute HTTPS"},
		{name: "empty port", pkg: testPackage("https://firmware.example:/fw.tar.zlib", testSHA256, 1), message: "valid HTTPS authority"},
		{name: "zero port", pkg: testPackage("https://firmware.example:0/fw.tar.zlib", testSHA256, 1), message: "valid HTTPS authority port"},
		{name: "out of range port", pkg: testPackage("https://firmware.example:65536/fw.tar.zlib", testSHA256, 1), message: "valid HTTPS authority port"},
		{name: "url too long", pkg: testPackage("https://firmware.example/"+strings.Repeat("a", maxFirmwarePackageURLBytes), testSHA256, 1), message: "at most 2048 bytes"},
		{name: "sha", pkg: testPackage("https://firmware.example/fw.tar.zlib", "bad", 1), message: "64 hexadecimal"},
		{name: "size zero", pkg: testPackage("https://firmware.example/fw.tar.zlib", testSHA256, 0), message: "between 1 and 9007199254740991"},
		{name: "size not exactly representable in JavaScript", pkg: testPackage("https://firmware.example/fw.tar.zlib", testSHA256, maxFirmwarePackageSize+1), message: "between 1 and 9007199254740991"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{Store: kv.NewMemory(nil)}
			input := firmwareUpsert("devkit", apitypes.FirmwareSlot{Package: &test.pkg}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
			response, err := server.CreateFirmware(context.Background(), adminhttp.CreateFirmwareRequestObject{Body: &input})
			if err != nil {
				t.Fatalf("CreateFirmware: %v", err)
			}
			bad, ok := response.(adminhttp.CreateFirmware400JSONResponse)
			if !ok {
				t.Fatalf("response = %T, want 400", response)
			}
			if !strings.Contains(bad.Error.Message, test.message) {
				t.Fatalf("message = %q, want %q", bad.Error.Message, test.message)
			}
		})
	}
}

func TestServerValidatesPeerVisibleStringLengths(t *testing.T) {
	tests := []struct {
		name    string
		input   adminhttp.FirmwareUpsert
		message string
	}{
		{
			name:    "name",
			input:   firmwareUpsert(strings.Repeat("n", maxFirmwareNameBytes+1), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}),
			message: "name must contain at most 256 bytes",
		},
		{
			name: "slot description",
			input: firmwareUpsert("devkit", apitypes.FirmwareSlot{
				Description: new(strings.Repeat("d", maxFirmwareSlotDescriptionBytes+1)),
			}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}),
			message: "description must contain at most 1024 bytes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{Store: kv.NewMemory(nil)}
			response, err := server.CreateFirmware(context.Background(), adminhttp.CreateFirmwareRequestObject{Body: &test.input})
			if err != nil {
				t.Fatalf("CreateFirmware: %v", err)
			}
			bad, ok := response.(adminhttp.CreateFirmware400JSONResponse)
			if !ok || !strings.Contains(bad.Error.Message, test.message) {
				t.Fatalf("response = %#v, want message %q", response, test.message)
			}
		})
	}
}

func TestServerNormalizesPackageConfiguration(t *testing.T) {
	server := &Server{Store: kv.NewMemory(nil)}
	upperSHA := strings.ToUpper(testSHA256)
	input := firmwareUpsert("devkit", apitypes.FirmwareSlot{Package: new(testPackage("  https://firmware.example/fw.tar.zlib?token=value  ", upperSHA, 7))}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
	created := createFirmware(t, server, input)
	if created.Slots.Stable.Package.Url != "https://firmware.example/fw.tar.zlib?token=value" {
		t.Fatalf("url = %q", created.Slots.Stable.Package.Url)
	}
	if created.Slots.Stable.Package.Sha256 != testSHA256 {
		t.Fatalf("sha256 = %q", created.Slots.Stable.Package.Sha256)
	}
}

func TestServerRejectsOperationLeavingStableEmpty(t *testing.T) {
	server := &Server{Store: kv.NewMemory(nil)}
	created := createFirmware(t, server, firmwareUpsert("devkit", firmwareSlot("stable", "https://firmware.example/stable.tar.zlib", 1), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}))
	if response, err := server.ReleaseFirmware(context.Background(), adminhttp.ReleaseFirmwareRequestObject{Id: created.Id}); err != nil {
		t.Fatalf("ReleaseFirmware: %v", err)
	} else if _, ok := response.(adminhttp.ReleaseFirmware409JSONResponse); !ok {
		t.Fatalf("ReleaseFirmware response = %T, want 409", response)
	}
}

func TestServerListFirmwaresPagination(t *testing.T) {
	server := &Server{Store: kv.NewMemory(nil)}
	for _, name := range []string{"a", "b", "c"} {
		createFirmware(t, server, firmwareUpsert(name, firmwareSlot(name, "https://firmware.example/"+name+".tar.zlib", 1), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}))
	}
	limit := int32(2)
	response, err := server.ListFirmwares(context.Background(), adminhttp.ListFirmwaresRequestObject{Params: adminhttp.ListFirmwaresParams{Limit: &limit}})
	if err != nil {
		t.Fatalf("ListFirmwares: %v", err)
	}
	page := adminhttp.FirmwareList(response.(adminhttp.ListFirmwares200JSONResponse))
	if len(page.Items) != 2 || !page.HasNext || page.NextCursor == nil {
		t.Fatalf("first page = %#v", page)
	}
}

func TestServerStoreNotConfigured(t *testing.T) {
	server := &Server{}
	response, err := server.ListFirmwares(context.Background(), adminhttp.ListFirmwaresRequestObject{})
	if err != nil {
		t.Fatalf("ListFirmwares: %v", err)
	}
	if _, ok := response.(adminhttp.ListFirmwares500JSONResponse); !ok {
		t.Fatalf("response = %T, want 500", response)
	}
}

func createFirmware(t *testing.T, server *Server, input adminhttp.FirmwareUpsert) apitypes.Firmware {
	t.Helper()
	response, err := server.CreateFirmware(context.Background(), adminhttp.CreateFirmwareRequestObject{Body: &input})
	if err != nil {
		t.Fatalf("CreateFirmware: %v", err)
	}
	created, ok := response.(adminhttp.CreateFirmware200JSONResponse)
	if !ok {
		t.Fatalf("CreateFirmware response = %T", response)
	}
	return apitypes.Firmware(created)
}

func firmwareUpsert(name string, stable, beta, develop, pending apitypes.FirmwareSlot) adminhttp.FirmwareUpsert {
	return adminhttp.FirmwareUpsert{Name: name, Slots: apitypes.FirmwareSlots{Stable: stable, Beta: beta, Develop: develop, Pending: pending}}
}

func firmwareSlot(description, packageURL string, size int64) apitypes.FirmwareSlot {
	return apitypes.FirmwareSlot{Description: &description, Package: new(testPackage(packageURL, testSHA256, size))}
}

func testPackage(packageURL, sha256 string, size int64) apitypes.FirmwarePackage {
	return apitypes.FirmwarePackage{Url: packageURL, Sha256: sha256, Size: size}
}

func assertPackageURL(t *testing.T, slot apitypes.FirmwareSlot, want string) {
	t.Helper()
	if slot.Package == nil || slot.Package.Url != want {
		t.Fatalf("package = %#v, want URL %q", slot.Package, want)
	}
}
