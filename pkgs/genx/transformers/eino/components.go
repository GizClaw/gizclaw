package eino

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
)

// ComponentResolver resolves lower-level Eino components by stable package
// names. Implementations and returned components must be safe for concurrent
// use.
type ComponentResolver interface {
	ResolveChatModel(context.Context, string) (model.BaseChatModel, error)
	ResolveRetriever(context.Context, string) (retriever.Retriever, error)
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
