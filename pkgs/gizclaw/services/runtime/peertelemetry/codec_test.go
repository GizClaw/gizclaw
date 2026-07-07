package peertelemetry

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"google.golang.org/protobuf/proto"
)

func TestDecodeRejectsInvalidFrames(t *testing.T) {
	if _, err := Decode(nil); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("Decode(nil) error = %v, want %v", err, ErrInvalidFrame)
	}
	if _, err := Decode([]byte{0xff}); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("Decode(invalid) error = %v, want %v", err, ErrInvalidFrame)
	}
	payload := marshalFrame(t, &telemetrypb.TelemetryFrame{})
	if _, err := Decode(payload); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("Decode(no observations) error = %v, want %v", err, ErrInvalidFrame)
	}
	payload = marshalFrame(t, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{}},
	})
	if _, err := Decode(payload); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("Decode(no body) error = %v, want %v", err, ErrInvalidFrame)
	}
}

func TestServiceReportPacketAppendsMetricsAndSyncsFixedStatus(t *testing.T) {
	peer := testPublicKey(t)
	base := time.Unix(200, 123_000_000).UTC()
	percent := 82.4
	charging := true
	voltage := 3700.0
	altitude := 12.5
	accuracy := 2.25
	rssi := -71.0
	connected := true
	temp := 31.5
	firmware := "v1.2.3"
	payload := marshalFrame(t, &telemetrypb.TelemetryFrame{
		Sequence:         7,
		ObservedAtUnixMs: base.UnixMilli(),
		Observations: []*telemetrypb.Observation{
			{
				Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
					Percent:   &percent,
					Charging:  &charging,
					VoltageMv: &voltage,
				}},
			},
			{
				ObservedAtDeltaMs: 10,
				Body: &telemetrypb.Observation_Gnss{Gnss: &telemetrypb.GnssObservation{
					Latitude:  31.2,
					Longitude: 121.4,
					AltitudeM: &altitude,
					AccuracyM: &accuracy,
				}},
			},
			{
				ObservedAtDeltaMs: 20,
				Body: &telemetrypb.Observation_Network{Network: &telemetrypb.NetworkObservation{
					RssiDbm:   &rssi,
					Connected: &connected,
				}},
			},
			{
				ObservedAtDeltaMs: 30,
				Body: &telemetrypb.Observation_System{System: &telemetrypb.SystemObservation{
					TemperatureC:    &temp,
					FirmwareVersion: &firmware,
				}},
			},
		},
	})
	metricsStore := &fakeMetricsStore{}
	statusStore := &fakeStatusStore{
		status: apitypes.PeerStatus{
			Volume:  intPtr(33),
			Details: &map[string]any{"keep": "yes"},
			Labels:  &map[string]string{"mode": "test"},
		},
	}
	service := &Service{
		Metrics: metricsStore,
		Status:  StatusSync{Store: statusStore},
	}
	if err := service.ReportPacket(context.Background(), peer, payload); err != nil {
		t.Fatalf("ReportPacket() error = %v", err)
	}

	assertSample(t, metricsStore.samples, MetricBatteryPercent, base, 82.4)
	assertSample(t, metricsStore.samples, MetricBatteryCharging, base, 1)
	assertSample(t, metricsStore.samples, MetricBatteryVoltageMv, base, 3700)
	assertSample(t, metricsStore.samples, MetricGNSSLatitude, base.Add(10*time.Millisecond), 31.2)
	assertSample(t, metricsStore.samples, MetricGNSSLongitude, base.Add(10*time.Millisecond), 121.4)
	assertSample(t, metricsStore.samples, MetricGNSSAltitudeM, base.Add(10*time.Millisecond), 12.5)
	assertSample(t, metricsStore.samples, MetricGNSSAccuracyM, base.Add(10*time.Millisecond), 2.25)
	assertSample(t, metricsStore.samples, MetricNetworkRSSIDbm, base.Add(20*time.Millisecond), -71)
	assertSample(t, metricsStore.samples, MetricNetworkConnected, base.Add(20*time.Millisecond), 1)
	assertSample(t, metricsStore.samples, MetricSystemTemperature, base.Add(30*time.Millisecond), 31.5)
	if len(metricsStore.samples) != 10 {
		t.Fatalf("samples length = %d, want 10: %+v", len(metricsStore.samples), metricsStore.samples)
	}
	for _, sample := range metricsStore.samples {
		if got := sample.Labels["peer_id"]; got != peer.String() {
			t.Fatalf("%s peer_id = %q, want %q", sample.Name, got, peer.String())
		}
		if _, ok := sample.Labels["firmware_version"]; ok {
			t.Fatalf("%s has high-cardinality firmware_version label", sample.Name)
		}
	}

	if statusStore.puts != 1 {
		t.Fatalf("status puts = %d, want 1", statusStore.puts)
	}
	status := statusStore.status
	if status.BatteryPercent == nil || *status.BatteryPercent != 82 {
		t.Fatalf("BatteryPercent = %#v, want 82", status.BatteryPercent)
	}
	if status.Charging == nil || !*status.Charging {
		t.Fatalf("Charging = %#v, want true", status.Charging)
	}
	if status.ReportedAt == nil || !status.ReportedAt.Equal(base) {
		t.Fatalf("ReportedAt = %#v, want %s", status.ReportedAt, base)
	}
	if status.Volume == nil || *status.Volume != 33 {
		t.Fatalf("Volume = %#v, want preserved 33", status.Volume)
	}
	if status.Details == nil || (*status.Details)["keep"] != "yes" {
		t.Fatalf("Details = %#v, want preserved map", status.Details)
	}
	if status.Labels == nil || (*status.Labels)["mode"] != "test" {
		t.Fatalf("Labels = %#v, want preserved map", status.Labels)
	}
}

func TestServiceReportWithMemoryMetricsStoreQueriesTelemetrySamples(t *testing.T) {
	peer := testPublicKey(t)
	base := time.Unix(800, 0).UTC()
	percent := 70.0
	nextPercent := 71.0
	latitude := 31.2
	longitude := 121.4
	store := metrics.NewMemoryStore()
	service := &Service{
		Metrics: store,
		Status:  StatusSync{Store: &fakeStatusStore{}},
	}
	if err := service.Report(context.Background(), peer, &telemetrypb.TelemetryFrame{
		ObservedAtUnixMs: base.UnixMilli(),
		Observations: []*telemetrypb.Observation{
			{Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &percent}}},
			{ObservedAtDeltaMs: 1000, Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &nextPercent}}},
			{ObservedAtDeltaMs: 2000, Body: &telemetrypb.Observation_Gnss{Gnss: &telemetrypb.GnssObservation{
				Latitude:  latitude,
				Longitude: longitude,
			}}},
		},
	}); err != nil {
		t.Fatalf("Report() error = %v", err)
	}

	batteryQuery, err := (metrics.Selector{
		Name:     MetricBatteryPercent,
		Matchers: []metrics.LabelMatcher{{Name: "peer_id", Op: metrics.MatchEqual, Value: peer.String()}},
	}).Expression()
	if err != nil {
		t.Fatalf("battery selector: %v", err)
	}
	got, err := store.Query(context.Background(), metrics.Query{
		Expression: batteryQuery,
		Time:       base.Add(1500 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("Query battery: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 1 || got[0].Points[0].Value != 71 {
		t.Fatalf("battery query = %+v, want latest 71", got)
	}
	got, err = store.QueryRange(context.Background(), metrics.RangeQuery{
		Expression: batteryQuery,
		Start:      base,
		End:        base.Add(time.Second),
		Step:       time.Second,
	})
	if err != nil {
		t.Fatalf("QueryRange battery: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("battery range = %+v, want 2 points", got)
	}

	gnssQuery, err := (metrics.Selector{
		Name:     MetricGNSSLatitude,
		Matchers: []metrics.LabelMatcher{{Name: "peer_id", Op: metrics.MatchEqual, Value: peer.String()}},
	}).Expression()
	if err != nil {
		t.Fatalf("gnss selector: %v", err)
	}
	got, err = store.Query(context.Background(), metrics.Query{Expression: gnssQuery})
	if err != nil {
		t.Fatalf("Query gnss: %v", err)
	}
	if len(got) != 1 || got[0].Points[0].Value != 31.2 {
		t.Fatalf("gnss query = %+v, want latitude 31.2", got)
	}
}

func TestServiceReportBoundsMetricsAppendContext(t *testing.T) {
	peer := testPublicKey(t)
	percent := 73.0
	metricsStore := &fakeMetricsStore{}
	service := &Service{
		Metrics:              metricsStore,
		Status:               StatusSync{Store: &fakeStatusStore{}},
		MetricsAppendTimeout: 50 * time.Millisecond,
	}
	if err := service.Report(context.Background(), peer, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &percent}},
		}},
	}); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if !metricsStore.deadlineSet {
		t.Fatal("metrics Append context has no deadline")
	}
	if got := time.Until(metricsStore.deadline); got <= 0 || got > time.Second {
		t.Fatalf("metrics Append deadline remaining = %s, want a short positive deadline", got)
	}
}

func TestMapFrameStatusUsesLatestBatteryObservationTime(t *testing.T) {
	peer := testPublicKey(t)
	base := time.Unix(900, 0).UTC()
	olderPercent := 10.0
	newerPercent := 80.0
	olderCharging := false
	newerCharging := true
	_, patch, err := MapFrame(peer, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{
			{
				ObservedAtDeltaMs: 1000,
				Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
					Percent:  &newerPercent,
					Charging: &newerCharging,
				}},
			},
			{
				ObservedAtDeltaMs: 100,
				Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
					Percent:  &olderPercent,
					Charging: &olderCharging,
				}},
			},
		},
	}, base)
	if err != nil {
		t.Fatalf("MapFrame() error = %v", err)
	}
	if patch.ReportedAt != base.Add(time.Second) {
		t.Fatalf("ReportedAt = %s, want latest observation time", patch.ReportedAt)
	}
	if patch.BatteryPercent == nil || *patch.BatteryPercent != 80 {
		t.Fatalf("BatteryPercent = %#v, want latest value 80", patch.BatteryPercent)
	}
	if patch.Charging == nil || !*patch.Charging {
		t.Fatalf("Charging = %#v, want latest value true", patch.Charging)
	}
}

func TestMapFrameStatusPreservesMissingFieldsFromOlderObservations(t *testing.T) {
	peer := testPublicKey(t)
	base := time.Unix(901, 0).UTC()
	olderPercent := 10.0
	newerCharging := true
	_, patch, err := MapFrame(peer, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{
			{
				ObservedAtDeltaMs: 1000,
				Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
					Charging: &newerCharging,
				}},
			},
			{
				ObservedAtDeltaMs: 100,
				Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
					Percent: &olderPercent,
				}},
			},
		},
	}, base)
	if err != nil {
		t.Fatalf("MapFrame() error = %v", err)
	}
	if patch.ReportedAt != base.Add(time.Second) {
		t.Fatalf("ReportedAt = %s, want latest observation time", patch.ReportedAt)
	}
	if patch.BatteryPercent == nil || *patch.BatteryPercent != 10 {
		t.Fatalf("BatteryPercent = %#v, want older missing field value 10", patch.BatteryPercent)
	}
	if patch.Charging == nil || !*patch.Charging {
		t.Fatalf("Charging = %#v, want latest value true", patch.Charging)
	}
}

func TestMapFrameRejectsInvalidNumbers(t *testing.T) {
	peer := testPublicKey(t)
	percent := math.NaN()
	_, _, err := MapFrame(peer, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &percent}},
		}},
	}, time.Unix(1, 0).UTC())
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("MapFrame(NaN) error = %v, want %v", err, ErrInvalidFrame)
	}
}

func TestReportUsesNowWhenObservedAtMissing(t *testing.T) {
	peer := testPublicKey(t)
	now := time.Unix(500, 0).UTC()
	percent := 10.0
	service := &Service{
		Metrics: &fakeMetricsStore{},
		Status:  StatusSync{Store: &fakeStatusStore{}},
		Now:     func() time.Time { return now },
	}
	if err := service.Report(context.Background(), peer, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &percent}},
		}},
	}); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	sample := service.Metrics.(*fakeMetricsStore).samples[0]
	if !sample.Timestamp.Equal(now) {
		t.Fatalf("sample timestamp = %s, want %s", sample.Timestamp, now)
	}
}

func TestServiceReportRejectsInvalidInputsAndMissingDependencies(t *testing.T) {
	peer := testPublicKey(t)
	percent := 25.0
	frame := &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &percent}},
		}},
	}
	if err := (&Service{Metrics: &fakeMetricsStore{}}).Report(context.Background(), giznet.PublicKey{}, frame); !errors.Is(err, ErrInvalidPeer) {
		t.Fatalf("Report(invalid peer) error = %v, want %v", err, ErrInvalidPeer)
	}
	if err := (&Service{Metrics: &fakeMetricsStore{}}).Report(context.Background(), peer, nil); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("Report(nil frame) error = %v, want %v", err, ErrInvalidFrame)
	}
	var nilService *Service
	if err := nilService.Report(context.Background(), peer, frame); !errors.Is(err, ErrServiceNil) {
		t.Fatalf("Report(nil service) error = %v, want %v", err, ErrServiceNil)
	}
	statusStore := &fakeStatusStore{}
	if err := (&Service{Status: StatusSync{Store: statusStore}}).Report(context.Background(), peer, frame); err != nil {
		t.Fatalf("Report(nil metrics) error = %v, want nil", err)
	}
	if statusStore.puts != 1 {
		t.Fatalf("Report(nil metrics) status puts = %d, want 1", statusStore.puts)
	}
	if err := (&Service{Metrics: &fakeMetricsStore{}}).Report(context.Background(), peer, frame); !errors.Is(err, ErrStatusServiceNil) {
		t.Fatalf("Report(nil status) error = %v, want %v", err, ErrStatusServiceNil)
	}
}

func TestServiceReportPropagatesStoreErrors(t *testing.T) {
	peer := testPublicKey(t)
	percent := 25.0
	frame := &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &percent}},
		}},
	}
	metricsErr := errors.New("metrics down")
	statusStore := &fakeStatusStore{}
	service := &Service{
		Metrics: &fakeMetricsStore{err: metricsErr},
		Status:  StatusSync{Store: statusStore},
	}
	if err := service.Report(context.Background(), peer, frame); !errors.Is(err, metricsErr) {
		t.Fatalf("Report(metrics error) = %v, want %v", err, metricsErr)
	}
	if got := statusStore.puts; got != 1 {
		t.Fatalf("Report(metrics error) status puts = %d, want 1", got)
	}
	statusErr := errors.New("status down")
	service = &Service{
		Metrics: &fakeMetricsStore{},
		Status:  StatusSync{Store: &fakeStatusStore{err: statusErr}},
	}
	if err := service.Report(context.Background(), peer, frame); !errors.Is(err, statusErr) {
		t.Fatalf("Report(status error) = %v, want %v", err, statusErr)
	}
}

func TestMapFrameStringOnlyObservationsDoNotCreateSamples(t *testing.T) {
	peer := testPublicKey(t)
	firmware := "v1"
	rat := "lte"
	samples, patch, err := MapFrame(peer, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{
			{Body: &telemetrypb.Observation_Network{Network: &telemetrypb.NetworkObservation{Rat: &rat}}},
			{Body: &telemetrypb.Observation_System{System: &telemetrypb.SystemObservation{FirmwareVersion: &firmware}}},
		},
	}, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatalf("MapFrame() error = %v", err)
	}
	if len(samples) != 0 {
		t.Fatalf("samples = %+v, want empty", samples)
	}
	if !patch.Empty() {
		t.Fatalf("patch = %+v, want empty", patch)
	}
}

func TestMapFrameRejectsInvalidGNSSAndMissingBodies(t *testing.T) {
	peer := testPublicKey(t)
	_, _, err := MapFrame(peer, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{}},
	}, time.Unix(1, 0).UTC())
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("MapFrame(missing body) error = %v, want %v", err, ErrInvalidFrame)
	}
	_, _, err = MapFrame(peer, &telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Gnss{Gnss: &telemetrypb.GnssObservation{Latitude: 91, Longitude: 121}},
		}},
	}, time.Unix(1, 0).UTC())
	if !errors.Is(err, ErrInvalidFrame) || !strings.Contains(err.Error(), "latitude") {
		t.Fatalf("MapFrame(invalid gnss) error = %v", err)
	}
}

func TestStatusSyncEdges(t *testing.T) {
	peer := testPublicKey(t)
	if err := (StatusSync{}).SyncTelemetryStatus(context.Background(), peer, StatusPatch{BatteryPercent: intPtr(50)}); !errors.Is(err, ErrStatusServiceNil) {
		t.Fatalf("SyncTelemetryStatus(nil store) error = %v, want %v", err, ErrStatusServiceNil)
	}
	store := &fakeStatusStore{}
	if err := (StatusSync{Store: store}).SyncTelemetryStatus(context.Background(), peer, StatusPatch{}); err != nil {
		t.Fatalf("SyncTelemetryStatus(empty patch) error = %v", err)
	}
	if store.puts != 0 {
		t.Fatalf("empty patch puts = %d, want 0", store.puts)
	}
	currentReportedAt := time.Unix(200, 0).UTC()
	store.status.ReportedAt = &currentReportedAt
	currentPercent := 80
	store.status.BatteryPercent = &currentPercent
	staleReportedAt := currentReportedAt.Add(-time.Second)
	if err := (StatusSync{Store: store}).SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt:     staleReportedAt,
		BatteryPercent: intPtr(10),
	}); err != nil {
		t.Fatalf("SyncTelemetryStatus(stale patch) error = %v", err)
	}
	if store.puts != 0 {
		t.Fatalf("stale patch puts = %d, want 0", store.puts)
	}
	if store.status.BatteryPercent == nil || *store.status.BatteryPercent != 80 {
		t.Fatalf("stale patch BatteryPercent = %#v, want preserved 80", store.status.BatteryPercent)
	}
	store.status = apitypes.PeerStatus{ReportedAt: &currentReportedAt}
	if err := (StatusSync{Store: store}).SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt:     staleReportedAt,
		BatteryPercent: intPtr(10),
	}); err != nil {
		t.Fatalf("SyncTelemetryStatus(stale missing field) error = %v", err)
	}
	if store.puts != 1 {
		t.Fatalf("stale missing field puts = %d, want 1", store.puts)
	}
	if store.status.ReportedAt == nil || !store.status.ReportedAt.Equal(currentReportedAt) {
		t.Fatalf("stale missing field ReportedAt = %#v, want preserved %s", store.status.ReportedAt, currentReportedAt)
	}
	if store.status.BatteryPercent == nil || *store.status.BatteryPercent != 10 {
		t.Fatalf("stale missing field BatteryPercent = %#v, want 10", store.status.BatteryPercent)
	}
}

func marshalFrame(t *testing.T, frame *telemetrypb.TelemetryFrame) []byte {
	t.Helper()
	data, err := proto.Marshal(frame)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func testPublicKey(t *testing.T) giznet.PublicKey {
	t.Helper()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	return keyPair.Public
}

func assertSample(t *testing.T, samples []metrics.Sample, name string, ts time.Time, value float64) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name {
			if !sample.Timestamp.Equal(ts) {
				t.Fatalf("%s timestamp = %s, want %s", name, sample.Timestamp, ts)
			}
			if sample.Value != value {
				t.Fatalf("%s value = %v, want %v", name, sample.Value, value)
			}
			return
		}
	}
	t.Fatalf("sample %q not found in %+v", name, samples)
}

type fakeMetricsStore struct {
	samples     []metrics.Sample
	err         error
	deadlineSet bool
	deadline    time.Time
}

func (s *fakeMetricsStore) Append(ctx context.Context, samples []metrics.Sample) error {
	if s.err != nil {
		return s.err
	}
	s.deadline, s.deadlineSet = ctx.Deadline()
	s.samples = append(s.samples, samples...)
	return nil
}

func (s *fakeMetricsStore) Query(context.Context, metrics.Query) (metrics.SeriesSet, error) {
	return nil, nil
}

func (s *fakeMetricsStore) QueryRange(context.Context, metrics.RangeQuery) (metrics.SeriesSet, error) {
	return nil, nil
}

func (s *fakeMetricsStore) Close() error {
	return nil
}

type fakeStatusStore struct {
	status apitypes.PeerStatus
	puts   int
	err    error
}

func (s *fakeStatusStore) GetStatus(context.Context, giznet.PublicKey) (apitypes.PeerStatus, error) {
	if s.err != nil {
		return apitypes.PeerStatus{}, s.err
	}
	return s.status, nil
}

func (s *fakeStatusStore) PutStatus(_ context.Context, _ giznet.PublicKey, status apitypes.PeerStatus) (apitypes.PeerStatus, error) {
	s.status = status
	s.puts++
	return status, nil
}

func intPtr(v int) *int {
	return &v
}
