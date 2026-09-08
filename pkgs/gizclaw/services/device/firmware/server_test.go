package firmware

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/api"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const testSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestServerCRUDDeclarativeChannels(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	server := &Server{DB: newTestDatabase(t), Now: func() time.Time { return now }}

	created := createFirmware(t, server, firmwareUpsert("devkit",
		firmwareSlot("stable-1", "https://firmware.example/stable.tar.zlib", 101),
		firmwareSlot("beta-1", "https://firmware.example/beta.tar.zlib", 102),
		firmwareSlot("develop-1", "https://firmware.example/develop.tar.zlib", 103),
	))
	assertPackageURL(t, created.Slots.Stable, "https://firmware.example/stable.tar.zlib")
	assertPackageURL(t, created.Slots.Beta, "https://firmware.example/beta.tar.zlib")
	assertPackageURL(t, created.Slots.Develop, "https://firmware.example/develop.tar.zlib")

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
		DB: newTestDatabase(t),
		Now: func() time.Time {
			current := nextTime
			nextTime = updatedAt
			return current
		},
	}
	created := createFirmware(t, server, firmwareUpsert("devkit",
		firmwareSlot("1.0.0", "https://firmware.example/1.0.0.tar.zlib", 100),
		firmwareSlot("beta", "https://firmware.example/beta.tar.zlib", 101),
		firmwareSlot("develop", "https://firmware.example/develop.tar.zlib", 102),
	))

	description := " updated firmware "
	update := firmwareUpsert("devkit",
		firmwareSlot("1.1.0", "https://firmware.example/1.1.0.tar.zlib?build=1", 200),
		firmwareSlot("beta", "https://firmware.example/beta.tar.zlib", 101),
		firmwareSlot("develop", "https://firmware.example/develop.tar.zlib", 102),
	)
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
	assertPackageURL(t, updated.Slots.Stable, "https://firmware.example/1.1.0.tar.zlib?build=1")
	if updated.Slots.Stable.Package.Size != 200 {
		t.Fatalf("size = %d, want 200", updated.Slots.Stable.Package.Size)
	}
	assertPackageURL(t, updated.Slots.Beta, "https://firmware.example/beta.tar.zlib")
	assertPackageURL(t, updated.Slots.Develop, "https://firmware.example/develop.tar.zlib")

	getResponse, err := server.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("GetFirmware: %v", err)
	}
	got := apitypes.Firmware(getResponse.(adminhttp.GetFirmware200JSONResponse))
	assertPackageURL(t, got.Slots.Stable, "https://firmware.example/1.1.0.tar.zlib?build=1")

	deleteResponse, err := server.DeleteFirmware(ctx, adminhttp.DeleteFirmwareRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteFirmware: %v", err)
	}
	if item := apitypes.Firmware(deleteResponse.(adminhttp.DeleteFirmware200JSONResponse)); item.Id != "devkit" {
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
			server := &Server{DB: newTestDatabase(t)}
			input := firmwareUpsert("devkit", apitypes.FirmwareSlot{Package: &test.pkg}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
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
			name: "slot description",
			input: firmwareUpsert("devkit", apitypes.FirmwareSlot{
				Description: new(strings.Repeat("d", maxFirmwareSlotDescriptionBytes+1)),
			}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}),
			message: "description must contain at most 1024 bytes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{DB: newTestDatabase(t)}
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
	server := &Server{DB: newTestDatabase(t)}
	upperSHA := strings.ToUpper(testSHA256)
	input := firmwareUpsert("devkit", apitypes.FirmwareSlot{Package: new(testPackage("  https://firmware.example/fw.tar.zlib?token=value  ", upperSHA, 7))}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
	created := createFirmware(t, server, input)
	if created.Slots.Stable.Package.Url != "https://firmware.example/fw.tar.zlib?token=value" {
		t.Fatalf("url = %q", created.Slots.Stable.Package.Url)
	}
	if created.Slots.Stable.Package.Sha256 != testSHA256 {
		t.Fatalf("sha256 = %q", created.Slots.Stable.Package.Sha256)
	}
}

func TestServerListFirmwaresPagination(t *testing.T) {
	server := &Server{DB: newTestDatabase(t)}
	for _, name := range []string{"a", "b", "c"} {
		createFirmware(t, server, firmwareUpsert(name, firmwareSlot(name, "https://firmware.example/"+name+".tar.zlib", 1), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}))
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

func firmwareUpsert(id string, stable, beta, develop apitypes.FirmwareSlot) adminhttp.FirmwareUpsert {
	return adminhttp.FirmwareUpsert{Id: id, Slots: apitypes.FirmwareSlots{Stable: stable, Beta: beta, Develop: develop}}
}

func firmwareSlot(description, packageURL string, size int64) apitypes.FirmwareSlot {
	return apitypes.FirmwareSlot{Description: &description, Package: new(testPackage(packageURL, testSHA256, size))}
}

func testPackage(packageURL, sha256 string, size int64) apitypes.FirmwarePackage {
	return apitypes.FirmwarePackage{Version: new("1.2.3"), Url: packageURL, Sha256: sha256, Size: size}
}

func assertPackageURL(t *testing.T, slot apitypes.FirmwareSlot, want string) {
	t.Helper()
	if slot.Package == nil || slot.Package.Url != want {
		t.Fatalf("package = %#v, want URL %q", slot.Package, want)
	}
}

func newTestDatabase(t testing.TB) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := (&Server{DB: db}).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSQLPaginationUsesIndexedBoundedRange(t *testing.T) {
	db := newTestDatabase(t)
	slots, err := json.Marshal(apitypes.FirmwareSlots{})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	for i := range 2000 {
		_, err := tx.ExecContext(t.Context(), `INSERT INTO firmwares(id,slots_json,created_at,updated_at) VALUES (?,?,?,?)`, fmt.Sprintf("firmware-%04d", i), string(slots), "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
		if err != nil {
			t.Fatal(err)
		}
	}
	// A malformed row outside the requested range must not be fetched or decoded.
	if _, err := tx.ExecContext(t.Context(), `INSERT INTO firmwares(id,slots_json,created_at,updated_at) VALUES ('z-corrupt','invalid','invalid','invalid')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	items, more, cursor, err := listFirmwarePage(t.Context(), db, "firmware-0999", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 || !more || cursor == nil || *cursor != "firmware-1009" || items[0].Id != "firmware-1000" {
		t.Fatalf("page = %v, %v, %v", items, more, cursor)
	}
	var id, parent, unused int
	var plan string
	if err := db.QueryRowContext(t.Context(), `EXPLAIN QUERY PLAN SELECT `+firmwareColumns+` FROM firmwares WHERE id>? ORDER BY id LIMIT ?`, "firmware-0999", 11).Scan(&id, &parent, &unused, &plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "SEARCH") || !strings.Contains(plan, "INDEX") {
		t.Fatalf("pagination lacks indexed range: %s", plan)
	}
}

func TestFirmwareRequestsDoNotInitializeSchema(t *testing.T) {
	db := newTestDatabase(t)
	if _, err := db.ExecContext(t.Context(), `DROP TABLE firmwares`); err != nil {
		t.Fatal(err)
	}
	response, err := (&Server{DB: db}).ListFirmwares(t.Context(), adminhttp.ListFirmwaresRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.ListFirmwares500JSONResponse); !ok {
		t.Fatalf("request recreated missing schema: %T", response)
	}
}

func TestPackageVersionValidationAndSchema(t *testing.T) {
	data, err := api.Files.ReadFile("http/shared/firmware_package.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		t.Fatal(err)
	}
	schema := doc.Components.Schemas["FirmwarePackage"].Value
	cases := []struct {
		version string
		valid   bool
	}{
		{"0.0.0", true}, {"1.2.3", true}, {"1.5.0-beta.1+abc123", true},
		{"1.2.3-0", true}, {"1.2.3-01a", true}, {"1.2.3+001", true},
		{"999999999999999999999999999.2.3", true},
		{"1.2.3+" + strings.Repeat("a", 122), true},
		{"", false}, {"v1.2.3", false}, {"1.2", false}, {"1.2.3.4", false},
		{"01.2.3", false}, {"1.02.3", false}, {"1.2.03", false},
		{"1.2.3-01", false}, {"1.2.3-beta.01", false},
		{" 1.2.3", false}, {"1.2.3 ", false}, {"1.2.3\n", false},
		{"1.2.3-", false}, {"1.2.3+", false}, {"1.2.3-a..b", false},
		{"1.2.3+a..b", false}, {"1.2.3+a_b", false}, {"1.2.3-测试", false},
		{"1.2.3+" + strings.Repeat("a", 123), false},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			pkg := testPackage("https://firmware.example/fw.tar.zlib", testSHA256, 42)
			pkg.Version = new(tc.version)
			got, err := normalizePackage(pkg)
			if (err == nil) != tc.valid {
				t.Fatalf("normalizePackage(%q): %v; want valid=%v", tc.version, err, tc.valid)
			}
			if tc.valid && (got.Version == nil || *got.Version != tc.version) {
				t.Fatalf("version changed to %v", got.Version)
			}
			value := map[string]any{"url": pkg.Url, "sha256": pkg.Sha256, "size": float64(pkg.Size), "version": tc.version}
			if err := schema.VisitJSON(value); (err == nil) != tc.valid {
				t.Fatalf("schema(%q): %v; want valid=%v", tc.version, err, tc.valid)
			}
		})
	}
	if err := schema.VisitJSON(map[string]any{"url": "https://firmware.example/fw.tar.zlib", "sha256": testSHA256, "size": float64(42)}); err != nil {
		t.Fatalf("response schema rejected an unversioned stored package: %v", err)
	}
}

func TestServerVersionWritesAndRollback(t *testing.T) {
	server := &Server{DB: newTestDatabase(t)}
	input := firmwareUpsert("versioned", firmwareSlot("release", "https://firmware.example/fw.tar.zlib", 42), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
	input.Slots.Stable.Package.Version = new("2.0.0")
	createFirmware(t, server, input)
	for _, version := range []string{"", "v1.2.3", "1.2.3-01", strings.Repeat("1", 129)} {
		input.Slots.Stable.Package.Version = new(version)
		response, err := server.PutFirmware(t.Context(), adminhttp.PutFirmwareRequestObject{Id: input.Id, Body: &input})
		if _, ok := response.(adminhttp.PutFirmware400JSONResponse); err != nil || !ok {
			t.Fatalf("invalid put %q: %T, %v", version, response, err)
		}
		responseCreate, err := server.CreateFirmware(t.Context(), adminhttp.CreateFirmwareRequestObject{Body: &input})
		if _, ok := responseCreate.(adminhttp.CreateFirmware400JSONResponse); err != nil || !ok {
			t.Fatalf("invalid create %q: %T, %v", version, responseCreate, err)
		}
		stored, err := server.GetFirmware(t.Context(), adminhttp.GetFirmwareRequestObject{Id: input.Id})
		if err != nil {
			t.Fatal(err)
		}
		if got := stored.(adminhttp.GetFirmware200JSONResponse).Slots.Stable.Package.Version; got == nil || *got != "2.0.0" {
			t.Fatalf("rejected write changed version to %v", got)
		}
	}
	input.Slots.Stable.Package.Version = new("1.5.0-beta.1+abc123")
	response, err := server.PutFirmware(t.Context(), adminhttp.PutFirmwareRequestObject{Id: input.Id, Body: &input})
	updated, ok := response.(adminhttp.PutFirmware200JSONResponse)
	if err != nil || !ok || updated.Slots.Stable.Package.Version == nil || *updated.Slots.Stable.Package.Version != *input.Slots.Stable.Package.Version {
		t.Fatalf("rollback: %T, %v", response, err)
	}
	stored, err := server.GetFirmware(t.Context(), adminhttp.GetFirmwareRequestObject{Id: input.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.(adminhttp.GetFirmware200JSONResponse).Slots.Stable.Package.Version; got == nil || *got != *input.Slots.Stable.Package.Version {
		t.Fatalf("stored rollback version = %v", got)
	}
}

func TestStoredUnversionedPackageRemainsReadable(t *testing.T) {
	server := &Server{DB: newTestDatabase(t)}
	input := firmwareUpsert("legacy", firmwareSlot("release", "https://firmware.example/fw.tar.zlib", 42), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
	createFirmware(t, server, input)
	legacy := `{"stable":{"package":{"url":"https://firmware.example/fw.tar.zlib","sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","size":42}},"beta":{},"develop":{}}`
	seedLegacy := func() {
		t.Helper()
		if _, err := server.DB.ExecContext(t.Context(), `UPDATE firmwares SET slots_json=? WHERE id=?`, legacy, input.Id); err != nil {
			t.Fatal(err)
		}
	}
	seedLegacy()
	got, err := server.GetFirmware(t.Context(), adminhttp.GetFirmwareRequestObject{Id: input.Id})
	valid, ok := got.(adminhttp.GetFirmware200JSONResponse)
	if err != nil || !ok || valid.Slots.Stable.Package == nil || valid.Slots.Stable.Package.Version != nil {
		t.Fatalf("legacy get = %#v, %v", got, err)
	}
	data, err := json.Marshal(valid)
	if err != nil || strings.Contains(string(data), `"version"`) || !strings.Contains(string(data), `"url":"https://firmware.example/fw.tar.zlib"`) {
		t.Fatalf("legacy JSON = %s, %v", data, err)
	}
	listed, err := server.ListFirmwares(t.Context(), adminhttp.ListFirmwaresRequestObject{})
	if _, ok := listed.(adminhttp.ListFirmwares200JSONResponse); err != nil || !ok {
		t.Fatalf("legacy list = %T, %v", listed, err)
	}
	missing := input
	missing.Slots.Stable.Package = new(*input.Slots.Stable.Package)
	missing.Slots.Stable.Package.Version = nil
	rejected, err := server.PutFirmware(t.Context(), adminhttp.PutFirmwareRequestObject{Id: input.Id, Body: &missing})
	if _, ok := rejected.(adminhttp.PutFirmware400JSONResponse); err != nil || !ok {
		t.Fatalf("unversioned put = %T, %v", rejected, err)
	}
	missing.Id = "new-unversioned"
	created, err := server.CreateFirmware(t.Context(), adminhttp.CreateFirmwareRequestObject{Body: &missing})
	if _, ok := created.(adminhttp.CreateFirmware400JSONResponse); err != nil || !ok {
		t.Fatalf("unversioned create = %T, %v", created, err)
	}
	var stored string
	if err := server.DB.GetContext(t.Context(), &stored, `SELECT slots_json FROM firmwares WHERE id=?`, input.Id); err != nil || stored != legacy {
		t.Fatalf("reads/rejected writes mutated stored package: %v", err)
	}
	updated, err := server.PutFirmware(t.Context(), adminhttp.PutFirmwareRequestObject{Id: input.Id, Body: &input})
	versioned, ok := updated.(adminhttp.PutFirmware200JSONResponse)
	if err != nil || !ok || versioned.Slots.Stable.Package.Version == nil || *versioned.Slots.Stable.Package.Version != "1.2.3" {
		t.Fatalf("versioned put = %#v, %v", updated, err)
	}
	seedLegacy()
	deleted, err := server.DeleteFirmware(t.Context(), adminhttp.DeleteFirmwareRequestObject{Id: input.Id})
	if _, ok := deleted.(adminhttp.DeleteFirmware200JSONResponse); err != nil || !ok {
		t.Fatalf("legacy delete = %T, %v", deleted, err)
	}
}

func TestStoredInvalidVersionRejectsReadAndPreservesDelete(t *testing.T) {
	for _, version := range []string{"", "v1.2.3"} {
		t.Run(version, func(t *testing.T) {
			server := &Server{DB: newTestDatabase(t)}
			input := firmwareUpsert("invalid", firmwareSlot("release", "https://firmware.example/fw.tar.zlib", 42), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
			createFirmware(t, server, input)
			input.Slots.Stable.Package.Version = new(version)
			invalid, err := json.Marshal(input.Slots)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := server.DB.ExecContext(t.Context(), `UPDATE firmwares SET slots_json=? WHERE id=?`, string(invalid), input.Id); err != nil {
				t.Fatal(err)
			}
			got, err := server.GetFirmware(t.Context(), adminhttp.GetFirmwareRequestObject{Id: input.Id})
			if _, ok := got.(adminhttp.GetFirmware500JSONResponse); err != nil || !ok {
				t.Fatalf("invalid get = %T, %v", got, err)
			}
			deleted, err := server.DeleteFirmware(t.Context(), adminhttp.DeleteFirmwareRequestObject{Id: input.Id})
			if _, ok := deleted.(adminhttp.DeleteFirmware500JSONResponse); err != nil || !ok {
				t.Fatalf("invalid delete = %T, %v", deleted, err)
			}
			var stored string
			if err := server.DB.GetContext(t.Context(), &stored, `SELECT slots_json FROM firmwares WHERE id=?`, input.Id); err != nil || stored != string(invalid) {
				t.Fatalf("invalid record mutated: %v", err)
			}
		})
	}
}
