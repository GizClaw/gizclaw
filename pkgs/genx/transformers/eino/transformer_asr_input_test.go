package eino

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/cloudwego/eino/schema"
)

func TestAudioDockCommitsOnlyDefiniteASRText(t *testing.T) {
	for _, test := range []struct {
		name        string
		texts       []string
		interim     []bool
		interrupted bool
		want        string
	}{
		{name: "replacement_hypotheses", texts: []string{"G.", "G.", "GIZ?", "Gizmod.", "Gist audio.", "Gist audio input.", "Gist audio input."}, interim: []bool{true, true, true, true, true, true, false}, want: "Gist audio input."},
		{name: "two_definite_segments_in_one_turn", texts: []string{"First?", "First. ", "Second?", "Second."}, interim: []bool{true, false, true, false}, want: "First. Second."},
		{name: "interim_only_interrupted", texts: []string{"G.", "GIZ?"}, interim: []bool{true, true}, interrupted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			chat := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("answer", nil)}}
			core, err := New(ctx, chatConfig(&componentMapResolver{chat: chat}))
			if err != nil {
				t.Fatal(err)
			}
			asr := latencyTransformer(func(context.Context, genx.Stream) (genx.Stream, error) {
				builder := newInputBuilder()
				for index, text := range test.texts {
					if err := builder.Add(&genx.MessageChunk{
						Role: genx.RoleUser, Name: "transcript", Part: genx.Text(text),
						Ctrl: &genx.StreamCtrl{StreamID: "speech", Label: "transcript", BeginOfStream: index == 0, TextInterim: test.interim[index]},
					}); err != nil {
						t.Fatal(err)
					}
				}
				terminal := &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "speech", Label: "transcript", EndOfStream: true}}
				if test.interrupted {
					terminal.Ctrl.Error = "interrupted"
				}
				if err := builder.Add(terminal); err != nil {
					t.Fatal(err)
				}
				if err := builder.Done(genx.Usage{}); err != nil {
					t.Fatal(err)
				}
				return builder.Stream(), nil
			})
			dock, err := audiodock.New(audiodock.Config{Agent: core, ASR: asr})
			if err != nil {
				t.Fatal(err)
			}
			input := newInputBuilder()
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			output, err := dock.Transform(ctx, input.Stream())
			if err != nil {
				t.Fatal(err)
			}
			var displayed []string
			for _, chunk := range drain(t, output) {
				if text, ok := chunk.Part.(genx.Text); ok && text != "" && chunk.Role == genx.RoleUser {
					displayed = append(displayed, string(text))
				}
			}
			if !slices.Equal(displayed, test.texts) {
				t.Fatalf("client transcript = %q, want %q", displayed, test.texts)
			}
			chat.mu.Lock()
			inputs := slices.Clone(chat.inputs)
			chat.mu.Unlock()
			if test.interrupted {
				if len(inputs) != 0 {
					t.Fatalf("interim-only interruption called ChatModel %d times", len(inputs))
				}
			} else {
				if len(inputs) != 1 {
					t.Fatalf("ChatModel calls = %d, want 1", len(inputs))
				}
				var users []string
				for _, message := range inputs[0] {
					if message.Role == schema.User {
						users = append(users, message.Content)
					}
				}
				if !slices.Equal(users, []string{test.want}) {
					t.Fatalf("ChatModel user messages = %q, want [%q]", users, test.want)
				}
			}
			messages, err := core.history.load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var historyUsers []string
			for _, message := range messages {
				if message.Role == schema.User {
					historyUsers = append(historyUsers, message.Content)
				}
			}
			if test.interrupted && len(historyUsers) != 0 || !test.interrupted && !slices.Equal(historyUsers, []string{test.want}) {
				t.Fatalf("History user messages = %q, want final text %q only", historyUsers, test.want)
			}
		})
	}
}

func TestAudioDockInterimInterruptsBeforeDefiniteText(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	chat := &interruptingChatModel{firstStarted: make(chan struct{}), firstCancelled: make(chan struct{})}
	core, err := New(ctx, chatConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	transcripts := newInputBuilder()
	dock, err := audiodock.New(audiodock.Config{
		Agent: core,
		ASR: latencyTransformer(func(context.Context, genx.Stream) (genx.Stream, error) {
			return transcripts.Stream(), nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	output, err := dock.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var outputErr error
	go func() {
		defer close(done)
		for {
			if _, err := output.Next(); err != nil {
				outputErr = err
				return
			}
		}
	}()
	defer func() { cancel(); _ = output.Close(); <-done }()
	addTextTurn(t, transcripts, "first")
	select {
	case <-chat.firstStarted:
	case <-ctx.Done():
		t.Fatal("first model did not start")
	}
	interim := textChunk("replacement", "unstable hypothesis", true, false)
	interim.Ctrl.TextInterim = true
	if err := transcripts.Add(interim); err != nil {
		t.Fatal(err)
	}
	// No definite text or EOS is available yet. Cancellation must depend on
	// the preserved BOS, even though Audio Dock removes the interim payload.
	select {
	case <-chat.firstCancelled:
	case <-ctx.Done():
		t.Fatal("interim BOS did not interrupt before definite text")
	}
	chat.mu.Lock()
	calls := chat.calls
	chat.mu.Unlock()
	if calls != 1 {
		t.Fatalf("interim started a new model call: calls=%d", calls)
	}
	if err := transcripts.Add(textChunk("replacement", "definite", false, true)); err != nil {
		t.Fatal(err)
	}
	if err := transcripts.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("replacement did not finish")
	}
	if !errors.Is(outputErr, io.EOF) {
		t.Fatalf("output ended with %v", outputErr)
	}
	history, err := core.history.load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, message := range history {
		if message.Role == schema.User {
			users = append(users, message.Content)
		}
	}
	if !slices.Equal(users, []string{"first", "definite"}) {
		t.Fatalf("history users = %q, want only definite text", users)
	}
}
