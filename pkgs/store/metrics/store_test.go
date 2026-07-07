package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestSelectorExpression(t *testing.T) {
	expr, err := (Selector{
		Name: "gizclaw_peer_battery_percent",
		Matchers: []LabelMatcher{
			{Name: "peer_id", Op: MatchEqual, Value: `peer"1`},
			{Name: "kind", Op: MatchRegexp, Value: "battery|gnss"},
		},
	}).Expression()
	if err != nil {
		t.Fatalf("Expression: %v", err)
	}
	want := `gizclaw_peer_battery_percent{kind=~"battery|gnss",peer_id="peer\"1"}`
	if expr != want {
		t.Fatalf("Expression = %q, want %q", expr, want)
	}
}

func TestSelectorExpressionRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		sel  Selector
		want string
	}{
		{
			name: "metric",
			sel:  Selector{Name: "1bad"},
			want: "invalid metric name",
		},
		{
			name: "label",
			sel: Selector{Name: "ok", Matchers: []LabelMatcher{
				{Name: "__name__", Op: MatchEqual, Value: "bad"},
			}},
			want: "reserved",
		},
		{
			name: "operator",
			sel: Selector{Name: "ok", Matchers: []LabelMatcher{
				{Name: "peer_id", Op: "==", Value: "bad"},
			}},
			want: "unsupported label matcher operator",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.sel.Expression(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Expression error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestAggregateExpression(t *testing.T) {
	expr, err := AggregateExpression(AggregationAvg, `gizclaw_peer_battery_percent{peer_id="p1"}`)
	if err != nil {
		t.Fatalf("AggregateExpression: %v", err)
	}
	want := `avg(gizclaw_peer_battery_percent{peer_id="p1"})`
	if expr != want {
		t.Fatalf("AggregateExpression = %q, want %q", expr, want)
	}
	if _, err := AggregateExpression(Aggregation("median"), "metric"); err == nil {
		t.Fatal("expected unsupported aggregation error")
	}
	if _, err := AggregateExpression(AggregationAvg, " "); err == nil {
		t.Fatal("expected empty expression error")
	}
}

func TestValidateSample(t *testing.T) {
	ts := time.Unix(1, 0)
	if err := validateSample(Sample{Name: "ok_metric", Labels: map[string]string{"peer_id": "p1"}, Timestamp: ts}); err != nil {
		t.Fatalf("validateSample valid: %v", err)
	}
	tests := []struct {
		name   string
		sample Sample
		want   string
	}{
		{name: "metric", sample: Sample{Name: "bad-name", Timestamp: ts}, want: "invalid metric name"},
		{name: "timestamp", sample: Sample{Name: "ok_metric"}, want: "timestamp is zero"},
		{name: "label", sample: Sample{Name: "ok_metric", Labels: map[string]string{"bad-label": "x"}, Timestamp: ts}, want: "invalid label name"},
		{name: "reserved", sample: Sample{Name: "ok_metric", Labels: map[string]string{"__name__": "x"}, Timestamp: ts}, want: "reserved"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSample(tc.sample); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateSample error = %v, want %q", err, tc.want)
			}
		})
	}
}
