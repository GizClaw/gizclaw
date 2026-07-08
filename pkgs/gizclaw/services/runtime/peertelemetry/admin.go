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
	exactStartLookback     = time.Millisecond
	latestTimestampStep    = time.Minute
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
	window := step
	if window > end.Sub(start) {
		window = end.Sub(start)
	}
	expr, err := rangeSampleExpression(peer, field, window)
	if err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, err
	}
	evalStart := start.Add(window)
	if evalStart.After(end) {
		evalStart = end
	}
	series, err := s.Metrics.QueryRange(ctx, metrics.RangeQuery{Expression: expr, Start: evalStart, End: end, Step: step})
	if err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, fmt.Errorf("peertelemetry: query range %s: %w", field, err)
	}
	points := telemetryPointsFromSeries(series)
	if point, ok, err := s.exactPointAt(ctx, peer, field, start); err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, err
	} else if ok {
		points = appendTelemetryPoint(points, point)
	}
	if point, ok, err := s.rangePointAt(ctx, expr, field, end); err != nil {
		return apitypes.PeerTelemetryRangeResponse{}, err
	} else if ok {
		point.Timestamp = end
		points = appendTelemetryPoint(points, point)
	}
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
	evalStart := start.Add(bucket)
	if evalStart.After(end) {
		return apitypes.PeerTelemetryAggregateResponse{}, fmt.Errorf("%w: bucket_ms must fit within the requested range", ErrInvalidQuery)
	}
	if countAggregateRangePoints(start, end, bucket) > maxAdminRangeLimit {
		return apitypes.PeerTelemetryAggregateResponse{}, fmt.Errorf("%w: requested bucket count exceeds %d", ErrInvalidQuery, maxAdminRangeLimit)
	}
	expr, err := aggregateExpression(peer, field, aggregate, bucket)
	if err != nil {
		return apitypes.PeerTelemetryAggregateResponse{}, err
	}
	series, err := s.Metrics.QueryRange(ctx, metrics.RangeQuery{Expression: expr, Start: evalStart, End: end, Step: bucket})
	if err != nil {
		return apitypes.PeerTelemetryAggregateResponse{}, fmt.Errorf("peertelemetry: aggregate %s %s: %w", field, aggregate, err)
	}
	points := aggregatePointsFromSeries(series, bucket)
	points, err = s.includeAggregateTail(ctx, peer, field, start, end, bucket, aggregate, points)
	if err != nil {
		return apitypes.PeerTelemetryAggregateResponse{}, err
	}
	points, err = s.includeAggregateStartBoundary(ctx, peer, field, start, bucket, aggregate, points)
	if err != nil {
		return apitypes.PeerTelemetryAggregateResponse{}, err
	}
	return apitypes.PeerTelemetryAggregateResponse{
		PeerPublicKey: peer.String(),
		Field:         field,
		Aggregate:     aggregate,
		BucketMs:      bucket.Milliseconds(),
		Points:        points,
	}, nil
}

func (s *AdminService) latestPoint(ctx context.Context, peer giznet.PublicKey, field apitypes.PeerTelemetryField, at time.Time) (metrics.Point, bool, error) {
	expr, err := selectorExpression(peer, field)
	if err != nil {
		return metrics.Point{}, false, err
	}
	series, err := s.Metrics.Query(ctx, metrics.Query{Expression: expr, Time: at})
	if err != nil {
		return metrics.Point{}, false, fmt.Errorf("peertelemetry: query latest %s: %w", field, err)
	}
	point, ok := latestPointFromSeries(series)
	if ok {
		timestampExpr, err := timestampExpression(peer, field)
		if err != nil {
			return metrics.Point{}, false, err
		}
		timestampSeries, err := s.Metrics.Query(ctx, metrics.Query{Expression: timestampExpr, Time: at})
		if err != nil {
			return metrics.Point{}, false, fmt.Errorf("peertelemetry: query latest timestamp %s: %w", field, err)
		}
		if timestampPoint, ok := latestPointFromSeries(timestampSeries); ok {
			point.Timestamp = time.UnixMilli(int64(math.Round(timestampPoint.Value * 1000))).UTC()
			return point, true, nil
		}
	}
	fallbackExpr, err := latestExpression(peer, field, defaultLatestLookback)
	if err != nil {
		return metrics.Point{}, false, err
	}
	series, err = s.Metrics.Query(ctx, metrics.Query{Expression: fallbackExpr, Time: at})
	if err != nil {
		return metrics.Point{}, false, fmt.Errorf("peertelemetry: query latest fallback %s: %w", field, err)
	}
	point, ok = latestPointFromSeries(series)
	if !ok {
		return metrics.Point{}, false, nil
	}
	fallbackTimestampExpr, err := latestTimestampExpression(peer, field, defaultLatestLookback, latestTimestampStep)
	if err != nil {
		return metrics.Point{}, false, err
	}
	timestampSeries, err := s.Metrics.Query(ctx, metrics.Query{Expression: fallbackTimestampExpr, Time: at})
	if err != nil {
		return metrics.Point{}, false, fmt.Errorf("peertelemetry: query latest fallback timestamp %s: %w", field, err)
	}
	timestampPoint, ok := latestPointFromSeries(timestampSeries)
	if !ok {
		return metrics.Point{}, false, nil
	}
	point.Timestamp = time.UnixMilli(int64(math.Round(timestampPoint.Value * 1000))).UTC()
	return point, true, nil
}

func (s *AdminService) exactPointAt(ctx context.Context, peer giznet.PublicKey, field apitypes.PeerTelemetryField, at time.Time) (metrics.Point, bool, error) {
	expr, err := rawRangeExpression(peer, field, exactStartLookback)
	if err != nil {
		return metrics.Point{}, false, err
	}
	series, err := s.Metrics.Query(ctx, metrics.Query{Expression: expr, Time: at})
	if err != nil {
		return metrics.Point{}, false, fmt.Errorf("peertelemetry: query exact range start %s: %w", field, err)
	}
	point, ok := latestPointFromSeries(series)
	if !ok || !point.Timestamp.Equal(at) {
		return metrics.Point{}, false, nil
	}
	return point, true, nil
}

func (s *AdminService) rangePointAt(ctx context.Context, expr string, field apitypes.PeerTelemetryField, at time.Time) (metrics.Point, bool, error) {
	series, err := s.Metrics.Query(ctx, metrics.Query{Expression: expr, Time: at})
	if err != nil {
		return metrics.Point{}, false, fmt.Errorf("peertelemetry: query range tail %s: %w", field, err)
	}
	point, ok := latestPointFromSeries(series)
	return point, ok, nil
}

func (s *AdminService) includeAggregateTail(
	ctx context.Context,
	peer giznet.PublicKey,
	field apitypes.PeerTelemetryField,
	start time.Time,
	end time.Time,
	bucket time.Duration,
	aggregate apitypes.PeerTelemetryAggregate,
	points []apitypes.PeerTelemetryAggregatePoint,
) ([]apitypes.PeerTelemetryAggregatePoint, error) {
	elapsed := end.Sub(start)
	tail := elapsed % bucket
	if tail == 0 {
		return points, nil
	}
	bucketStart := end.Add(-tail)
	expr, err := aggregateExpression(peer, field, aggregate, tail)
	if err != nil {
		return nil, err
	}
	series, err := s.Metrics.Query(ctx, metrics.Query{Expression: expr, Time: end})
	if err != nil {
		return nil, fmt.Errorf("peertelemetry: query aggregate tail %s %s: %w", field, aggregate, err)
	}
	point, ok := latestPointFromSeries(series)
	if !ok {
		return points, nil
	}
	return upsertAggregatePoint(points, apitypes.PeerTelemetryAggregatePoint{
		BucketStartTimeMs: bucketStart.UnixMilli(),
		Value:             point.Value,
	}), nil
}

func (s *AdminService) includeAggregateStartBoundary(
	ctx context.Context,
	peer giznet.PublicKey,
	field apitypes.PeerTelemetryField,
	start time.Time,
	bucket time.Duration,
	aggregate apitypes.PeerTelemetryAggregate,
	points []apitypes.PeerTelemetryAggregatePoint,
) ([]apitypes.PeerTelemetryAggregatePoint, error) {
	startPoint, ok, err := s.exactPointAt(ctx, peer, field, start)
	if err != nil {
		return nil, err
	}
	if !ok {
		return points, nil
	}
	expr, err := rawRangeExpression(peer, field, bucket)
	if err != nil {
		return nil, err
	}
	series, err := s.Metrics.Query(ctx, metrics.Query{Expression: expr, Time: start.Add(bucket)})
	if err != nil {
		return nil, fmt.Errorf("peertelemetry: query aggregate start bucket %s: %w", field, err)
	}
	bucketPoints := []metrics.Point{startPoint}
	for _, item := range series {
		for _, point := range item.Points {
			bucketPoints = appendMetricPoint(bucketPoints, point)
		}
	}
	value, ok := aggregateMetricPoints(bucketPoints, aggregate)
	if !ok {
		return points, nil
	}
	return upsertAggregatePoint(points, apitypes.PeerTelemetryAggregatePoint{
		BucketStartTimeMs: start.UnixMilli(),
		Value:             value,
	}), nil
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

func selectorExpression(peer giznet.PublicKey, field apitypes.PeerTelemetryField) (string, error) {
	name, err := fieldMetricName(field)
	if err != nil {
		return "", err
	}
	return metrics.Selector{
		Name: name,
		Matchers: []metrics.LabelMatcher{{
			Name:  "peer_id",
			Op:    metrics.MatchEqual,
			Value: peer.String(),
		}},
	}.Expression()
}

func latestExpression(peer giznet.PublicKey, field apitypes.PeerTelemetryField, lookback time.Duration) (string, error) {
	selector, err := selectorExpression(peer, field)
	if err != nil {
		return "", err
	}
	promDuration, err := promQLDuration(lookback)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("last_over_time(%s[%s])", selector, promDuration), nil
}

func timestampExpression(peer giznet.PublicKey, field apitypes.PeerTelemetryField) (string, error) {
	selector, err := selectorExpression(peer, field)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("timestamp(%s)", selector), nil
}

func rawRangeExpression(peer giznet.PublicKey, field apitypes.PeerTelemetryField, lookback time.Duration) (string, error) {
	selector, err := selectorExpression(peer, field)
	if err != nil {
		return "", err
	}
	promDuration, err := promQLDuration(lookback)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s[%s]", selector, promDuration), nil
}

func latestTimestampExpression(peer giznet.PublicKey, field apitypes.PeerTelemetryField, lookback, resolution time.Duration) (string, error) {
	selector, err := selectorExpression(peer, field)
	if err != nil {
		return "", err
	}
	promLookback, err := promQLDuration(lookback)
	if err != nil {
		return "", err
	}
	promResolution, err := promQLDuration(resolution)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("last_over_time(timestamp(%s)[%s:%s])", selector, promLookback, promResolution), nil
}

func rangeSampleExpression(peer giznet.PublicKey, field apitypes.PeerTelemetryField, window time.Duration) (string, error) {
	return latestExpression(peer, field, window)
}

func aggregateExpression(peer giznet.PublicKey, field apitypes.PeerTelemetryField, aggregate apitypes.PeerTelemetryAggregate, bucket time.Duration) (string, error) {
	if !aggregate.Valid() {
		return "", fmt.Errorf("%w: invalid aggregate %q", ErrInvalidQuery, aggregate)
	}
	selector, err := selectorExpression(peer, field)
	if err != nil {
		return "", err
	}
	promDuration, err := promQLDuration(bucket)
	if err != nil {
		return "", err
	}
	operator := ""
	switch aggregate {
	case apitypes.PeerTelemetryAggregateAvg:
		operator = "avg_over_time"
	case apitypes.PeerTelemetryAggregateMin:
		operator = "min_over_time"
	case apitypes.PeerTelemetryAggregateMax:
		operator = "max_over_time"
	case apitypes.PeerTelemetryAggregateSum:
		operator = "sum_over_time"
	case apitypes.PeerTelemetryAggregateCount:
		operator = "count_over_time"
	case apitypes.PeerTelemetryAggregateLast:
		operator = "last_over_time"
	}
	return fmt.Sprintf("%s(%s[%s])", operator, selector, promDuration), nil
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
		if step < minAdminStep {
			step = minAdminStep
		}
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
	window := step
	if window > end.Sub(start) {
		window = end.Sub(start)
	}
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

func promQLDuration(d time.Duration) (string, error) {
	if d <= 0 {
		return "", fmt.Errorf("%w: duration must be > 0", ErrInvalidQuery)
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", d/time.Hour), nil
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", d/time.Minute), nil
	}
	if d%time.Second == 0 {
		return fmt.Sprintf("%ds", d/time.Second), nil
	}
	if d%time.Millisecond == 0 {
		return fmt.Sprintf("%dms", d/time.Millisecond), nil
	}
	return "", fmt.Errorf("%w: duration must have millisecond precision", ErrInvalidQuery)
}

func latestPointFromSeries(series metrics.SeriesSet) (metrics.Point, bool) {
	var latest metrics.Point
	ok := false
	for _, item := range series {
		for _, point := range item.Points {
			if !ok || point.Timestamp.After(latest.Timestamp) {
				latest = point
				ok = true
			}
		}
	}
	return latest, ok
}

func telemetryPointsFromSeries(series metrics.SeriesSet) []apitypes.PeerTelemetryPoint {
	points := make([]apitypes.PeerTelemetryPoint, 0)
	for _, item := range series {
		for _, point := range item.Points {
			points = append(points, apitypes.PeerTelemetryPoint{
				ObservedAtUnixMs: point.Timestamp.UnixMilli(),
				Value:            point.Value,
			})
		}
	}
	sortTelemetryPoints(points)
	return points
}

func appendTelemetryPoint(points []apitypes.PeerTelemetryPoint, point metrics.Point) []apitypes.PeerTelemetryPoint {
	unixMs := point.Timestamp.UnixMilli()
	for index := range points {
		if points[index].ObservedAtUnixMs == unixMs {
			points[index].Value = point.Value
			return points
		}
	}
	return append(points, apitypes.PeerTelemetryPoint{ObservedAtUnixMs: unixMs, Value: point.Value})
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
			points = append(points, apitypes.PeerTelemetryAggregatePoint{
				BucketStartTimeMs: point.Timestamp.Add(-bucket).UnixMilli(),
				Value:             point.Value,
			})
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

func aggregateMetricPoints(points []metrics.Point, aggregate apitypes.PeerTelemetryAggregate) (float64, bool) {
	if len(points) == 0 {
		return 0, false
	}
	switch aggregate {
	case apitypes.PeerTelemetryAggregateAvg:
		sum := 0.0
		for _, point := range points {
			sum += point.Value
		}
		return sum / float64(len(points)), true
	case apitypes.PeerTelemetryAggregateMin:
		value := points[0].Value
		for _, point := range points[1:] {
			if point.Value < value {
				value = point.Value
			}
		}
		return value, true
	case apitypes.PeerTelemetryAggregateMax:
		value := points[0].Value
		for _, point := range points[1:] {
			if point.Value > value {
				value = point.Value
			}
		}
		return value, true
	case apitypes.PeerTelemetryAggregateSum:
		sum := 0.0
		for _, point := range points {
			sum += point.Value
		}
		return sum, true
	case apitypes.PeerTelemetryAggregateCount:
		return float64(len(points)), true
	case apitypes.PeerTelemetryAggregateLast:
		latest := points[0]
		for _, point := range points[1:] {
			if point.Timestamp.After(latest.Timestamp) {
				latest = point
			}
		}
		return latest.Value, true
	default:
		return 0, false
	}
}

func appendMetricPoint(points []metrics.Point, point metrics.Point) []metrics.Point {
	for index := range points {
		if points[index].Timestamp.Equal(point.Timestamp) {
			points[index] = point
			return points
		}
	}
	return append(points, point)
}

func upsertAggregatePoint(points []apitypes.PeerTelemetryAggregatePoint, point apitypes.PeerTelemetryAggregatePoint) []apitypes.PeerTelemetryAggregatePoint {
	for index := range points {
		if points[index].BucketStartTimeMs == point.BucketStartTimeMs {
			points[index].Value = point.Value
			return points
		}
	}
	points = append(points, point)
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
