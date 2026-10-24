package eino

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestOutputEpochRetainsProducerInput(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		name := "declared"
		if automatic {
			name = "first-output"
		}
		t.Run(name, func(t *testing.T) {
			config := textConfig()
			if automatic {
				config.Graph.Compile.PrimaryOutputMode = PrimaryFirstOutput
			}
			transformer, err := New(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer transformer.Close()
			input := newInputBuilder()
			if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("question"), Ctrl: &genx.StreamCtrl{StreamID: "input-owner", BeginOfStream: true, EndOfStream: true}}); err != nil {
				t.Fatal(err)
			}
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			output, err := transformer.Transform(t.Context(), input.Stream())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			var epoch *genx.ResponseEpoch
			completed := 0
			for _, chunk := range drain(t, output) {
				if chunk.Role != genx.RoleModel {
					continue
				}
				if chunk.Ctrl == nil || chunk.Ctrl.ResponseEpoch == nil || chunk.Ctrl.ResponseEpoch.InputStreamID() != "input-owner" {
					t.Fatalf("producer input provenance is missing: %#v", chunk.Ctrl)
				}
				if epoch == nil {
					epoch = chunk.Ctrl.ResponseEpoch
				} else if epoch != chunk.Ctrl.ResponseEpoch {
					t.Fatal("response epoch changed across output chunks")
				}
				if chunk.Ctrl.ResponseEpochEnd {
					completed++
				}
			}
			if epoch == nil || completed != 1 {
				t.Fatalf("epoch=%v completed=%d", epoch, completed)
			}
		})
	}
}
