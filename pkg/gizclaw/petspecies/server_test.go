package petspecies

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

func TestServerPutUploadDownloadAndList(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	srv := &Server{
		Store:  kv.NewMemory(nil),
		Assets: objectstore.Dir(t.TempDir()),
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
	}
	item, err := srv.Put(ctx, "rabbit", apitypes.PetSpeciesSpec{Name: "Rabbit"})
	if err != nil {
		t.Fatalf("Put error = %v", err)
	}
	if item.Id != "rabbit" || item.Name != "Rabbit" || item.PixaPath != "" {
		t.Fatalf("Put item = %#v", item)
	}
	data := testPixa(120, 96, []string{"default", "feed"})
	item, err = srv.UploadPixa(ctx, "rabbit", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("UploadPixa error = %v", err)
	}
	if item.PixaPath != "rabbit.pixa" || item.PixaMetadata.CanvasWidth != 120 || item.PixaMetadata.FrameCount != 1 || len(item.PixaMetadata.ClipNames) != 2 {
		t.Fatalf("UploadPixa item = %#v", item)
	}
	r, err := srv.DownloadPixa(ctx, "rabbit")
	if err != nil {
		t.Fatalf("DownloadPixa error = %v", err)
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll error = %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("download bytes = %q, want %q", got, data)
	}

	if _, err := srv.Put(ctx, "cat", apitypes.PetSpeciesSpec{Name: "Cat"}); err != nil {
		t.Fatalf("Put cat error = %v", err)
	}
	items, hasNext, next, err := srv.List(ctx, "", 1)
	if err != nil {
		t.Fatalf("List page 1 error = %v", err)
	}
	if len(items) != 1 || !hasNext || next == nil {
		t.Fatalf("List page 1 = items %#v hasNext %v next %v", items, hasNext, next)
	}
	items, hasNext, _, err = srv.List(ctx, *next, 1)
	if err != nil {
		t.Fatalf("List page 2 error = %v", err)
	}
	if len(items) != 1 || hasNext {
		t.Fatalf("List page 2 = items %#v hasNext %v", items, hasNext)
	}
}

func TestParsePixaMetadataRejectsInvalidFiles(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(""),
		[]byte("ZPET"),
		testPixaWithHeaderMutation(func(data []byte) {
			binary.LittleEndian.PutUint16(data[4:6], 2)
		}),
		testPixaWithHeaderMutation(func(data []byte) {
			binary.LittleEndian.PutUint32(data[36:40], 9999)
		}),
		testPixaWithHeaderMutation(func(data []byte) {
			binary.LittleEndian.PutUint32(data[40+2+36:40+2+40], 5)
		}),
		testPixaWithHeaderMutation(func(data []byte) {
			binary.LittleEndian.PutUint32(data[40+2+56+8:40+2+56+12], 1)
		}),
	} {
		if _, err := ParsePixaMetadata(data); err == nil {
			t.Fatalf("ParsePixaMetadata(%q) error = nil, want error", data)
		}
	}
}

func TestServerGetUpdateDeleteAndConfigurationErrors(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())}
	if _, err := srv.Put(ctx, " ", apitypes.PetSpeciesSpec{Name: "bad"}); err == nil {
		t.Fatalf("Put blank id error = nil, want error")
	}
	if _, err := srv.Put(ctx, "fox", apitypes.PetSpeciesSpec{}); err == nil {
		t.Fatalf("Put blank name error = nil, want error")
	}
	item, err := srv.Put(ctx, "fox", apitypes.PetSpeciesSpec{Name: "Fox", PixaPath: stringPtr("custom/fox.pixa")})
	if err != nil {
		t.Fatalf("Put fox error = %v", err)
	}
	data := testPixa(64, 32, []string{"idle"})
	item, err = srv.UploadPixa(ctx, "fox", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("UploadPixa fox error = %v", err)
	}
	if item.PixaPath != "custom/fox.pixa" || item.PixaMetadata.Version != 1 {
		t.Fatalf("UploadPixa fox item = %#v", item)
	}
	got, err := srv.Get(ctx, "fox")
	if err != nil {
		t.Fatalf("Get fox error = %v", err)
	}
	if got.Name != "Fox" || got.PixaPath != "custom/fox.pixa" {
		t.Fatalf("Get fox = %#v", got)
	}
	updated, err := srv.Put(ctx, "fox", apitypes.PetSpeciesSpec{Name: "Red Fox"})
	if err != nil {
		t.Fatalf("Put update fox error = %v", err)
	}
	if updated.Name != "Red Fox" || updated.PixaPath != "custom/fox.pixa" || len(updated.PixaMetadata.ClipNames) != 1 {
		t.Fatalf("Put update fox = %#v", updated)
	}
	deleted, err := srv.Delete(ctx, "fox")
	if err != nil {
		t.Fatalf("Delete fox error = %v", err)
	}
	if deleted.Id != "fox" {
		t.Fatalf("Delete fox = %#v", deleted)
	}
	if _, err := srv.Get(ctx, "fox"); err == nil {
		t.Fatalf("Get deleted fox error = nil, want error")
	}
	if _, err := srv.DownloadPixa(ctx, "fox"); err == nil {
		t.Fatalf("Download deleted fox error = nil, want error")
	}

	if _, _, _, err := (&Server{}).List(ctx, "", 0); err == nil {
		t.Fatalf("List without store error = nil, want error")
	}
	if _, err := (&Server{Store: kv.NewMemory(nil)}).UploadPixa(ctx, "fox", bytes.NewReader(data)); err == nil {
		t.Fatalf("UploadPixa without assets error = nil, want error")
	}
}

func stringPtr(value string) *string {
	return &value
}

func testPixa(width, height uint16, clips []string) []byte {
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
	binary.LittleEndian.PutUint32(data[36:40], 0)
	for i, name := range clips {
		base := clipOffset + i*clipEntrySize
		copy(data[base:base+32], name)
		binary.LittleEndian.PutUint32(data[base+36:base+40], 0)
		binary.LittleEndian.PutUint32(data[base+40:base+44], 1)
		binary.LittleEndian.PutUint32(data[base+44:base+48], 120)
		binary.LittleEndian.PutUint16(data[base+48:base+50], 1)
	}
	binary.LittleEndian.PutUint16(data[frameOffset:frameOffset+2], 120)
	return data
}

func testPixaWithHeaderMutation(mutate func([]byte)) []byte {
	data := testPixa(16, 16, []string{"idle"})
	mutate(data)
	return data
}
