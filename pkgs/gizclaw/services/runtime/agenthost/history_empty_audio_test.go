package agenthost

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func historyTestOggHeaders(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	writer, err := ogg.NewStreamWriter(&data, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, packet := range [][]byte{codecconv.OpusHeadPacket(16000, 1), codecconv.OpusTagsPacket("test")} {
		if _, err := writer.WritePacket(packet, 0, false); err != nil {
			t.Fatal(err)
		}
	}
	return data.Bytes()
}

func TestHistoryInterruptedHeaderOnlyAudioPreservesText(t *testing.T) {
	for _, withPackets := range []bool{false, true} {
		t.Run(map[bool]string{false: "headers_only", true: "mixed_audio"}[withPackets], func(t *testing.T) {
			history := newTestWorkspaceHistory(t, newTestObjectStore(t))
			recorder := newHistoryRecorder(history, "", nil)
			observe := func(part genx.Part, end bool) error {
				ctrl := &genx.StreamCtrl{StreamID: "interrupted", Label: "assistant", MessageID: "section", EndOfStream: end, ResponseEpochEnd: end}
				if end {
					ctrl.Error = "interrupted"
				}
				return recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Name: "host", Part: part, Ctrl: ctrl})
			}
			for _, part := range []genx.Part{genx.Text("delivered text"), &genx.Blob{MIMEType: "audio/ogg", Data: historyTestOggHeaders(t)}} {
				if err := observe(part, false); err != nil {
					t.Fatal(err)
				}
			}
			if withPackets {
				if err := observe(&genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}, false); err != nil {
					t.Fatal(err)
				}
			}
			if err := observe(&genx.Blob{MIMEType: "audio/ogg"}, true); err != nil {
				t.Fatalf("interrupted TTS headers lost delivered text: %v", err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if err := recorder.waitForText(ctx); err != nil {
				t.Fatalf("empty interrupted audio poisoned the next History read: %v", err)
			}
			page, err := history.ListPage(t.Context(), apitypes.PeerRunHistoryListRequest{})
			if err != nil || len(page.Entries) != 1 || page.Entries[0].Text != "delivered text" || !page.Entries[0].Interrupted {
				t.Fatalf("interrupted text History changed: %+v, %v", page, err)
			}
			wantAssets := 0
			if withPackets {
				wantAssets = 1
			}
			if len(page.Entries[0].Assets) != wantAssets {
				t.Fatalf("audio assets = %d, want %d", len(page.Entries[0].Assets), wantAssets)
			}
			if withPackets {
				asset, err := history.ReadAsset(t.Context(), page.Entries[0].Assets[0].Name)
				if err != nil {
					t.Fatal(err)
				}
				defer asset.Close()
				data, err := io.ReadAll(asset)
				if err != nil {
					t.Fatal(err)
				}
				packets, err := historyOpusFramesFromOgg(data)
				if err != nil || len(packets) != 1 || !bytes.Equal(packets[0], []byte{0xf8, 0xff, 0xfe}) {
					t.Fatalf("real audio was changed by an empty segment: %x, %v", packets, err)
				}
			}
		})
	}
}

func TestHistoryEmptyAudioRemainsInvalidOutsideInterruption(t *testing.T) {
	headers := historyTestOggHeaders(t)
	if _, err := historyAudioReplayChunks(genx.RoleModel, "host", "stream", "assistant", "audio/ogg", headers); err == nil {
		t.Fatal("replay accepted an asset without audio packets")
	}
	for _, tc := range []struct {
		name, interruption, wantError string
		data                          []byte
	}{
		{"normal_empty_audio", "", "no audio packets", headers},
		{"corrupt_interrupted_audio", "interrupted", "read history ogg opus", []byte("not ogg")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := newHistoryRecorder(newTestWorkspaceHistory(t, newTestObjectStore(t)), "", nil)
			for _, part := range []genx.Part{genx.Text("delivered text"), &genx.Blob{MIMEType: "audio/ogg", Data: tc.data}} {
				if err := recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Name: "host", Part: part, Ctrl: &genx.StreamCtrl{StreamID: "stream", Label: "assistant"}}); err != nil {
					t.Fatal(err)
				}
			}
			err := recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Name: "host", Part: &genx.Blob{MIMEType: "audio/ogg"}, Ctrl: &genx.StreamCtrl{StreamID: "stream", Label: "assistant", EndOfStream: true, ResponseEpochEnd: true, Error: tc.interruption}})
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("invalid audio error = %v, want %q", err, tc.wantError)
			}
		})
	}
}
