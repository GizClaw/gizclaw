package gizrun

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/labelset"
)

const (
	nsHTTP    = "http"
	nsGenx    = "genx"
	nsLogSink = "logsink"
)

const (
	httpMethod     = "method"
	httpPath       = "path"
	httpHost       = "host"
	httpStatusCode = "status_code"
)

const (
	genxProvider  = "provider"
	genxMethod    = "method"
	genxModel     = "model"
	genxStatus    = "status"
	genxTokenType = "token_type"
)

const (
	tokenCached    = "cached"
	tokenGenerated = "generated"
	tokenPrompt    = "prompt"
)

func tagHTTP(ctx context.Context, kvs ...string) context.Context {
	return labelset.Tag(ctx, nsHTTP, kvs...)
}

func tagGenx(ctx context.Context, kvs ...string) context.Context {
	return labelset.Tag(ctx, nsGenx, kvs...)
}

func tagLogSink(ctx context.Context, kvs ...string) context.Context {
	return labelset.Tag(ctx, nsLogSink, kvs...)
}

func tag(ctx context.Context, name string, kvs ...string) context.Context {
	return labelset.Tag(ctx, name, kvs...)
}

func httpLabels(ctx context.Context) (labelset.LabelSet, bool) {
	return labelset.FromContext(ctx, nsHTTP)
}

func genxLabels(ctx context.Context) (labelset.LabelSet, bool) {
	return labelset.FromContext(ctx, nsGenx)
}

func logSinkLabels(ctx context.Context) (labelset.LabelSet, bool) {
	return labelset.FromContext(ctx, nsLogSink)
}

func labels(ctx context.Context, namespace string) (labelset.LabelSet, bool) {
	return labelset.FromContext(ctx, namespace)
}
