package runtimeprofile

import (
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestMhsProfileValidationRevisionAndSQL(t *testing.T) {
	manifest := apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "led.status", Kind: "led", States: []apitypes.MhsV0State{{Name: "brightness", Type: "int", Access: "read_write"}}}}}
	input := adminhttp.RuntimeProfileUpsert{Id: "mhs", Spec: apitypes.RuntimeProfileSpec{Mhs: &apitypes.RuntimeProfileMhs{V0: &manifest}}}
	item, err := normalizeProfile(input, "")
	if err != nil {
		t.Fatal(err)
	}
	firstRevision := item.Revision
	manifest.Devices[0].States[0].Max = new(100.0)
	item, err = normalizeProfile(input, "")
	if err != nil || item.Revision == firstRevision {
		t.Fatalf("revision unchanged: %v", err)
	}
	db := profileSQLTestDB(t)
	item.CreatedAt = time.Now().UTC()
	item.UpdatedAt = item.CreatedAt
	if _, err := insertRuntimeProfileSQL(t.Context(), db, item); err != nil {
		t.Fatal(err)
	}
	got, version, err := getRuntimeProfileSQL(t.Context(), db, item.Id)
	if err != nil || !reflect.DeepEqual(got.Spec.Mhs, item.Spec.Mhs) {
		t.Fatalf("read %v %v", got.Spec.Mhs, err)
	}
	listed, _, _, err := listRuntimeProfileSQL(t.Context(), db, "", 10)
	if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0].Spec.Mhs, item.Spec.Mhs) {
		t.Fatalf("list %v %v", listed, err)
	}
	item.Spec.Mhs = nil
	updated, _, err := updateRuntimeProfileSQL(t.Context(), db, item, version)
	if err != nil || updated.Spec.Mhs != nil {
		t.Fatalf("clear %v %v", updated.Spec.Mhs, err)
	}
	manifest.Devices = append(manifest.Devices, manifest.Devices[0])
	if _, err := normalizeProfile(input, ""); err == nil {
		t.Fatal("duplicate manifest accepted")
	}
}
