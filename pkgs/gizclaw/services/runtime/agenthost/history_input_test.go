package agenthost

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestHistoryAgentPersistsDeliveredMessageAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	records := newTestHistoryLogStore(t)
	history := workspace.NewHistoryStore(records, newTestObjectStore(t), "conversation")
	builder := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("delivered prefix"), Ctrl: &genx.StreamCtrl{StreamID: "reply", MessageID: "message", Label: "assistant", BeginOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	agent := wrapHistoryAgent(historyTestAgent{output: builder.Stream()}, history)
	output, err := agent.Transform(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := output.Next(); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = builder.Abort(context.Canceled)
	_ = output.Close()
	page, err := records.Query(t.Context(), memory.HistoryQuery{ConversationID: "conversation", Limit: 10})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Text != "delivered prefix" || !page.Entries[0].Interrupted {
		t.Fatalf("delivered message lost after cancellation: %+v err=%v", page.Entries, err)
	}
}

func TestHistoryInputRecordsTextWithoutTranscriptEcho(t *testing.T) {
	for _, test := range []struct {
		name   string
		chunks []*genx.MessageChunk
		echo   string
		want   string
	}{
		{
			name: "fragmented text and control EOS",
			chunks: []*genx.MessageChunk{
				{Role: genx.RoleUser, Part: genx.Text("old partial"), Ctrl: &genx.StreamCtrl{StreamID: "old", BeginOfStream: true}},
				{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "current", BeginOfStream: true}},
				{Role: genx.RoleUser, Part: genx.Text("current "), Ctrl: &genx.StreamCtrl{StreamID: "current"}},
				{Role: genx.RoleUser, Part: genx.Text("input"), Ctrl: &genx.StreamCtrl{StreamID: "current"}},
				{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "current", EndOfStream: true}},
			},
			echo: "current input", want: "current input",
		},
		{
			name: "audio keeps ASR recording",
			chunks: []*genx.MessageChunk{
				{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "current", BeginOfStream: true, EndOfStream: true}},
			},
			echo: "recognized input", want: "recognized input",
		},
		{
			name: "interrupted input is uncommitted",
			chunks: []*genx.MessageChunk{
				{Role: genx.RoleUser, Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: "current", BeginOfStream: true}},
				{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "current", EndOfStream: true, Error: "interrupted"}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			records := newTestHistoryLogStore(t)
			history := workspace.NewHistoryStore(records, newTestObjectStore(t), "conversation")
			recorder := newHistoryRecorder(history, "gear", nil)
			builder := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			if err := builder.Add(test.chunks...); err != nil {
				t.Fatal(err)
			}
			if err := builder.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			input := &historyInputStream{Stream: builder.Stream(), ctx: t.Context(), recorder: recorder}
			defer input.Close()
			for {
				_, err := input.Next()
				if errors.Is(err, genx.ErrDone) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if test.echo != "" {
				if err := recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(test.echo), Ctrl: &genx.StreamCtrl{StreamID: "current", Label: "transcript", BeginOfStream: true, EndOfStream: true}}); err != nil {
					t.Fatal(err)
				}
			}
			page, err := records.Query(t.Context(), memory.HistoryQuery{ConversationID: "conversation", Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if test.want == "" {
				if len(page.Entries) != 0 {
					t.Fatalf("uncommitted input persisted: %+v", page.Entries)
				}
			} else if len(page.Entries) != 1 || page.Entries[0].Text != test.want || page.Entries[0].Role != memory.RoleUser {
				t.Fatalf("user input lost or duplicated: %+v", page.Entries)
			}
		})
	}
}
