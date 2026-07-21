package flowcraft

import (
	"testing"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestGenXStreamPreservesTextOnEOS(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		errorText string
	}{
		{name: "success"},
		{name: "error", errorText: "provider failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
			if err := builder.Add(&genx.MessageChunk{
				Role: genx.RoleModel, Part: genx.Text("final"),
				Ctrl: &genx.StreamCtrl{EndOfStream: true, Error: test.errorText},
			}); err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			_ = builder.Done(genx.Usage{})
			stream := &genXStream{stream: builder.Stream()}
			if !stream.Next() {
				t.Fatalf("Next() = false, error = %v", stream.Err())
			}
			if got := stream.Current(); got.Role != flowmodel.RoleAssistant || got.Content != "final" {
				t.Fatalf("Current() = %#v", got)
			}
			if stream.Next() {
				t.Fatal("second Next() = true")
			}
			if test.errorText == "" && stream.Err() != nil {
				t.Fatalf("Err() = %v", stream.Err())
			}
			if test.errorText != "" && stream.Err() == nil {
				t.Fatal("Err() = nil")
			}
			if test.errorText != "" && stream.Err().Error() != test.errorText {
				t.Fatalf("Err() = %v", stream.Err())
			}
		})
	}
}
