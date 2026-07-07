// Package metrics defines a business-neutral store for numeric time series
// samples.
package metrics

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Store persists numeric metric samples and queries them back as time series.
type Store interface {
	Append(ctx context.Context, samples []Sample) error
	Query(ctx context.Context, query Query) (SeriesSet, error)
	QueryRange(ctx context.Context, query RangeQuery) (SeriesSet, error)
	Close() error
}

// Sample is one timestamped metric value.
type Sample struct {
	Name      string
	Labels    map[string]string
	Timestamp time.Time
	Value     float64
}

// Query is an instant Prometheus-compatible metric query.
type Query struct {
	Expression string
	Time       time.Time
}

// RangeQuery is a Prometheus-compatible metric range query.
type RangeQuery struct {
	Expression string
	Start      time.Time
	End        time.Time
	Step       time.Duration
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

// SeriesSet is the result of a query.
type SeriesSet []Series

// MatchOp is a Prometheus label matcher operator.
type MatchOp string

const (
	MatchEqual     MatchOp = "="
	MatchNotEqual  MatchOp = "!="
	MatchRegexp    MatchOp = "=~"
	MatchNotRegexp MatchOp = "!~"
)

// LabelMatcher matches a metric label in a selector.
type LabelMatcher struct {
	Name  string
	Op    MatchOp
	Value string
}

// Selector describes a metric selector before it is rendered to PromQL.
type Selector struct {
	Name     string
	Matchers []LabelMatcher
}

// Expression renders a PromQL selector such as metric{peer_id="abc"}.
func (s Selector) Expression() (string, error) {
	if err := ValidateMetricName(s.Name); err != nil {
		return "", err
	}
	matchers := slices.Clone(s.Matchers)
	slices.SortFunc(matchers, func(a, b LabelMatcher) int {
		return strings.Compare(a.Name, b.Name)
	})
	if len(matchers) == 0 {
		return s.Name, nil
	}
	parts := make([]string, 0, len(matchers))
	for _, matcher := range matchers {
		if err := validateMatcher(matcher); err != nil {
			return "", err
		}
		parts = append(parts, matcher.Name+string(matcher.Op)+strconv.Quote(matcher.Value))
	}
	return s.Name + "{" + strings.Join(parts, ",") + "}", nil
}

// Aggregation is a supported PromQL aggregation operator.
type Aggregation string

const (
	AggregationAvg   Aggregation = "avg"
	AggregationMin   Aggregation = "min"
	AggregationMax   Aggregation = "max"
	AggregationSum   Aggregation = "sum"
	AggregationCount Aggregation = "count"
)

// AggregateExpression wraps expr with a supported PromQL aggregation.
func AggregateExpression(aggregation Aggregation, expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("metrics: query expression is empty")
	}
	switch aggregation {
	case AggregationAvg, AggregationMin, AggregationMax, AggregationSum, AggregationCount:
		return string(aggregation) + "(" + expr + ")", nil
	default:
		return "", fmt.Errorf("metrics: unsupported aggregation %q", aggregation)
	}
}

var (
	metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// ValidateMetricName checks whether name is a valid Prometheus metric name.
func ValidateMetricName(name string) error {
	if name == "" {
		return fmt.Errorf("metrics: metric name is empty")
	}
	if !metricNameRE.MatchString(name) {
		return fmt.Errorf("metrics: invalid metric name %q", name)
	}
	return nil
}

// ValidateLabelName checks whether name is a valid non-reserved label name.
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

func validateSample(sample Sample) error {
	if err := ValidateMetricName(sample.Name); err != nil {
		return err
	}
	if sample.Timestamp.IsZero() {
		return fmt.Errorf("metrics: sample %q timestamp is zero", sample.Name)
	}
	for name := range sample.Labels {
		if err := ValidateLabelName(name); err != nil {
			return err
		}
	}
	return nil
}

func validateQueryExpression(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("metrics: query expression is empty")
	}
	return nil
}

func validateMatcher(matcher LabelMatcher) error {
	if err := ValidateLabelName(matcher.Name); err != nil {
		return err
	}
	switch matcher.Op {
	case MatchEqual, MatchNotEqual, MatchRegexp, MatchNotRegexp:
		return nil
	case "":
		return fmt.Errorf("metrics: label matcher %q operator is empty", matcher.Name)
	default:
		return fmt.Errorf("metrics: unsupported label matcher operator %q", matcher.Op)
	}
}
