package genx

import "context"

// Transformer converts one Stream into another Stream.
type Transformer interface {
	Transform(ctx context.Context, input Stream) (Stream, error)
}

// TransformerMux resolves a pattern to a Transformer and applies it to a Stream.
// Pattern routing belongs to composition layers rather than concrete Transformers.
type TransformerMux interface {
	Transform(ctx context.Context, pattern string, input Stream) (Stream, error)
}
