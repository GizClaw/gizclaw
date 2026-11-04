package agenthost

import (
	"fmt"
	"io"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

func TestHistoryRecorderKeepsArrivalOrderWithEqualClockReadings(t *testing.T) {
	for _, async := range []bool{false, true} {
		t.Run(fmt.Sprintf("async=%t", async), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				history := newTestWorkspaceHistory(t, newTestObjectStore(t))
				recorder := newHistoryRecorder(history, "native-peer", nil)
				ctx := t.Context()
				if async {
					recorder.startPersistence(ctx)
					defer recorder.persistence.close(ctx)
					ctx = recorder.outputPersistenceContext(ctx)
				}
				observe := func(chunk *genx.MessageChunk) {
					t.Helper()
					if err := recorder.ObserveOutput(ctx, chunk); err != nil {
						t.Fatal(err)
					}
				}
				packet := []byte{0xf8, 0xff, 0xfe}
				at := time.Now()
				observe(&genx.MessageChunk{
					Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
					Ctrl: &genx.StreamCtrl{StreamID: "speech", Label: genx.HistoryUserAudioLabel, BeginOfStream: true, EndOfStream: true},
				})
				names := []string{"assistant", "reply-part-1", "reply-part-2"}
				for _, name := range names {
					observe(&genx.MessageChunk{
						Role: genx.RoleModel, Name: name, Part: genx.Text(name),
						Ctrl: &genx.StreamCtrl{StreamID: name, Label: "assistant", BeginOfStream: true},
					})
				}
				// No fake time has elapsed: distinct arrivals must keep their order
				// even when the platform clock cannot distinguish them.
				if !time.Now().Equal(at) {
					t.Fatal("clock advanced while observing entries")
				}
				// Eino ends secondary outputs before its primary reply. The native
				// input's independent transcription may finish after every reply.
				for _, name := range slices.Backward(names) {
					observe(&genx.MessageChunk{
						Role: genx.RoleModel, Name: name, Part: genx.Text(""),
						Ctrl: &genx.StreamCtrl{StreamID: name, Label: "assistant", EndOfStream: true, ResponseEpochEnd: true},
					})
				}
				observe(&genx.MessageChunk{
					Role: genx.RoleUser, Name: "transcript", Part: genx.Text("spoken question"),
					Ctrl: &genx.StreamCtrl{StreamID: "speech", Label: "transcript", BeginOfStream: true, EndOfStream: true},
				})
				if err := recorder.waitForText(ctx); err != nil {
					t.Fatal(err)
				}
				page, err := history.ListEntries(ctx, "", "", 10)
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Entries) != 1+len(names) {
					t.Fatalf("History has %d entries, want %d: %+v", len(page.Entries), 1+len(names), page.Entries)
				}
				user := page.Entries[0]
				if user.Type != historyEntryTypeGear || user.Name != "transcript" || user.Text != "spoken question" || user.TurnID != "speech" || user.GearID != "native-peer" || len(user.Assets) != 1 || !user.ReplayAvailable || user.Origin != workspace.HistoryOriginAgentHost {
					t.Fatalf("input did not precede replies with its text, audio and metadata: %+v", page.Entries)
				}
				for index, reply := range page.Entries[1:] {
					if reply.Type != historyEntryTypeAgent || reply.Name != names[index] || reply.Text != names[index] || reply.TurnID != names[index] || reply.GearID != "" || len(reply.Assets) != 0 || !reply.ReplayAvailable || reply.Origin != workspace.HistoryOriginAgentHost {
						t.Errorf("reply %d lost arrival order or metadata: %+v", index, reply)
					}
				}
				reader, err := history.ReadAsset(ctx, user.Assets[0].Name)
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				audio, err := io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				packets, err := historyOpusFramesFromOgg(audio)
				if err != nil || len(packets) != 1 || !slices.Equal(packets[0], packet) {
					t.Fatalf("input replay changed: packets=%v error=%v", packets, err)
				}
			})
		})
	}
}
