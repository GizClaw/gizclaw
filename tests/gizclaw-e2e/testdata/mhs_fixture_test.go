package testdata_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/goccy/go-yaml"
)

func TestMhsProfilesCoverManagementScenarios(t *testing.T) {
	var manifest map[string]string
	for _, path := range []string{"resources/09-giztest/01-runtime-profile.yaml", "resources/mhs/runtime-profile.yaml"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := yaml.YAMLToJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		var profile struct {
			Metadata struct {
				ID string `json:"id"`
			} `json:"metadata"`
			Spec apitypes.RuntimeProfileSpec `json:"spec"`
		}
		if err := json.Unmarshal(data, &profile); err != nil {
			t.Fatal(err)
		}
		if profile.Metadata.ID != "e2e-giztest" || profile.Spec.Mhs == nil || profile.Spec.Mhs.V0 == nil {
			t.Fatalf("%s has no e2e-giztest MHS manifest", path)
		}
		devices := map[string]string{}
		for _, device := range profile.Spec.Mhs.V0.Devices {
			if _, duplicate := devices[device.Id]; duplicate {
				t.Fatalf("%s repeats instance %s", path, device.Id)
			}
			devices[device.Id] = string(device.Hwd)
		}
		if manifest != nil && !maps.Equal(manifest, devices) {
			t.Fatalf("%s differs from the shared e2e-giztest MHS manifest", path)
		}
		manifest = devices
	}
	for _, name := range []string{
		"server.device.mhs.management.instances.giztest.yaml",
		"server.device.mhs.management.invalid_response.giztest.yaml",
		"server.device.mhs.nfc.control.giztest.yaml",
	} {
		doc, err := giztest.LoadDocument(filepath.Join("..", "giztest", name), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range doc.Steps {
			if step.HTTP == nil {
				continue
			}
			body, ok := step.HTTP.Body.(map[string]any)
			if !ok {
				t.Fatalf("%s/%s has no HWD request body", name, step.ID)
			}
			id, _ := body["id"].(string)
			hwd, _ := body["hwd"].(string)
			if hwd == "" || manifest[id] != hwd {
				t.Errorf("%s/%s requests undeclared instance %s/%s", name, step.ID, id, hwd)
			}
		}
	}
}
