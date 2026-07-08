//go:build gizclaw_e2e

package admin_test

import (
	"archive/tar"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIFirmwaresListGetPaginationAndUpload(t *testing.T) {
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
	created, err := env.api.CreateFirmwareWithResponse(env.ctx, adminhttp.FirmwareUpsert{
		Name:        name,
		Description: ptr("Admin API mutation firmware"),
		Slots:       firmwareSlots("Admin API stable firmware"),
	})
	if err != nil {
		t.Fatalf("create firmware: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	t.Cleanup(func() { _, _ = env.api.DeleteFirmwareWithResponse(env.ctx, name) })

	payload := adminFirmwareTarPayload(t, map[string]string{
		"MANIFEST.txt":            "admin api firmware bundle",
		"firmware/main.bin":       "admin api main firmware payload",
		"firmware/voice_dsp.bin":  "admin api voice dsp firmware payload",
		"assets/icons/status.png": "\x89PNG\r\n\x1a\nadmin icon",
		"config/device.json":      `{"modules":["main","voice_dsp"]}`,
		"docs/release-notes.txt":  "admin api artifact release notes",
	})
	upload, err := env.api.UploadFirmwareArtifactWithBodyWithResponse(env.ctx, name, adminhttp.UploadFirmwareArtifactParamsChannelStable, "application/x-tar", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("upload firmware artifact: %v", err)
	}
	requireStatusOK(t, upload, upload.Body)
	if upload.JSON200 == nil || upload.JSON200.Slots.Stable.Artifact == nil {
		t.Fatalf("upload firmware artifact = %#v", upload.JSON200)
	}
	list, err := env.api.ListFirmwareArtifactEntriesWithResponse(env.ctx, name, adminhttp.ListFirmwareArtifactEntriesParamsChannelStable, nil)
	if err != nil {
		t.Fatalf("list firmware artifact entries: %v", err)
	}
	requireStatusOK(t, list, list.Body)
	if list.JSON200 == nil || !artifactEntriesContain(list.JSON200.Items, "firmware", "assets", "MANIFEST.txt") {
		t.Fatalf("artifact list = %#v", list.JSON200)
	}
	firmwarePath := "firmware"
	listFirmware, err := env.api.ListFirmwareArtifactEntriesWithResponse(env.ctx, name, adminhttp.ListFirmwareArtifactEntriesParamsChannelStable, &adminhttp.ListFirmwareArtifactEntriesParams{Path: &firmwarePath})
	if err != nil {
		t.Fatalf("list firmware artifact firmware dir: %v", err)
	}
	requireStatusOK(t, listFirmware, listFirmware.Body)
	if listFirmware.JSON200 == nil || !artifactEntriesContain(listFirmware.JSON200.Items, "firmware/main.bin", "firmware/voice_dsp.bin") {
		t.Fatalf("artifact firmware list = %#v", listFirmware.JSON200)
	}
	tree, err := env.api.TreeFirmwareArtifactEntriesWithResponse(env.ctx, name, adminhttp.TreeFirmwareArtifactEntriesParamsChannel("stable"), nil)
	if err != nil {
		t.Fatalf("tree firmware artifact entries: %v", err)
	}
	requireStatusOK(t, tree, tree.Body)
	if tree.JSON200 == nil || !artifactEntriesContain(tree.JSON200.Items, "assets/icons/status.png", "config/device.json", "docs/release-notes.txt", "firmware/main.bin") {
		t.Fatalf("artifact tree = %#v", tree.JSON200)
	}
	statPath := "assets/icons/status.png"
	stat, err := env.api.StatFirmwareArtifactEntryWithResponse(env.ctx, name, adminhttp.StatFirmwareArtifactEntryParamsChannelStable, &adminhttp.StatFirmwareArtifactEntryParams{Path: &statPath})
	if err != nil {
		t.Fatalf("stat firmware artifact entry: %v", err)
	}
	requireStatusOK(t, stat, stat.Body)
	if stat.JSON200 == nil || stat.JSON200.Entry == nil || stat.JSON200.Entry.Path != statPath || stat.JSON200.Entry.Size <= 0 || !strings.Contains(ptrValue(stat.JSON200.Entry.ContentType), "image/png") {
		t.Fatalf("artifact stat = %#v", stat.JSON200)
	}
	downloadEntry, err := env.api.DownloadFirmwareArtifactEntryWithResponse(env.ctx, name, adminhttp.DownloadFirmwareArtifactEntryParamsChannelStable, &adminhttp.DownloadFirmwareArtifactEntryParams{Path: "firmware/main.bin"})
	if err != nil {
		t.Fatalf("download firmware artifact entry: %v", err)
	}
	requireStatusOK(t, downloadEntry, downloadEntry.Body)
	if !bytes.Contains(downloadEntry.Body, []byte("admin api main firmware payload")) {
		t.Fatalf("artifact entry payload = %q", string(downloadEntry.Body))
	}
	downloadTar, err := env.api.DownloadFirmwareArtifactWithResponse(env.ctx, name, adminhttp.DownloadFirmwareArtifactParamsChannelStable)
	if err != nil {
		t.Fatalf("download firmware artifact tar: %v", err)
	}
	requireStatusOK(t, downloadTar, downloadTar.Body)
	requireTarEntries(t, downloadTar.Body, "firmware/main.bin", "assets/icons/status.png", "config/device.json")

	deletedArtifact, err := env.api.DeleteFirmwareArtifactWithResponse(env.ctx, name, adminhttp.DeleteFirmwareArtifactParamsChannelStable)
	if err != nil {
		t.Fatalf("delete firmware artifact: %v", err)
	}
	requireStatusOK(t, deletedArtifact, deletedArtifact.Body)
	if deletedArtifact.JSON200 == nil || deletedArtifact.JSON200.Slots.Stable.Artifact != nil {
		t.Fatalf("delete firmware artifact = %#v", deletedArtifact.JSON200)
	}
}

func artifactEntriesContain(items []apitypes.FirmwareArtifactEntry, paths ...string) bool {
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[item.Path] = true
	}
	for _, path := range paths {
		if !seen[path] {
			return false
		}
	}
	return true
}

func requireTarEntries(t *testing.T, payload []byte, paths ...string) {
	t.Helper()
	seen := make(map[string]bool, len(paths))
	tr := tar.NewReader(bytes.NewReader(payload))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read artifact tar: %v", err)
		}
		seen[header.Name] = true
	}
	for _, path := range paths {
		if !seen[path] {
			t.Fatalf("artifact tar missing %q; seen=%v", path, seen)
		}
	}
}

func ptrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
