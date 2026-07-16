package peertelemetry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

var (
	ErrMetricsStoreNil = errors.New("peertelemetry: metrics store is nil")
	ErrInvalidQuery    = errors.New("peertelemetry: invalid telemetry query")
)

const (
	defaultAdminRangeLimit = 240
	maxAdminRangeLimit     = 1000
	minAdminStep           = time.Second
	defaultLatestLookback  = 30 * 24 * time.Hour
)

type AdminService struct {
	Metrics metrics.Store
	Now     func() time.Time
}

func (s *AdminService) Latest(ctx context.Context, peer giznet.PublicKey, fields []apitypes.PeerTelemetryField) (apitypes.PeerTelemetryLatestResponse, error) {
	if peer.IsZero() {
		return apitypes.PeerTelemetryLatestResponse{}, ErrInvalidPeer
	}
	if s == nil || s.Metrics == nil {
		return apitypes.PeerTelemetryLatestResponse{}, ErrMetricsStoreNil
	}
	fields, err := normalizeFields(fields)
	if err != nil {
		return apitypes.PeerTelemetryLatestResponse{}, err
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	values := make([]apitypes.PeerTelemetryValue, 0, len(fields))
	for _, field := range fields {
		point, ok, err := s.latestPoint(ctx, peer, field, now().UTC())
		if err != nil {
			return apitypes.PeerTelemetryLatestResponse{}, err
		}
		if !ok {
			continue
		}
		values = append(values, apitypes.PeerTelemetryValue{
			Field:            field,
			ObservedAtUnixMs: point.Timestamp.UnixMilli(),
			Value:            point.Value,
		})
	}
	return apitypes.PeerTelemetryLatestResponse{PeerPublicKey: peer.String(), Values: values}, nil
}

func (s *AdminService) QueryRange(ctx context.Context, peer giznet.PublicKey, field apitypes.PeerTelemetryField, start, end time.Time, step time.Duration, limit int, order apitypes.PeerTelemetryOrder) (apitypes.PeerTelemetryRangeResponse, error) {
	if peer.IsZero() {
		return apitypes.PeerTelemetryRangeResponse{}, ErrInvalidPeer
	}
	if s == nil || s.Metrics == nil {
		return apitypes.PeerTelemetryRangeResponse{}, ErrMetricsStoreNil
	}
	if _, err := fieldMetricName(field); err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, err
	}
	start, end, err := validateTimeRange(start, end)
	if err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, err
	}
	limit, err = normalizeLimit(limit)
	if err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, err
	}
	step, err = normalizeStep(start, end, step, limit)
	if err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, err
	}
	if order == "" {
		order = apitypes.PeerTelemetryOrderAsc
	}
	if !order.Valid() {
		return apitypes.PeerTelemetryRangeResponse{}, fmt.Errorf("%w: invalid order %q", ErrInvalidQuery, order)
	}
	selector, err := telemetrySelector(peer, field)
	if err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, err
	}
	series, err := s.Metrics.Range(ctx, metrics.RangeQuery{Selector: selector, Start: start, End: end, Step: step})
	if err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, fmt.Errorf("peertelemetry: query range %s: %w", field, err)
	}
	points := telemetryPointsFromSeries(series)
	sortTelemetryPoints(points)
	if order == apitypes.PeerTelemetryOrderDesc {
		slices.Reverse(points)
	}
	if len(points) > limit {
		points = points[:limit]
	}
	return apitypes.PeerTelemetryRangeResponse{
		PeerPublicKey: peer.String(),
		Field:         field,
		StartTimeMs:   start.UnixMilli(),
		EndTimeMs:     end.UnixMilli(),
		StepMs:        step.Milliseconds(),
		Points:        points,
	}, nil
}

func (s *AdminService) Aggregate(ctx context.Context, peer giznet.PublicKey, field apitypes.PeerTelemetryField, start, end time.Time, bucket time.Duration, aggregate apitypes.PeerTelemetryAggregate) (apitypes.PeerTelemetryAggregateResponse, error) {
	if peer.IsZero() {
		return apitypes.PeerTelemetryAggregateResponse{}, ErrInvalidPeer
	}
	if s == nil || s.Metrics == nil {
		return apitypes.PeerTelemetryAggregateResponse{}, ErrMetricsStoreNil
	}
	start, end, err := validateTimeRange(start, end)
	if err != nil {
		return apitypes.PeerTelemetryAggregateResponse{}, err
	}
	if bucket <= 0 {
		return apitypes.PeerTelemetryAggregateResponse{}, fmt.Errorf("%w: bucket_ms must be > 0", ErrInvalidQuery)
	}
	if bucket%time.Millisecond != 0 {
		return apitypes.PeerTelemetryAggregateResponse{}, fmt.Errorf("%w: bucket_ms must have millisecond precision", ErrInvalidQuery)
	}
	if start.Add(bucket).After(end) {
		return apitypes.PeerTelemetryAggregateResponse{}, fmt.Errorf("%w: bucket_ms must fit within the requested range", ErrInvalidQuery)
	}
	if countAggregateRangePoints(start, end, bucket) > maxAdminRangeLimit {
		return apitypes.PeerTelemetryAggregateResponse{}, fmt.Errorf("%w: requested bucket count exceeds %d", ErrInvalidQuery, maxAdminRangeLimit)
	}
	selector, err := telemetrySelector(peer, field)
	if err != nil {
		return apitypes.PeerTelemetryAggregateResponse{}, err
	}
	operation, err := metricsAggregation(aggregate)
	if err != nil {
		return apitypes.PeerTelemetryAggregateResponse{}, err
	}
	series, err := s.Metrics.Aggregate(ctx, metrics.AggregateQuery{Selector: selector, Start: start, End: end, Bucket: bucket, Operation: operation})
	if err != nil {
		return apitypes.PeerTelemetryAggregateResponse{}, fmt.Errorf("peertelemetry: aggregate %s %s: %w", field, aggregate, err)
	}
	points := aggregatePointsFromSeries(series, 0)
	return apitypes.PeerTelemetryAggregateResponse{
		PeerPublicKey: peer.String(),
		Field:         field,
		Aggregate:     aggregate,
		BucketMs:      bucket.Milliseconds(),
		Points:        points,
	}, nil
}

func (s *AdminService) latestPoint(ctx context.Context, peer giznet.PublicKey, field apitypes.PeerTelemetryField, at time.Time) (metrics.Point, bool, error) {
	selector, err := telemetrySelector(peer, field)
	if err != nil {
		return metrics.Point{}, false, err
	}
	series, err := s.Metrics.Latest(ctx, metrics.LatestQuery{Selector: selector, At: at, Lookback: defaultLatestLookback})
	if err != nil {
		return metrics.Point{}, false, fmt.Errorf("peertelemetry: query latest %s: %w", field, err)
	}
	point, ok := latestPointFromSeries(series)
	return point, ok, nil
}

func normalizeFields(fields []apitypes.PeerTelemetryField) ([]apitypes.PeerTelemetryField, error) {
	if len(fields) == 0 {
		return slices.Clone(allTelemetryFields), nil
	}
	seen := make(map[apitypes.PeerTelemetryField]struct{}, len(fields))
	out := make([]apitypes.PeerTelemetryField, 0, len(fields))
	for _, field := range fields {
		if _, err := fieldMetricName(field); err != nil {
			return nil, err
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out, nil
}

var allTelemetryFields = []apitypes.PeerTelemetryField{
	apitypes.PeerTelemetryFieldBatteryPercent,
	apitypes.PeerTelemetryFieldBatteryCharging,
	apitypes.PeerTelemetryFieldBatteryVoltageMv,
	apitypes.PeerTelemetryFieldGnssLatitude,
	apitypes.PeerTelemetryFieldGnssLongitude,
	apitypes.PeerTelemetryFieldGnssAltitudeM,
	apitypes.PeerTelemetryFieldGnssAccuracyM,
	apitypes.PeerTelemetryFieldNetworkRssiDbm,
	apitypes.PeerTelemetryFieldNetworkSignalLevel,
	apitypes.PeerTelemetryFieldNetworkConnected,
	apitypes.PeerTelemetryFieldSystemUptimeSeconds,
	apitypes.PeerTelemetryFieldSystemFreeMemoryBytes,
	apitypes.PeerTelemetryFieldSystemTemperatureC,
}

func fieldMetricName(field apitypes.PeerTelemetryField) (string, error) {
	switch field {
	case apitypes.PeerTelemetryFieldBatteryPercent:
		return MetricBatteryPercent, nil
	case apitypes.PeerTelemetryFieldBatteryCharging:
		return MetricBatteryCharging, nil
	case apitypes.PeerTelemetryFieldBatteryVoltageMv:
		return MetricBatteryVoltageMv, nil
	case apitypes.PeerTelemetryFieldGnssLatitude:
		return MetricGNSSLatitude, nil
	case apitypes.PeerTelemetryFieldGnssLongitude:
		return MetricGNSSLongitude, nil
	case apitypes.PeerTelemetryFieldGnssAltitudeM:
		return MetricGNSSAltitudeM, nil
	case apitypes.PeerTelemetryFieldGnssAccuracyM:
		return MetricGNSSAccuracyM, nil
	case apitypes.PeerTelemetryFieldNetworkRssiDbm:
		return MetricNetworkRSSIDbm, nil
	case apitypes.PeerTelemetryFieldNetworkSignalLevel:
		return MetricNetworkSignal, nil
	case apitypes.PeerTelemetryFieldNetworkConnected:
		return MetricNetworkConnected, nil
	case apitypes.PeerTelemetryFieldSystemUptimeSeconds:
		return MetricSystemUptime, nil
	case apitypes.PeerTelemetryFieldSystemFreeMemoryBytes:
		return MetricSystemFreeMemory, nil
	case apitypes.PeerTelemetryFieldSystemTemperatureC:
		return MetricSystemTemperature, nil
	default:
		return "", fmt.Errorf("%w: invalid field %q", ErrInvalidQuery, field)
	}
}

func telemetrySelector(peer giznet.PublicKey, field apitypes.PeerTelemetryField) (metrics.Selector, error) {
	name, err := fieldMetricName(field)
	if err != nil {
		return metrics.Selector{}, err
	}
	return metrics.Selector{
		Name: name,
		Matchers: []metrics.LabelMatcher{{
			Name:  "peer_id",
			Op:    metrics.MatchEqual,
			Value: peer.String(),
		}},
	}, nil
}

func metricsAggregation(value apitypes.PeerTelemetryAggregate) (metrics.Aggregation, error) {
	switch value {
	case apitypes.PeerTelemetryAggregateAvg:
		return metrics.AggregationAvg, nil
	case apitypes.PeerTelemetryAggregateMin:
		return metrics.AggregationMin, nil
	case apitypes.PeerTelemetryAggregateMax:
		return metrics.AggregationMax, nil
	case apitypes.PeerTelemetryAggregateSum:
		return metrics.AggregationSum, nil
	case apitypes.PeerTelemetryAggregateCount:
		return metrics.AggregationCount, nil
	case apitypes.PeerTelemetryAggregateLast:
		return metrics.AggregationLast, nil
	default:
		return "", fmt.Errorf("%w: invalid aggregate %q", ErrInvalidQuery, value)
	}
}

func validateTimeRange(start, end time.Time) (time.Time, time.Time, error) {
	if start.IsZero() {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: start_time_ms is required", ErrInvalidQuery)
	}
	if end.IsZero() {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: end_time_ms is required", ErrInvalidQuery)
	}
	start = start.UTC()
	end = end.UTC()
	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: end_time_ms must be greater than start_time_ms", ErrInvalidQuery)
	}
	return start, end, nil
}

func normalizeLimit(limit int) (int, error) {
	if limit <= 0 {
		return defaultAdminRangeLimit, nil
	}
	if limit > maxAdminRangeLimit {
		return 0, fmt.Errorf("%w: limit must be <= %d", ErrInvalidQuery, maxAdminRangeLimit)
	}
	return limit, nil
}

func normalizeStep(start, end time.Time, step time.Duration, limit int) (time.Duration, error) {
	if step < 0 {
		return 0, fmt.Errorf("%w: step_ms must be > 0", ErrInvalidQuery)
	}
	if step == 0 {
		if limit == 1 {
			step = end.Sub(start)
		} else {
			step = time.Duration(math.Ceil(float64(end.Sub(start)) / float64(limit-1)))
		}
		step = ceilDuration(step, time.Millisecond)
		step = max(step, minAdminStep)
	}
	if step <= 0 {
		return 0, fmt.Errorf("%w: step_ms must be > 0", ErrInvalidQuery)
	}
	if countSampleRangePoints(start, end, step) > limit {
		return 0, fmt.Errorf("%w: requested range and step exceed limit", ErrInvalidQuery)
	}
	return step, nil
}

func ceilDuration(d, unit time.Duration) time.Duration {
	if unit <= 0 || d%unit == 0 {
		return d
	}
	return (d/unit + 1) * unit
}

func countSampleRangePoints(start, end time.Time, step time.Duration) int {
	window := min(step, end.Sub(start))
	return countRangePoints(start.Add(window), end, step)
}

func countRangePoints(start, end time.Time, step time.Duration) int {
	if step <= 0 || end.Before(start) {
		return 0
	}
	return int(end.Sub(start)/step) + 1
}

func countAggregateRangePoints(start, end time.Time, bucket time.Duration) int {
	evalStart := start.Add(bucket)
	count := countRangePoints(evalStart, end, bucket)
	if end.Sub(start)%bucket != 0 {
		count++
	}
	return count
}

func latestPointFromSeries(series metrics.SeriesSet) (metrics.Point, bool) {
	var latest metrics.Point
	ok := false
	for _, item := range series {
		for _, point := range item.Points {
			if !ok || point.Timestamp.After(latest.Timestamp) {
				latest, ok = point, true
			}
		}
	}
	return latest, ok
}

func telemetryPointsFromSeries(series metrics.SeriesSet) []apitypes.PeerTelemetryPoint {
	points := make([]apitypes.PeerTelemetryPoint, 0)
	for _, item := range series {
		for _, point := range item.Points {
			points = append(points, apitypes.PeerTelemetryPoint{ObservedAtUnixMs: point.Timestamp.UnixMilli(), Value: point.Value})
		}
	}
	sortTelemetryPoints(points)
	return points
}

func sortTelemetryPoints(points []apitypes.PeerTelemetryPoint) {
	slices.SortFunc(points, func(a, b apitypes.PeerTelemetryPoint) int {
		if a.ObservedAtUnixMs < b.ObservedAtUnixMs {
			return -1
		}
		if a.ObservedAtUnixMs > b.ObservedAtUnixMs {
			return 1
		}
		return 0
	})
}

func aggregatePointsFromSeries(series metrics.SeriesSet, bucket time.Duration) []apitypes.PeerTelemetryAggregatePoint {
	points := make([]apitypes.PeerTelemetryAggregatePoint, 0)
	for _, item := range series {
		for _, point := range item.Points {
			points = append(points, apitypes.PeerTelemetryAggregatePoint{BucketStartTimeMs: point.Timestamp.Add(-bucket).UnixMilli(), Value: point.Value})
		}
	}
	slices.SortFunc(points, func(a, b apitypes.PeerTelemetryAggregatePoint) int {
		if a.BucketStartTimeMs < b.BucketStartTimeMs {
			return -1
		}
		if a.BucketStartTimeMs > b.BucketStartTimeMs {
			return 1
		}
		return 0
	})
	return points
}
