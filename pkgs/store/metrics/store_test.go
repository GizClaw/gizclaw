package metrics

import (
	"math"
	"testing"
	"time"
)

func TestQueryValidation(t *testing.T) {
	t.Parallel()
	at := time.Now()
	valid := Selector{Name: "m", Matchers: []LabelMatcher{{Name: "missing", Op: MatchNotEqual, Value: "x"}, {Name: "label", Op: MatchRegexp, Value: "a.*"}}}
	if err := validateLatestQuery(LatestQuery{Selector: valid, At: at, Lookback: time.Second}); err != nil {
		t.Fatal(err)
	}
	if err := validateSelector(Selector{Name: "m", Matchers: []LabelMatcher{{Name: "x", Op: MatchRegexp, Value: "["}}}); err == nil {
		t.Fatal("expected regexp error")
	}
	for _, op := range []Aggregation{AggregationAvg, AggregationMin, AggregationMax, AggregationSum, AggregationCount, AggregationLast} {
		if err := validateAggregateQuery(AggregateQuery{Selector: Selector{Name: "m"}, Start: at, End: at, Bucket: time.Second, Operation: op}); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
	}
}
func TestSampleValidationAllowsIEEEValues(t *testing.T) {
	t.Parallel()
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := validateSample(Sample{Name: "m", Timestamp: time.Now(), Value: v}); err != nil {
			t.Fatal(err)
		}
	}
}
