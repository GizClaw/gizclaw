package metrics

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMemoryStoreAppendQueryAndRange(t *testing.T) {
	store := NewMemoryStore()
	base := time.Unix(100, 0).UTC()
	if err := store.Append(context.Background(), []Sample{
		{Name: "gizclaw_peer_battery_percent", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base, Value: 80},
		{Name: "gizclaw_peer_battery_percent", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base.Add(time.Second), Value: 81},
		{Name: "gizclaw_peer_battery_percent", Labels: map[string]string{"peer_id": "p2"}, Timestamp: base, Value: 10},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	query, err := (Selector{
		Name:     "gizclaw_peer_battery_percent",
		Matchers: []LabelMatcher{{Name: "peer_id", Op: MatchEqual, Value: "p1"}},
	}).Expression()
	if err != nil {
		t.Fatalf("Expression: %v", err)
	}
	got, err := store.Query(context.Background(), Query{Expression: query, Time: base.Add(1500 * time.Millisecond)})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 1 || got[0].Points[0].Value != 81 {
		t.Fatalf("Query result = %+v", got)
	}

	got, err = store.QueryRange(context.Background(), RangeQuery{
		Expression: query,
		Start:      base,
		End:        base.Add(time.Second),
		Step:       time.Second,
	})
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("QueryRange result = %+v", got)
	}
	if got[0].Points[0].Value != 80 || got[0].Points[1].Value != 81 {
		t.Fatalf("QueryRange points = %+v", got[0].Points)
	}
}

func TestMemoryStoreAppendInvalidBatchDoesNotMutate(t *testing.T) {
	store := NewMemoryStore()
	base := time.Unix(100, 0).UTC()
	err := store.Append(context.Background(), []Sample{
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base, Value: 1},
		{Name: "bad-name", Timestamp: base, Value: 2},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid metric name") {
		t.Fatalf("Append invalid batch error = %v, want invalid metric name", err)
	}
	got, err := store.Query(context.Background(), Query{Expression: "metric_a", Time: base})
	if err != nil {
		t.Fatalf("Query after invalid batch: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("invalid batch mutated store: %+v", got)
	}
}

func TestMemoryStoreMatchersAndErrors(t *testing.T) {
	store := NewMemoryStore()
	base := time.Unix(100, 0).UTC()
	if err := store.Append(context.Background(), []Sample{
		{Name: "metric_a", Labels: map[string]string{"peer_id": "peer-a"}, Timestamp: base, Value: 1},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "peer-b"}, Timestamp: base, Value: 2},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "peer-ab"}, Timestamp: base, Value: 3},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := store.Query(context.Background(), Query{Expression: `metric_a{peer_id=~"peer-[ab]"}`, Time: base})
	if err != nil {
		t.Fatalf("Query regexp: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("regexp query series = %d, want 2: %+v", len(got), got)
	}
	got, err = store.Query(context.Background(), Query{Expression: `metric_a{peer_id=~"peer-a"}`, Time: base})
	if err != nil {
		t.Fatalf("Query anchored regexp: %v", err)
	}
	if len(got) != 1 || got[0].Labels["peer_id"] != "peer-a" {
		t.Fatalf("anchored regexp query = %+v, want only peer-a", got)
	}
	if err := store.Append(context.Background(), []Sample{
		{Name: "metric_b", Labels: map[string]string{"label": "a!=b", "pattern": "x=y"}, Timestamp: base, Value: 4},
	}); err != nil {
		t.Fatalf("Append operator label values: %v", err)
	}
	got, err = store.Query(context.Background(), Query{Expression: `metric_b{label="a!=b",pattern=~"x=y"}`, Time: base})
	if err != nil {
		t.Fatalf("Query operator label values: %v", err)
	}
	if len(got) != 1 || got[0].Labels["label"] != "a!=b" {
		t.Fatalf("operator label value query = %+v, want metric_b", got)
	}
	if _, err := store.Query(context.Background(), Query{Expression: `metric_a{peer_id=`}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("bad selector error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := store.Query(context.Background(), Query{Expression: "metric_a"}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("closed Query error = %v", err)
	}
}

func TestMemoryStoreQueryZeroTimeUsesNow(t *testing.T) {
	store := NewMemoryStore()
	if err := store.Append(context.Background(), []Sample{
		{Name: "metric_a", Timestamp: time.Now().Add(time.Hour).UTC(), Value: 1},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := store.Query(context.Background(), Query{Expression: "metric_a"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Query zero time = %+v, want no future samples", got)
	}
}

func TestMemoryStoreRangeHonorsStepAndAggregates(t *testing.T) {
	store := NewMemoryStore()
	base := time.Unix(1000, 0).UTC()
	if err := store.Append(context.Background(), []Sample{
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base, Value: 1},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base.Add(time.Second), Value: 2},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base.Add(2 * time.Second), Value: 3},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p2"}, Timestamp: base, Value: 10},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p2"}, Timestamp: base.Add(2 * time.Second), Value: 20},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := store.QueryRange(context.Background(), RangeQuery{
		Expression: "metric_a",
		Start:      base,
		End:        base.Add(2 * time.Second),
		Step:       2 * time.Second,
	})
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("QueryRange series = %+v, want 2", got)
	}
	for _, series := range got {
		if len(series.Points) != 2 {
			t.Fatalf("series %s points = %+v, want 2 step-aligned points", series.Labels["peer_id"], series.Points)
		}
		if !series.Points[0].Timestamp.Equal(base) || !series.Points[1].Timestamp.Equal(base.Add(2*time.Second)) {
			t.Fatalf("series %s timestamps = %+v, want step timestamps", series.Labels["peer_id"], series.Points)
		}
	}

	got, err = store.Query(context.Background(), Query{
		Expression: "avg(metric_a)",
		Time:       base.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("Query aggregate: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 1 || got[0].Points[0].Value != 11.5 {
		t.Fatalf("aggregate query = %+v, want avg 11.5", got)
	}

	got, err = store.QueryRange(context.Background(), RangeQuery{
		Expression: "sum(metric_a)",
		Start:      base,
		End:        base.Add(2 * time.Second),
		Step:       time.Second,
	})
	if err != nil {
		t.Fatalf("QueryRange aggregate: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 3 {
		t.Fatalf("aggregate range = %+v, want 3 points", got)
	}
	if got[0].Points[0].Value != 11 || got[0].Points[1].Value != 12 || got[0].Points[2].Value != 23 {
		t.Fatalf("aggregate range points = %+v", got[0].Points)
	}
}

func TestMemoryStoreOverTimeFunctions(t *testing.T) {
	store := NewMemoryStore()
	base := time.Unix(1000, 0).UTC()
	if err := store.Append(context.Background(), []Sample{
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base, Value: 1},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base.Add(2 * time.Minute), Value: 3},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p2"}, Timestamp: base.Add(time.Minute), Value: 10},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := store.Query(context.Background(), Query{
		Expression: `last_over_time(metric_a{peer_id="p1"}[30m])`,
		Time:       base.Add(20 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Query last_over_time: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 1 || got[0].Points[0].Value != 3 {
		t.Fatalf("last_over_time query = %+v, want p1 last value", got)
	}
	if !got[0].Points[0].Timestamp.Equal(base.Add(2 * time.Minute)) {
		t.Fatalf("last_over_time timestamp = %s, want actual sample timestamp", got[0].Points[0].Timestamp)
	}

	got, err = store.Query(context.Background(), Query{
		Expression: `metric_a{peer_id="p1"}[30m]`,
		Time:       base.Add(20 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Query raw range selector: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("raw range selector = %+v, want p1 raw samples", got)
	}
	if !got[0].Points[0].Timestamp.Equal(base) || got[0].Points[0].Value != 1 || !got[0].Points[1].Timestamp.Equal(base.Add(2*time.Minute)) || got[0].Points[1].Value != 3 {
		t.Fatalf("raw range selector points = %+v", got[0].Points)
	}

	got, err = store.QueryRange(context.Background(), RangeQuery{
		Expression: `sum_over_time(metric_a{peer_id="p1"}[2m])`,
		Start:      base.Add(2 * time.Minute),
		End:        base.Add(4 * time.Minute),
		Step:       2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("QueryRange sum_over_time: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 1 {
		t.Fatalf("sum_over_time range = %+v, want 1 point", got)
	}
	if got[0].Points[0].Value != 3 {
		t.Fatalf("sum_over_time points = %+v", got[0].Points)
	}
	if !got[0].Points[0].Timestamp.Equal(base.Add(2 * time.Minute)) {
		t.Fatalf("sum_over_time timestamp = %s, want evaluation timestamp", got[0].Points[0].Timestamp)
	}
}

func TestMemoryStoreQueriesRespectLookback(t *testing.T) {
	store := NewMemoryStore()
	base := time.Unix(1000, 0).UTC()
	if err := store.Append(context.Background(), []Sample{
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p1"}, Timestamp: base, Value: 1},
		{Name: "metric_a", Labels: map[string]string{"peer_id": "p2"}, Timestamp: base.Add(6 * time.Minute), Value: 2},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := store.Query(context.Background(), Query{
		Expression: "metric_a",
		Time:       base.Add(MemoryStoreDefaultLookback).Add(time.Millisecond),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("stale instant query = %+v, want empty", got)
	}

	got, err = store.QueryRange(context.Background(), RangeQuery{
		Expression: "metric_a",
		Start:      base,
		End:        base.Add(6 * time.Minute),
		Step:       3 * time.Minute,
	})
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("range series = %+v, want 2 series", got)
	}
	for _, series := range got {
		if series.Labels["peer_id"] == "p1" && len(series.Points) != 2 {
			t.Fatalf("p1 points = %+v, want points only within lookback", series.Points)
		}
		if series.Labels["peer_id"] == "p2" && len(series.Points) != 1 {
			t.Fatalf("p2 points = %+v, want fresh point only", series.Points)
		}
	}

	got, err = store.QueryRange(context.Background(), RangeQuery{
		Expression: "sum(metric_a)",
		Start:      base,
		End:        base.Add(6 * time.Minute),
		Step:       3 * time.Minute,
	})
	if err != nil {
		t.Fatalf("QueryRange aggregate: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 3 {
		t.Fatalf("aggregate range = %+v, want 3 non-empty points", got)
	}
	if got[0].Points[0].Value != 1 || got[0].Points[1].Value != 1 || got[0].Points[2].Value != 2 {
		t.Fatalf("aggregate range points = %+v", got[0].Points)
	}
}
