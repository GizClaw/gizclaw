package genx

import (
	"context"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
)

// modelTiming belongs to one provider request and its single producer goroutine.
// Observing before enqueueing excludes consumer backpressure from first text.
type modelTiming struct {
	ctx     context.Context
	started time.Time
	labels  []gizmetrics.Label
	first   bool
}

func newModelTiming(ctx context.Context, provider, model string) *modelTiming {
	return &modelTiming{ctx: ctx, started: time.Now(), labels: []gizmetrics.Label{{Name: "provider", Value: provider}, {Name: "model", Value: model}}}
}
func (m *modelTiming) text(text string) {
	if m == nil || m.first || strings.TrimSpace(text) == "" {
		return
	}
	m.first = true
	gizmetrics.ObserveDuration(m.ctx, "genx_model_first_text_seconds", time.Since(m.started), m.labels...)
}
func (m *modelTiming) finish(err error) {
	if m == nil {
		return
	}
	labels := append(append([]gizmetrics.Label(nil), m.labels...), gizmetrics.Label{Name: "result", Value: gizmetrics.Result(err)})
	gizmetrics.ObserveDuration(m.ctx, "genx_model_request_duration_seconds", time.Since(m.started), labels...)
}
