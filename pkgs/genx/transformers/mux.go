package transformers

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/trie"
)

var _ genx.TransformerMux = (*Mux)(nil)

// DefaultMux is the default transformer multiplexer.
var DefaultMux = NewMux()

// Handle registers a transformer for the given pattern to the default mux.
func Handle(pattern string, t genx.Transformer) error {
	return DefaultMux.Handle(pattern, t)
}

// Transform applies the transformer registered for the pattern using the default mux.
func Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	return DefaultMux.Transform(ctx, pattern, input)
}

// Mux routes requests to registered transformers based on pattern matching.
type Mux struct {
	mux *trie.Trie[genx.Transformer]
}

// NewMux creates a new transformer multiplexer.
func NewMux() *Mux {
	return &Mux{mux: trie.New[genx.Transformer]()}
}

// Handle registers a transformer for the given pattern.
func (m *Mux) Handle(pattern string, t genx.Transformer) error {
	return m.mux.Set(pattern, func(ptr *genx.Transformer, existed bool) error {
		if existed {
			return fmt.Errorf("transformers: transformer already registered for %s", pattern)
		}
		*ptr = t
		return nil
	})
}

// Transform routes to the transformer registered for the given pattern.
func (m *Mux) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	t, err := m.get(pattern)
	if err != nil {
		return nil, err
	}
	return t.Transform(ctx, input)
}

func (m *Mux) get(pattern string) (genx.Transformer, error) {
	ptr, ok := m.mux.Get(pattern)
	if !ok {
		return nil, fmt.Errorf("transformers: transformer not found for %s", pattern)
	}
	t := *ptr
	if t == nil {
		return nil, fmt.Errorf("transformers: transformer not found for %s", pattern)
	}
	return t, nil
}

// streamToReader converts a genx.Stream of Text chunks to an io.Reader.
func streamToReader(stream genx.Stream) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		for {
			chunk, err := stream.Next()
			if err != nil {
				if !errors.Is(err, genx.ErrDone) && !errors.Is(err, io.EOF) {
					pw.CloseWithError(err)
				}
				return
			}

			if chunk == nil {
				continue
			}

			if text, ok := chunk.Part.(genx.Text); ok {
				if _, err := pw.Write([]byte(text)); err != nil {
					return
				}
			}
		}
	}()

	return pr
}
