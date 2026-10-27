package eino

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ComponentResolver resolves lower-level Eino components by stable package
// names. Implementations and returned components must be safe for concurrent
// use.
type ComponentResolver interface {
	ResolveChatModel(context.Context, string) (model.BaseChatModel, error)
	ResolveRetriever(context.Context, string) (retriever.Retriever, error)
}

// StructuredChatModel is an optional capability of a resolved chat model.
// It must use native schema enforcement, honor cancellation and model options,
// and return one complete assistant JSON object without executing runtime Tools.
// Output validation and publication remain owned by the Transformer.
type StructuredChatModel interface {
	GenerateStructured(context.Context, []*schema.Message, StructuredOutput, ...model.Option) (*schema.Message, error)
}

// ResolvedLambda is one caller-owned Eino Lambda and its declared port schema.
type ResolvedLambda struct {
	Lambda  *compose.Lambda
	Inputs  map[string]StateType
	Outputs map[string]StateType
}

// LambdaResolver resolves named application behavior without giving Config a
// raw callback or Graph factory.
type LambdaResolver interface {
	ResolveLambda(context.Context, string) (ResolvedLambda, error)
}
