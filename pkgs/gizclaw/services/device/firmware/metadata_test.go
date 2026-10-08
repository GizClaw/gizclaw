package firmware

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestFirmwareMetadataLifecycle(t *testing.T) {
	server := &Server{DB: newTestDatabase(t)}
	metadata := apitypes.FirmwareMetadata{
		"modem":   json.RawMessage(`{"version":"vendor-2026.10","urls":["https://firmware.example/a.bin","https://firmware.example/b.bin"]}`),
		"string":  json.RawMessage(`"hello"`),
		"number":  json.RawMessage(`9007199254740993`),
		"boolean": json.RawMessage(`true`),
		"array":   json.RawMessage(`[1,"x",null]`),
		"null":    json.RawMessage(`null`),
	}
	input := firmwareUpsert("metadata-device", firmwareSlot("stable", "https://firmware.example/stable.tar.zlib", 42), apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
	input.Metadata = &metadata
	created := createFirmware(t, server, input)
	if !reflect.DeepEqual(created.Metadata, &metadata) {
		t.Fatalf("created metadata = %#v", created.Metadata)
	}
	metadata["modem"][0] = 'x'
	stored, err := Get(t.Context(), server.DB, input.Id)
	if err != nil {
		t.Fatal(err)
	}
	if string((*stored.Metadata)["number"]) != "9007199254740993" || (*stored.Metadata)["modem"][0] != '{' {
		t.Fatalf("stored metadata lost precision or shared caller buffers: %#v", stored.Metadata)
	}
	list, err := server.ListFirmwares(t.Context(), adminhttp.ListFirmwaresRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(list.(adminhttp.ListFirmwares200JSONResponse).Items[0].Metadata, stored.Metadata) {
		t.Fatal("list lost metadata")
	}
	input.Metadata = nil
	response, err := server.PutFirmware(t.Context(), adminhttp.PutFirmwareRequestObject{Id: input.Id, Body: &input})
	if err != nil {
		t.Fatal(err)
	}
	updated := apitypes.Firmware(response.(adminhttp.PutFirmware200JSONResponse))
	if updated.Metadata != nil || !reflect.DeepEqual(updated.Slots, created.Slots) {
		t.Fatalf("metadata removal changed channels: %#v", updated)
	}
}

func TestFirmwareMetadataRejectsInvalidReplacement(t *testing.T) {
	server := &Server{DB: newTestDatabase(t)}
	metadata := apitypes.FirmwareMetadata{"modem": json.RawMessage(`"1.2.3"`)}
	input := firmwareUpsert("metadata-device", apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{}, apitypes.FirmwareSlot{})
	input.Metadata = &metadata
	createFirmware(t, server, input)
	for _, test := range []struct {
		name, key string
		value     json.RawMessage
	}{
		{"bad key", "../modem", json.RawMessage(`null`)},
		{"malformed JSON", "modem", json.RawMessage(`{"missing":`)},
		{"multiple values", "modem", json.RawMessage(`{} []`)},
		{"invalid UTF-8", "modem", json.RawMessage{'"', 0xff, '"'}},
		{"oversize", "modem", json.RawMessage(`"` + strings.Repeat("x", 65535) + `"`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := apitypes.FirmwareMetadata{test.key: test.value}
			update := input
			update.Metadata = &bad
			response, err := server.PutFirmware(t.Context(), adminhttp.PutFirmwareRequestObject{Id: input.Id, Body: &update})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := response.(adminhttp.PutFirmware400JSONResponse); !ok {
				t.Fatalf("replacement response = %T", response)
			}
			stored, err := Get(t.Context(), server.DB, input.Id)
			if err != nil || !reflect.DeepEqual(stored.Metadata, &metadata) {
				t.Fatalf("invalid replacement changed metadata: %#v, %v", stored.Metadata, err)
			}
		})
	}
}

func TestFirmwareMetadataCompactsAndPreservesNull(t *testing.T) {
	metadata := apitypes.FirmwareMetadata{"null": nil, "object": json.RawMessage(` { "n": 9007199254740993, "html": "<" } `)}
	got, err := normalizeMetadata(&metadata)
	if err != nil {
		t.Fatal(err)
	}
	if string((*got)["null"]) != "null" || !json.Valid((*got)["object"]) || !strings.Contains(string((*got)["object"]), "9007199254740993") {
		t.Fatalf("normalized metadata = %#v", got)
	}
	again, err := normalizeMetadata(got)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatalf("normalization changed on re-read: %v", err)
	}
}

func TestFirmwareMetadataInitializePreservesOldCatalog(t *testing.T) {
	db := newTestDatabase(t)
	if _, err := db.ExecContext(t.Context(), "DROP TABLE firmwares"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE firmwares (id TEXT PRIMARY KEY, description TEXT, slots_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	slots, err := json.Marshal(apitypes.FirmwareSlots{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO firmwares(id, slots_json, created_at, updated_at) VALUES (?, ?, ?, ?)`, "old-device", string(slots), "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	server := &Server{DB: db}
	for range 2 {
		if err := server.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := Get(t.Context(), db, "old-device")
	if err != nil || stored.Id != "old-device" || stored.Metadata != nil {
		t.Fatalf("old catalog = %#v, %v", stored, err)
	}
}
