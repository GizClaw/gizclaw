package metrics

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestMemoryStoreStructuredQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()
	store := NewMemoryStore()
	selector := Selector{Name: "temperature", Matchers: []LabelMatcher{{Name: "peer", Op: MatchEqual, Value: "a"}}}
	samples := []Sample{
		{Name: "temperature", Labels: map[string]string{"peer": "a"}, Timestamp: base, Value: 1},
		{Name: "temperature", Labels: map[string]string{"peer": "a"}, Timestamp: base.Add(time.Minute), Value: 2},
		{Name: "temperature", Labels: map[string]string{"peer": "a"}, Timestamp: base.Add(9 * time.Minute / 4), Value: 5},
		{Name: "temperature", Labels: map[string]string{"peer": "a"}, Timestamp: base.Add(3 * time.Minute), Value: 4},
		{Name: "temperature", Labels: map[string]string{"peer": "b"}, Timestamp: base, Value: 99},
	}
	if err := store.Append(ctx, samples); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(ctx, LatestQuery{Selector: selector, At: base.Add(2 * time.Minute), Lookback: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if got := latest[0].Points[0]; got.Value != 2 || !got.Timestamp.Equal(base.Add(time.Minute)) {
		t.Fatalf("Latest=%+v", got)
	}
	rangeSet, err := store.Range(ctx, RangeQuery{Selector: selector, Start: base, End: base.Add(5 * time.Minute / 2), Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	want := []Point{{base, 1}, {base.Add(time.Minute), 2}, {base.Add(5 * time.Minute / 2), 5}}
	if !pointsEqual(rangeSet[0].Points, want) {
		t.Fatalf("Range=%+v want %+v", rangeSet[0].Points, want)
	}
	agg, err := store.Aggregate(ctx, AggregateQuery{Selector: selector, Start: base, End: base.Add(3 * time.Minute), Bucket: 2 * time.Minute, Operation: AggregationAvg})
	if err != nil {
		t.Fatal(err)
	}
	want = []Point{{base, 1.5}, {base.Add(2 * time.Minute), 4.5}}
	if !pointsEqual(agg[0].Points, want) {
		t.Fatalf("Aggregate=%+v want %+v", agg[0].Points, want)
	}
}

func TestMemoryStoreMatchersAndFloatValues(t *testing.T) {
	t.Parallel()
	s := NewMemoryStore()
	at := time.Now().UTC()
	if err := s.Append(context.Background(), []Sample{{Name: "m", Timestamp: at, Value: math.Inf(1)}, {Name: "m", Labels: map[string]string{"x": "abc"}, Timestamp: at, Value: math.NaN()}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Latest(context.Background(), LatestQuery{Selector: Selector{Name: "m", Matchers: []LabelMatcher{{Name: "x", Op: MatchRegexp, Value: "a.*"}}}, At: at, Lookback: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !math.IsNaN(got[0].Points[0].Value) {
		t.Fatalf("got %+v", got)
	}
	got, err = s.Latest(context.Background(), LatestQuery{Selector: Selector{Name: "m", Matchers: []LabelMatcher{{Name: "x", Op: MatchRegexp, Value: "a"}}}, At: at, Lookback: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("regexp must be fully anchored: %+v", got)
	}
}
func TestMemoryStoreValidationAndClose(t *testing.T) {
	t.Parallel()
	s := NewMemoryStore()
	if _, err := s.Latest(context.Background(), LatestQuery{}); err == nil {
		t.Fatal("expected validation error")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(context.Background(), []Sample{{Name: "m", Timestamp: time.Now()}}); err == nil {
		t.Fatal("expected closed error")
	}
}
func pointsEqual(a, b []Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Timestamp.Equal(b[i].Timestamp) || a[i].Value != b[i].Value {
			return false
		}
	}
	return true
}
