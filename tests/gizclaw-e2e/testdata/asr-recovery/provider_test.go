package peergenx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestASRRecoveryFixtureProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	transformer, err := newASRRecoveryFixture(TransformerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(recoveryFixture().Close)
	for turn := 1; turn <= 9; turn++ {
		input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
		output, err := transformer.Transform(ctx, input.Stream())
		if err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprintf("turn-%d", turn)
		if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 3200)}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}); err != nil {
			t.Fatal(err)
		}
		if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}}); err != nil {
			t.Fatal(err)
		}
		if err := input.Done(genx.Usage{}); err != nil {
			t.Fatal(err)
		}
		var text strings.Builder
		for {
			chunk, err := output.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
					t.Fatal(err)
				}
				break
			}
			if value, ok := chunk.Part.(genx.Text); ok {
				text.WriteString(string(value))
			}
		}
		_ = output.Close()
		if got, want := text.String(), fmt.Sprintf("answer %d", turn); got != want {
			t.Fatalf("turn %d: %q, want %q", turn, got, want)
		}
	}
}
