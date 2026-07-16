// Package metrics defines a business-neutral store for numeric time series samples.
package metrics

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// Store persists numeric metric samples through backend-neutral queries.
type Store interface {
	Append(context.Context, []Sample) error
	Latest(context.Context, LatestQuery) (SeriesSet, error)
	Range(context.Context, RangeQuery) (SeriesSet, error)
	Aggregate(context.Context, AggregateQuery) (SeriesSet, error)
	Close() error
}

// Sample is one timestamped metric value.
type Sample struct {
	Name      string
	Labels    map[string]string
	Timestamp time.Time
	Value     float64
}

// LatestQuery selects the newest sample at or before At within Lookback.
type LatestQuery struct {
	Selector Selector
	At       time.Time
	Lookback time.Duration
}

// RangeQuery evaluates step-sized last-sample windows over a time range.
type RangeQuery struct {
	Selector   Selector
	Start, End time.Time
	Step       time.Duration
}

// AggregateQuery evaluates one aggregation per bucket anchored at Start.
type AggregateQuery struct {
	Selector   Selector
	Start, End time.Time
	Bucket     time.Duration
	Operation  Aggregation
}

// Point is one timestamped value returned by a query.
type Point struct {
	Timestamp time.Time
	Value     float64
}

// Series is one named metric stream returned by a query.
type Series struct {
	Name   string
	Labels map[string]string
	Points []Point
}

// SeriesSet is a query result containing zero or more series.
type SeriesSet []Series

// MatchOp is a label matcher operator.
type MatchOp string

const (
	MatchEqual     MatchOp = "="
	MatchNotEqual  MatchOp = "!="
	MatchRegexp    MatchOp = "=~"
	MatchNotRegexp MatchOp = "!~"
)

// LabelMatcher matches a label; a missing label has the empty value.
type LabelMatcher struct {
	Name  string
	Op    MatchOp
	Value string
}

// Selector identifies a metric and optional label matchers.
type Selector struct {
	Name     string
	Matchers []LabelMatcher
}

// Aggregation identifies a supported bucket aggregation.
type Aggregation string

const (
	AggregationAvg   Aggregation = "avg"
	AggregationMin   Aggregation = "min"
	AggregationMax   Aggregation = "max"
	AggregationSum   Aggregation = "sum"
	AggregationCount Aggregation = "count"
	AggregationLast  Aggregation = "last"
)

var (
	metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// ValidateMetricName validates a Prometheus-compatible metric name.
func ValidateMetricName(name string) error {
	if name == "" {
		return fmt.Errorf("metrics: metric name is empty")
	}
	if !metricNameRE.MatchString(name) {
		return fmt.Errorf("metrics: invalid metric name %q", name)
	}
	return nil
}

// ValidateLabelName validates a non-reserved label name.
func ValidateLabelName(name string) error {
	if name == "" {
		return fmt.Errorf("metrics: label name is empty")
	}
	if name == "__name__" {
		return fmt.Errorf("metrics: label name %q is reserved", name)
	}
	if !labelNameRE.MatchString(name) {
		return fmt.Errorf("metrics: invalid label name %q", name)
	}
	return nil
}
func validateSample(s Sample) error {
	if err := ValidateMetricName(s.Name); err != nil {
		return err
	}
	if s.Timestamp.IsZero() {
		return fmt.Errorf("metrics: sample %q timestamp is zero", s.Name)
	}
	for name := range s.Labels {
		if err := ValidateLabelName(name); err != nil {
			return err
		}
	}
	return nil
}
func validateSelector(s Selector) error {
	if err := ValidateMetricName(s.Name); err != nil {
		return err
	}
	for _, m := range s.Matchers {
		if err := validateMatcher(m); err != nil {
			return err
		}
		if m.Op == MatchRegexp || m.Op == MatchNotRegexp {
			if _, err := regexp.Compile("^(?:" + m.Value + ")$"); err != nil {
				return fmt.Errorf("metrics: invalid matcher regexp for %q: %w", m.Name, err)
			}
		}
	}
	return nil
}
func validateMatcher(m LabelMatcher) error {
	if err := ValidateLabelName(m.Name); err != nil {
		return err
	}
	switch m.Op {
	case MatchEqual, MatchNotEqual, MatchRegexp, MatchNotRegexp:
		return nil
	case "":
		return fmt.Errorf("metrics: label matcher %q operator is empty", m.Name)
	default:
		return fmt.Errorf("metrics: unsupported label matcher operator %q", m.Op)
	}
}
func validateLatestQuery(q LatestQuery) error {
	if err := validateSelector(q.Selector); err != nil {
		return err
	}
	if q.At.IsZero() {
		return fmt.Errorf("metrics: latest query time is zero")
	}
	if q.Lookback <= 0 {
		return fmt.Errorf("metrics: latest query lookback must be > 0")
	}
	return nil
}
func validateRangeQuery(q RangeQuery) error {
	if err := validateSelector(q.Selector); err != nil {
		return err
	}
	if q.Start.IsZero() || q.End.IsZero() {
		return fmt.Errorf("metrics: range query timestamps must be non-zero")
	}
	if q.End.Before(q.Start) {
		return fmt.Errorf("metrics: range query end is before start")
	}
	if q.Step <= 0 {
		return fmt.Errorf("metrics: range query step must be > 0")
	}
	return nil
}
func validateAggregateQuery(q AggregateQuery) error {
	if err := validateSelector(q.Selector); err != nil {
		return err
	}
	if q.Start.IsZero() || q.End.IsZero() {
		return fmt.Errorf("metrics: aggregate query timestamps must be non-zero")
	}
	if q.End.Before(q.Start) {
		return fmt.Errorf("metrics: aggregate query end is before start")
	}
	if q.Bucket <= 0 {
		return fmt.Errorf("metrics: aggregate query bucket must be > 0")
	}
	switch q.Operation {
	case AggregationAvg, AggregationMin, AggregationMax, AggregationSum, AggregationCount, AggregationLast:
		return nil
	default:
		return fmt.Errorf("metrics: unsupported aggregation %q", q.Operation)
	}
}
