package peertelemetry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

type fakeAdminMetricsStore struct {
	query        metrics.Query
	queries      []metrics.Query
	rangeQuery   metrics.RangeQuery
	series       metrics.SeriesSet
	querySeries  metrics.SeriesSet
	rangeSeries  metrics.SeriesSet
	seriesByExpr map[string]metrics.SeriesSet
	err          error
}

func (s *fakeAdminMetricsStore) Append(context.Context, []metrics.Sample) error {
	return nil
}

func (s *fakeAdminMetricsStore) Query(ctx context.Context, query metrics.Query) (metrics.SeriesSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.query = query
	s.queries = append(s.queries, query)
	if s.seriesByExpr != nil {
		if series, ok := s.seriesByExpr[query.Expression]; ok {
			return series, s.err
		}
	}
	if s.querySeries != nil {
		return s.querySeries, s.err
	}
	return s.series, s.err
}

func (s *fakeAdminMetricsStore) QueryRange(ctx context.Context, query metrics.RangeQuery) (metrics.SeriesSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.rangeQuery = query
	if s.rangeSeries != nil {
		return s.rangeSeries, s.err
	}
	return s.series, s.err
}

func (s *fakeAdminMetricsStore) Close() error {
	return nil
}

func TestAdminLatestQueriesSelectedField(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	observedAt := time.Unix(1783403541, 123000000).UTC()
	valueExpr := `gizclaw_peer_battery_percent{peer_id="` + peer.String() + `"}`
	timestampExpr := `timestamp(` + valueExpr + `)`
	store := &fakeAdminMetricsStore{seriesByExpr: map[string]metrics.SeriesSet{
		valueExpr: {{
			Name: MetricBatteryPercent,
			Points: []metrics.Point{{
				Timestamp: observedAt.Add(time.Minute),
				Value:     87,
			}},
		}},
		timestampExpr: {{
			Name: "timestamp",
			Points: []metrics.Point{{
				Timestamp: observedAt.Add(time.Minute),
				Value:     float64(observedAt.UnixMilli()) / 1000,
			}},
		}},
	}}
	service := &AdminService{Metrics: store, Now: func() time.Time {
		return observedAt.Add(time.Minute)
	}}

	response, err := service.Latest(context.Background(), peer, []apitypes.PeerTelemetryField{apitypes.PeerTelemetryFieldBatteryPercent})
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if len(store.queries) != 2 || store.queries[0].Expression != valueExpr || store.queries[1].Expression != timestampExpr {
		t.Fatalf("queries = %#v, want value and timestamp expressions", store.queries)
	}
	if response.PeerPublicKey != peer.String() || len(response.Values) != 1 {
		t.Fatalf("response = %#v", response)
	}
	if got := response.Values[0]; got.Field != apitypes.PeerTelemetryFieldBatteryPercent || got.Value != 87 || got.ObservedAtUnixMs != observedAt.UnixMilli() {
		t.Fatalf("value = %#v", got)
	}
}

func TestAdminLatestFallsBackPastStoreLookback(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	observedAt := time.Unix(1783403541, 123000000).UTC()
	store := metrics.NewMemoryStore()
	if err := store.Append(context.Background(), []metrics.Sample{{
		Name:      MetricBatteryPercent,
		Labels:    map[string]string{"peer_id": peer.String()},
		Timestamp: observedAt,
		Value:     74,
	}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	service := &AdminService{Metrics: store, Now: func() time.Time {
		return observedAt.Add(time.Hour)
	}}

	response, err := service.Latest(context.Background(), peer, []apitypes.PeerTelemetryField{apitypes.PeerTelemetryFieldBatteryPercent})
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if len(response.Values) != 1 {
		t.Fatalf("Latest() values = %#v, want one fallback value", response.Values)
	}
	if got := response.Values[0]; got.Value != 74 || got.ObservedAtUnixMs != observedAt.UnixMilli() {
		t.Fatalf("fallback value = %#v", got)
	}
}

func TestAdminQueryRangeDerivesStepAndOrdersDesc(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	start := time.UnixMilli(1000).UTC()
	end := start.Add(4 * time.Minute)
	store := &fakeAdminMetricsStore{querySeries: metrics.SeriesSet{}, series: metrics.SeriesSet{{
		Name: MetricGNSSLatitude,
		Points: []metrics.Point{
			{Timestamp: start, Value: 1},
			{Timestamp: start.Add(2 * time.Minute), Value: 2},
			{Timestamp: end, Value: 3},
		},
	}}}
	service := &AdminService{Metrics: store}

	response, err := service.QueryRange(context.Background(), peer, apitypes.PeerTelemetryFieldGnssLatitude, start, end, 0, 3, apitypes.PeerTelemetryOrderDesc)
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}
	if store.rangeQuery.Step != 2*time.Minute {
		t.Fatalf("step = %s, want 2m", store.rangeQuery.Step)
	}
	wantExpr := `last_over_time(gizclaw_peer_gnss_latitude{peer_id="` + peer.String() + `"}[2m])`
	if store.rangeQuery.Expression != wantExpr {
		t.Fatalf("range expression = %q, want %q", store.rangeQuery.Expression, wantExpr)
	}
	if store.rangeQuery.Start != start.Add(2*time.Minute) {
		t.Fatalf("range start = %s, want %s", store.rangeQuery.Start, start.Add(2*time.Minute))
	}
	if len(response.Points) != 3 || response.Points[0].Value != 3 || response.Points[2].Value != 1 {
		t.Fatalf("points = %#v", response.Points)
	}
}

func TestAdminQueryRangeIncludesExactStartSample(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	start := time.Unix(1783400000, 0).UTC()
	end := start.Add(time.Minute)
	store := metrics.NewMemoryStore()
	if err := store.Append(context.Background(), []metrics.Sample{
		{Name: MetricBatteryPercent, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: start, Value: 70},
		{Name: MetricBatteryPercent, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: end, Value: 71},
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	service := &AdminService{Metrics: store}

	response, err := service.QueryRange(context.Background(), peer, apitypes.PeerTelemetryFieldBatteryPercent, start, end, time.Minute, 2, apitypes.PeerTelemetryOrderAsc)
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}
	if len(response.Points) != 2 || response.Points[0].Value != 70 || response.Points[1].Value != 71 {
		t.Fatalf("points = %#v, want exact start and window end samples", response.Points)
	}
}

func TestAdminQueryRangeUsesWindowedLastSample(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	start := time.Unix(1783400000, 0).UTC()
	end := start.Add(30 * time.Minute)
	store := metrics.NewMemoryStore()
	samples := []metrics.Sample{
		{Name: MetricBatteryPercent, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: start.Add(-time.Minute), Value: 99},
		{Name: MetricBatteryPercent, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: start.Add(6 * time.Minute), Value: 71},
		{Name: MetricBatteryPercent, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: start.Add(16 * time.Minute), Value: 72},
		{Name: MetricBatteryPercent, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: start.Add(26 * time.Minute), Value: 73},
	}
	if err := store.Append(context.Background(), samples); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	service := &AdminService{Metrics: store}

	response, err := service.QueryRange(context.Background(), peer, apitypes.PeerTelemetryFieldBatteryPercent, start, end, 10*time.Minute, 10, apitypes.PeerTelemetryOrderAsc)
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}
	if len(response.Points) != 3 {
		t.Fatalf("points = %#v, want 3 sparse samples", response.Points)
	}
	for i, want := range []float64{71, 72, 73} {
		if response.Points[i].Value != want {
			t.Fatalf("point[%d] = %#v, want value %v", i, response.Points[i], want)
		}
	}
}

func TestAdminQueryRangeAllowsOnePointAutoStep(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	start := time.Unix(1783400000, 0).UTC()
	end := start.Add(10 * time.Minute)
	store := metrics.NewMemoryStore()
	if err := store.Append(context.Background(), []metrics.Sample{
		{Name: MetricBatteryPercent, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: end, Value: 77},
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	service := &AdminService{Metrics: store}

	response, err := service.QueryRange(context.Background(), peer, apitypes.PeerTelemetryFieldBatteryPercent, start, end, 0, 1, apitypes.PeerTelemetryOrderAsc)
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}
	if len(response.Points) != 1 || response.Points[0].Value != 77 {
		t.Fatalf("points = %#v, want one point", response.Points)
	}
}

func TestAdminQueryRangeRoundsAutoStepToMilliseconds(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	start := time.Unix(1783400000, 0).UTC()
	end := start.Add(time.Hour)
	store := &fakeAdminMetricsStore{querySeries: metrics.SeriesSet{}, rangeSeries: metrics.SeriesSet{}}
	service := &AdminService{Metrics: store}

	_, err := service.QueryRange(context.Background(), peer, apitypes.PeerTelemetryFieldBatteryPercent, start, end, 0, 0, apitypes.PeerTelemetryOrderAsc)
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}
	if store.rangeQuery.Step%time.Millisecond != 0 {
		t.Fatalf("step = %s, want millisecond aligned", store.rangeQuery.Step)
	}
	wantExpr := `last_over_time(gizclaw_peer_battery_percent{peer_id="` + peer.String() + `"}[15063ms])`
	if store.rangeQuery.Expression != wantExpr {
		t.Fatalf("range expression = %q, want %q", store.rangeQuery.Expression, wantExpr)
	}
}

func TestAdminAggregateLastBuildsPromQLOverTime(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	start := time.UnixMilli(1000).UTC()
	end := start.Add(10 * time.Minute)
	store := &fakeAdminMetricsStore{series: metrics.SeriesSet{{
		Name: "last_over_time",
		Points: []metrics.Point{{
			Timestamp: start.Add(time.Minute),
			Value:     42,
		}},
	}}}
	service := &AdminService{Metrics: store}

	response, err := service.Aggregate(context.Background(), peer, apitypes.PeerTelemetryFieldSystemTemperatureC, start, end, time.Minute, apitypes.PeerTelemetryAggregateLast)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}
	wantExpr := `last_over_time(gizclaw_peer_system_temperature_c{peer_id="` + peer.String() + `"}[1m])`
	if store.rangeQuery.Expression != wantExpr {
		t.Fatalf("aggregate expression = %q, want %q", store.rangeQuery.Expression, wantExpr)
	}
	if store.rangeQuery.Step != time.Minute {
		t.Fatalf("aggregate step = %s, want 1m", store.rangeQuery.Step)
	}
	if store.rangeQuery.Start != start.Add(time.Minute) {
		t.Fatalf("aggregate start = %s, want %s", store.rangeQuery.Start, start.Add(time.Minute))
	}
	if response.Aggregate != apitypes.PeerTelemetryAggregateLast || response.BucketMs != 60000 || len(response.Points) != 1 {
		t.Fatalf("response = %#v", response)
	}
	if response.Points[0].BucketStartTimeMs != start.UnixMilli() {
		t.Fatalf("bucket_start_time_ms = %d, want %d", response.Points[0].BucketStartTimeMs, start.UnixMilli())
	}
}

func TestAdminQueryValidation(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	service := &AdminService{Metrics: &fakeAdminMetricsStore{}}
	start := time.Unix(1, 0).UTC()
	end := start.Add(10 * time.Microsecond)
	if _, err := service.QueryRange(context.Background(), peer, "bad", start, end, time.Second, 10, apitypes.PeerTelemetryOrderAsc); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid field error = %v, want %v", err, ErrInvalidQuery)
	}
	if _, err := service.QueryRange(context.Background(), peer, apitypes.PeerTelemetryFieldBatteryPercent, start, end, time.Second, maxAdminRangeLimit+1, apitypes.PeerTelemetryOrderAsc); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid limit error = %v, want %v", err, ErrInvalidQuery)
	}
	if _, err := service.QueryRange(context.Background(), peer, apitypes.PeerTelemetryFieldBatteryPercent, start, end, time.Second, 10, "sideways"); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid order error = %v, want %v", err, ErrInvalidQuery)
	}
}

func TestAdminAggregateRejectsUnsupportedDurationPrecision(t *testing.T) {
	t.Parallel()

	peer := adminTestPeer()
	service := &AdminService{Metrics: &fakeAdminMetricsStore{}}
	start := time.Unix(1, 0).UTC()
	end := start.Add(10 * time.Microsecond)
	_, err := service.Aggregate(context.Background(), peer, apitypes.PeerTelemetryFieldBatteryPercent, start, end, time.Microsecond, apitypes.PeerTelemetryAggregateAvg)
	if !errors.Is(err, ErrInvalidQuery) || !strings.Contains(err.Error(), "millisecond") {
		t.Fatalf("Aggregate() error = %v, want millisecond invalid query", err)
	}
}

func adminTestPeer() giznet.PublicKey {
	var peer giznet.PublicKey
	for i := range peer {
		peer[i] = byte(i + 1)
	}
	return peer
}
