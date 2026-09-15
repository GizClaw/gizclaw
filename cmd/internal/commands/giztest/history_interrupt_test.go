package giztestcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// The local provider drives real interruption terminals; Host installs the
// production History wrapper, backed by SQLite and filesystem replay assets.
func TestDeviceTextInterruptedHistory(t *testing.T) {
	for _, workflow := range []string{"doubao-ptt", "doubao-realtime"} {
		for _, committed := range []bool{false, true} {
			name := "before-transcript"
			if committed {
				name = "after-transcript"
			}
			t.Run(workflow+"/"+name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				tr := textRegressionTransformer(t, ctx, workflow)
				agent := openHistoryRegressionAgent(t, ctx, tr)
				input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
				defer input.Close()
				output, err := agent.Transform(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				push := func(id, text string) {
					t.Helper()
					for _, chunk := range textInputChunks(&giztest.PeerStreamOperation{TextDone: true}, id, text) {
						if err := input.Push(ctx, chunk); err != nil {
							t.Fatal(err)
						}
					}
				}
				firstText := "第一段：partial reply"
				if !committed {
					firstText = "audio-only:" + firstText
				}
				push("first", firstText)
				firstID, secondID := "", ""
				var delivered strings.Builder
				firstTextEOS, firstAudioEOS := false, false
				secondTextEOS, secondAudioEOS := false, false
				sent := false
				for !secondTextEOS || !secondAudioEOS {
					chunk, err := output.Next()
					if err != nil {
						t.Fatal(err)
					}
					if chunk == nil || chunk.Ctrl == nil || chunk.Role != genx.RoleModel {
						continue
					}
					if firstID == "" {
						firstID = chunk.Ctrl.StreamID
					}
					if chunk.Ctrl.StreamID == firstID {
						if text, ok := chunk.Part.(genx.Text); ok && !chunk.Ctrl.TextInterim {
							delivered.WriteString(string(text))
						}
						if chunk.IsEndOfStream() && strings.HasPrefix(chunk.Ctrl.Error, "interrupted") {
							mime, _ := chunk.MIMEType()
							firstTextEOS = firstTextEOS || mime == "text/plain"
							firstAudioEOS = firstAudioEOS || strings.HasPrefix(mime, "audio/")
						}
						if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 && !sent {
							if chunk.IsEndOfStream() {
								t.Fatal("first reply completed before barge-in")
							}
							if committed && delivered.String() != firstText {
								t.Fatalf("partial transcript not delivered: %q", delivered.String())
							}
							if !committed && delivered.Len() != 0 {
								t.Fatalf("unexpected committed transcript: %q", delivered.String())
							}
							sent = true
							push("second", "replacement reply")
						}
					} else {
						if !sent || !firstTextEOS || !firstAudioEOS {
							t.Fatal("replacement preceded interruption")
						}
						if secondID == "" {
							secondID = chunk.Ctrl.StreamID
						}
						if chunk.Ctrl.StreamID != secondID {
							t.Fatal("unexpected third reply")
						}
						if chunk.IsEndOfStream() {
							if chunk.Ctrl.Error != "" {
								t.Fatalf("replacement error: %s", chunk.Ctrl.Error)
							}
							mime, _ := chunk.MIMEType()
							secondTextEOS = secondTextEOS || mime == "text/plain"
							secondAudioEOS = secondAudioEOS || strings.HasPrefix(mime, "audio/")
						}
					}
				}
				// Both replacement channels have ended. Poll for its durable entry before
				// asserting absence, so an empty list during startup cannot pass the test.
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					history, err := agent.ListHistory(ctx, apitypes.PeerRunHistoryListRequest{})
					if err != nil {
						t.Fatal(err)
					}
					if !history.Available || history.HasNext {
						t.Fatalf("incomplete History: %+v", history)
					}
					replacement, partial, agents := 0, 0, 0
					for _, item := range history.Items {
						if item.Type != apitypes.PeerRunHistoryEntryTypeAgent {
							continue
						}
						agents++
						if item.Text == "replacement reply" {
							replacement++
						}
						if item.Text == firstText {
							partial++
						}
					}
					if replacement > 0 {
						want := 1
						if committed {
							want++
						}
						if agents != want || replacement != 1 || (committed && partial != 1) {
							t.Fatalf("History agents=%d replacement=%d partial=%d, want agents=%d: %+v", agents, replacement, partial, want, history.Items)
						}
						break
					}
					select {
					case <-ticker.C:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
			})
		}
	}
}

type historyRegressionResolver struct{ spec agenthost.Spec }

func (r historyRegressionResolver) Resolve(context.Context, string) (agenthost.Spec, error) {
	return r.spec, nil
}

func openHistoryRegressionAgent(t *testing.T, ctx context.Context, tr genx.Transformer) agenthost.Agent {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	records, err := logstore.NewSQLStoreWithDB(db, "workspace_history")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	objects, err := objectstore.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	host := agenthost.New(historyRegressionResolver{spec: agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "history-regression", Name: "history-regression"},
		AgentType: "fixture", Runtime: workspace.Runtime{History: workspace.NewHistoryStore(records, objects, "history-regression")},
	}})
	if err := host.Register("fixture", agenthost.FactoryFunc(func(context.Context, agenthost.Spec) (genx.Transformer, error) { return tr, nil })); err != nil {
		t.Fatal(err)
	}
	agent, release, err := host.OpenAgent(ctx, "history-regression")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return agent
}
