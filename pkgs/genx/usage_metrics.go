package genx

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
)

// Provider adapters supply configured billing identities, never user, request,
// session, voice or workflow IDs. The process recorder also bounds total series.
func recordTokenMetrics(ctx context.Context, record UsageRecord) {
	if record.Unit != UsageUnitToken ||
		(record.Modality != UsageModalityText && record.Modality != UsageModalityAudio) ||
		!validUsageMetricIdentity(record.Provider, 64) || !validUsageMetricIdentity(record.Model, 256) ||
		record.Input < 0 || record.CachedInput < 0 || record.Output < 0 {
		return
	}
	base := [4]gizmetrics.Label{
		{Name: "provider", Value: record.Provider},
		{Name: "model", Value: record.Model},
		{Name: "modality", Value: string(record.Modality)},
		{Name: "token_type"},
	}
	var labels [3][4]gizmetrics.Label
	var deltas [3]gizmetrics.CounterDelta
	count := 0
	for _, quantity := range [...]struct {
		kind  string
		count int64
	}{{"input", record.Input}, {"cached_input", record.CachedInput}, {"output", record.Output}} {
		if quantity.count > 0 {
			labels[count] = base
			labels[count][3].Value = quantity.kind
			deltas[count] = gizmetrics.CounterDelta{Value: float64(quantity.count), Labels: labels[count][:]}
			count++
		}
	}
	gizmetrics.AddPeriodicCounters(ctx, "genx_model_tokens_total", deltas[:count]...)
}

func validUsageMetricIdentity(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsFunc(value, unicode.IsControl)
}
