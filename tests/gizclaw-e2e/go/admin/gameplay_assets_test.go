//go:build gizclaw_e2e

package admin_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIBadgeIconUploadDownload(t *testing.T) {
	env := newAdminAPIHarness(t)

	id := mutationName("badge")
	_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindBadge, id)
	var resource apitypes.Resource
	if err := resource.FromBadgeResource(apitypes.BadgeResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.BadgeResourceKindBadge,
		Metadata:   apitypes.ResourceMetadata{Name: id},
		Spec:       apitypes.BadgeSpec{Name: "Admin API Badge", Description: "Admin API badge asset fixture"},
	}); err != nil {
		t.Fatalf("build badge resource: %v", err)
	}
	apply, err := env.api.ApplyResourceWithResponse(env.ctx, resource)
	if err != nil {
		t.Fatalf("apply badge resource: %v", err)
	}
	requireStatusOK(t, apply, apply.Body)
	t.Cleanup(func() { _, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindBadge, id) })

	icon := []byte("admin-api-badge-icon")
	upload, err := env.api.UploadBadgeIconWithBodyWithResponse(env.ctx, id, "image/png", bytes.NewReader(icon))
	if err != nil {
		t.Fatalf("upload badge icon: %v", err)
	}
	requireStatusOK(t, upload, upload.Body)
	if upload.JSON200 == nil || upload.JSON200.IconPath != id+"/icon" {
		t.Fatalf("upload badge icon = %#v", upload.JSON200)
	}

	download, err := env.api.DownloadBadgeIconWithResponse(env.ctx, id)
	if err != nil {
		t.Fatalf("download badge icon: %v", err)
	}
	requireStatusOK(t, download, download.Body)
	if !bytes.Equal(download.Body, icon) {
		t.Fatalf("download badge icon = %q, want %q", string(download.Body), string(icon))
	}
}

func TestAdminAPIPetSpeciesPixaUploadDownload(t *testing.T) {
	env := newAdminAPIHarness(t)

	id := mutationName("pet-species")
	_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindPetSpecies, id)
	var resource apitypes.Resource
	if err := resource.FromPetSpeciesResource(apitypes.PetSpeciesResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.PetSpeciesResourceKindPetSpecies,
		Metadata:   apitypes.ResourceMetadata{Name: id},
		Spec:       apitypes.PetSpeciesSpec{Name: "Admin API Pet"},
	}); err != nil {
		t.Fatalf("build pet species resource: %v", err)
	}
	apply, err := env.api.ApplyResourceWithResponse(env.ctx, resource)
	if err != nil {
		t.Fatalf("apply pet species resource: %v", err)
	}
	requireStatusOK(t, apply, apply.Body)
	t.Cleanup(func() { _, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindPetSpecies, id) })

	pixa := testAdminPixa(120, 120, []string{"idle"})
	upload, err := env.api.UploadPetSpeciesPixaWithBodyWithResponse(env.ctx, id, "application/octet-stream", bytes.NewReader(pixa))
	if err != nil {
		t.Fatalf("upload pet species pixa: %v", err)
	}
	requireStatusOK(t, upload, upload.Body)
	if upload.JSON200 == nil || upload.JSON200.PixaPath != id+".pixa" || upload.JSON200.PixaMetadata.FrameCount != 1 {
		t.Fatalf("upload pet species pixa = %#v", upload.JSON200)
	}

	download, err := env.api.DownloadPetSpeciesPixaWithResponse(env.ctx, id)
	if err != nil {
		t.Fatalf("download pet species pixa: %v", err)
	}
	requireStatusOK(t, download, download.Body)
	if !bytes.Equal(download.Body, pixa) {
		t.Fatalf("download pet species pixa bytes = %d, want %d", len(download.Body), len(pixa))
	}
}

func testAdminPixa(width, height uint16, clips []string) []byte {
	const (
		headerSize     = 40
		clipEntrySize  = 56
		frameEntrySize = 16
	)
	paletteOffset := headerSize
	clipOffset := paletteOffset + 2
	frameOffset := clipOffset + len(clips)*clipEntrySize
	payloadOffset := frameOffset + frameEntrySize
	data := make([]byte, payloadOffset)
	copy(data[0:4], "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], width)
	binary.LittleEndian.PutUint16(data[10:12], height)
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], uint16(len(clips)))
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	for i, name := range clips {
		base := clipOffset + i*clipEntrySize
		copy(data[base:base+32], name)
		binary.LittleEndian.PutUint32(data[base+40:base+44], 1)
	}
	return data
}
