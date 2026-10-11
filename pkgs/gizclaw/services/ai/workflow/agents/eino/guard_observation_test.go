package eino

import (
	"context"
	"io"
	"testing"
	"testing/synctest"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type guardObservedSource struct {
	chunk     *genx.MessageChunk
	pulled    bool
	deferred  bool
	observed  *genx.MessageChunk
	abandoned *genx.MessageChunk
}

func (s *guardObservedSource) Next() (*genx.MessageChunk, error) {
	if s.pulled {
		return nil, io.EOF
	}
	s.pulled = true
	if !s.deferred {
		s.observed = s.chunk
	}
	return s.chunk, nil
}
func (*guardObservedSource) Close() error                                        { return nil }
func (*guardObservedSource) CloseWithError(error) error                          { return nil }
func (s *guardObservedSource) DeferOutputObservation()                           { s.deferred = true }
func (s *guardObservedSource) ObserveOutput(chunk *genx.MessageChunk)            { s.observed = chunk }
func (s *guardObservedSource) AbandonOutputObservation(chunk *genx.MessageChunk) { s.abandoned = chunk }

type guardObservedTransformer struct{ source *guardObservedSource }

func (g guardObservedTransformer) Transform(context.Context, genx.Stream) (genx.Stream, error) {
	return g.source, nil
}

func TestAudioInputGuardPreservesFinalDeliveryObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &guardObservedSource{chunk: &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("narration")}}
		input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
		_ = input.Done(genx.Usage{})
		output, err := (einoAudioInputGuard{next: guardObservedTransformer{source}}).Transform(t.Context(), input.Stream())
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		stream := output.(*einoAudioOutputStream)
		stream.DeferOutputObservation()
		synctest.Wait()
		if !source.deferred || source.observed != nil {
			t.Fatal("guard acknowledged read-ahead output")
		}
		chunk, err := stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if source.observed != nil {
			t.Fatal("guard acknowledged before the final consumer")
		}
		stream.ObserveOutput(chunk)
		stream.AbandonOutputObservation(chunk)
		if source.observed != source.chunk || source.abandoned != source.chunk {
			t.Fatal("guard lost exact acknowledgement identity")
		}
	})
}
