package gizmetrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

const (
	warningInterval   = time.Minute
	maxMetricNameSize = 128
	maxLabelNameSize  = 128
)

type recorderConfig struct {
	flushInterval time.Duration
	appendTimeout time.Duration
	maxSeries     int
}

type metricKind uint8

const (
	metricKindCounter metricKind = iota + 1
	metricKindGauge
	metricKindHistogram
)

type metricSeries struct {
	kind       metricKind
	name       string
	labels     []Label
	value      float64
	buckets    []float64
	bucketHits []uint64
	count      uint64
	version    uint64
	dirty      bool
}

type pendingSeries struct {
	key     string
	version uint64
	samples []storemetrics.Sample
}

type storeRecorder struct {
	store  storemetrics.Store
	config recorderConfig

	mu     sync.Mutex
	series map[string]*metricSeries
	closed bool

	flushMu sync.Mutex
	stop    chan struct{}
	done    chan struct{}

	warningMu   sync.Mutex
	nextWarning time.Time
}

func newStoreRecorder(store storemetrics.Store, config recorderConfig) *storeRecorder {
	return &storeRecorder{
		store:  store,
		config: config,
		series: make(map[string]*metricSeries),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (r *storeRecorder) start() {
	go r.run()
}

func (r *storeRecorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(r.config.flushInterval)
	defer ticker.Stop()
	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-r.stop:
			cancel()
		case <-workerCtx.Done():
		}
	}()
	for {
		select {
		case <-ticker.C:
			_ = r.flush(workerCtx)
		case <-r.stop:
			return
		}
	}
}

func (r *storeRecorder) shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.stop)
	}
	r.mu.Unlock()

	select {
	case <-r.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return r.flush(ctx)
}

func (r *storeRecorder) addCounter(ctx context.Context, name string, delta float64, labels []Label) {
	if !finite(delta) || delta < 0 {
		r.warn(ctx, name, "counter delta must be finite and non-negative")
		return
	}
	r.record(ctx, metricKindCounter, name, delta, nil, labels)
}

func (r *storeRecorder) setGauge(ctx context.Context, name string, value float64, labels []Label) {
	if !finite(value) {
		r.warn(ctx, name, "gauge value must be finite")
		return
	}
	r.record(ctx, metricKindGauge, name, value, nil, labels)
}

func (r *storeRecorder) observeHistogram(ctx context.Context, name string, value float64, buckets []float64, labels []Label) {
	if !finite(value) {
		r.warn(ctx, name, "histogram value must be finite")
		return
	}
	if err := validateBuckets(buckets); err != nil {
		r.warn(ctx, name, err.Error())
		return
	}
	r.record(ctx, metricKindHistogram, name, value, buckets, labels)
}

func (r *storeRecorder) record(ctx context.Context, kind metricKind, name string, value float64, buckets []float64, labels []Label) {
	if !validMetricName(name) {
		r.warn(ctx, "", "invalid metric name")
		return
	}
	canonical, err := canonicalLabels(labels, kind == metricKindHistogram)
	if err != nil {
		r.warn(ctx, name, err.Error())
		return
	}
	key := seriesKey(name, canonical)

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	series := r.series[key]
	if series == nil {
		if len(r.series) >= r.config.maxSeries {
			r.mu.Unlock()
			r.warn(ctx, name, "maximum series reached")
			return
		}
		series = &metricSeries{
			kind:   kind,
			name:   name,
			labels: canonical,
		}
		if kind == metricKindHistogram {
			series.buckets = slices.Clone(buckets)
			series.bucketHits = make([]uint64, len(buckets))
		}
		r.series[key] = series
	}
	if series.kind != kind {
		r.mu.Unlock()
		r.warn(ctx, name, "metric kind changed for existing series")
		return
	}
	if kind == metricKindHistogram && !slices.Equal(series.buckets, buckets) {
		r.mu.Unlock()
		r.warn(ctx, name, "histogram buckets changed for existing series")
		return
	}

	switch kind {
	case metricKindCounter:
		next := series.value + value
		if !finite(next) {
			r.mu.Unlock()
			r.warn(ctx, name, "counter total must remain finite")
			return
		}
		series.value = next
	case metricKindGauge:
		series.value = value
	case metricKindHistogram:
		next := series.value + value
		if !finite(next) {
			r.mu.Unlock()
			r.warn(ctx, name, "histogram sum must remain finite")
			return
		}
		series.value = next
		series.count++
		for index, upperBound := range series.buckets {
			if value <= upperBound {
				series.bucketHits[index]++
			}
		}
	}
	series.version++
	series.dirty = true
	r.mu.Unlock()
}

func (r *storeRecorder) flush(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.flushMu.Lock()
	defer r.flushMu.Unlock()

	pending := r.snapshot(time.Now().UTC())
	if len(pending) == 0 {
		return nil
	}
	var samples []storemetrics.Sample
	for _, item := range pending {
		samples = append(samples, item.samples...)
	}
	appendCtx, cancel := context.WithTimeout(ctx, r.config.appendTimeout)
	err := r.store.Append(appendCtx, samples)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			r.warnFlush(ctx, "store append failed")
		}
		return fmt.Errorf("gizmetrics: append samples: %w", err)
	}
	r.markClean(pending)
	return nil
}

func (r *storeRecorder) snapshot(timestamp time.Time) []pendingSeries {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending := make([]pendingSeries, 0, len(r.series))
	for key, series := range r.series {
		if !series.dirty {
			continue
		}
		pending = append(pending, pendingSeries{
			key:     key,
			version: series.version,
			samples: seriesSamples(series, timestamp),
		})
	}
	return pending
}

func (r *storeRecorder) markClean(pending []pendingSeries) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range pending {
		series := r.series[item.key]
		if series != nil && series.version == item.version {
			series.dirty = false
		}
	}
}

func (r *storeRecorder) warn(ctx context.Context, name string, reason string) {
	r.warnRecord(ctx, "gizmetrics: metric update dropped", name, reason)
}

func (r *storeRecorder) warnFlush(ctx context.Context, reason string) {
	r.warnRecord(ctx, "gizmetrics: flush failed", "", reason)
}

func (r *storeRecorder) warnRecord(ctx context.Context, message, name, reason string) {
	r.warningMu.Lock()
	now := time.Now()
	if now.Before(r.nextWarning) {
		r.warningMu.Unlock()
		return
	}
	r.nextWarning = now.Add(warningInterval)
	r.warningMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	attrs := []slog.Attr{slog.String("reason", reason)}
	if validMetricName(name) {
		attrs = append(attrs, slog.String("metric", name))
	}
	slog.LogAttrs(ctx, slog.LevelWarn, message, attrs...)
}

func canonicalLabels(labels []Label, histogram bool) ([]Label, error) {
	canonical := slices.Clone(labels)
	for _, label := range canonical {
		if len(label.Name) > maxLabelNameSize || storemetrics.ValidateLabelName(label.Name) != nil {
			return nil, fmt.Errorf("invalid label name")
		}
		if histogram && label.Name == "le" {
			return nil, fmt.Errorf("histogram label le is reserved")
		}
	}
	slices.SortFunc(canonical, func(left, right Label) int {
		return strings.Compare(left.Name, right.Name)
	})
	for index := 1; index < len(canonical); index++ {
		if canonical[index-1].Name == canonical[index].Name {
			return nil, fmt.Errorf("duplicate label name")
		}
	}
	return canonical, nil
}

func validMetricName(name string) bool {
	return name != "" && len(name) <= maxMetricNameSize && storemetrics.ValidateMetricName(name) == nil
}

func seriesKey(name string, labels []Label) string {
	var key strings.Builder
	key.WriteString(strconv.Itoa(len(name)))
	key.WriteByte(':')
	key.WriteString(name)
	for _, label := range labels {
		key.WriteByte('|')
		key.WriteString(strconv.Itoa(len(label.Name)))
		key.WriteByte(':')
		key.WriteString(label.Name)
		key.WriteByte('=')
		key.WriteString(strconv.Itoa(len(label.Value)))
		key.WriteByte(':')
		key.WriteString(label.Value)
	}
	return key.String()
}

func validateBuckets(buckets []float64) error {
	if len(buckets) == 0 {
		return errors.New("histogram buckets are required")
	}
	for index, bucket := range buckets {
		if !finite(bucket) {
			return errors.New("histogram buckets must be finite")
		}
		if index > 0 && bucket <= buckets[index-1] {
			return errors.New("histogram buckets must be strictly increasing")
		}
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func seriesSamples(series *metricSeries, timestamp time.Time) []storemetrics.Sample {
	labels := labelsMap(series.labels)
	switch series.kind {
	case metricKindCounter, metricKindGauge:
		return []storemetrics.Sample{{Name: series.name, Labels: labels, Timestamp: timestamp, Value: series.value}}
	case metricKindHistogram:
		samples := make([]storemetrics.Sample, 0, len(series.buckets)+3)
		for index, bucket := range series.buckets {
			bucketLabels := cloneLabelMap(labels)
			bucketLabels["le"] = strconv.FormatFloat(bucket, 'g', -1, 64)
			samples = append(samples, storemetrics.Sample{
				Name:      series.name + "_bucket",
				Labels:    bucketLabels,
				Timestamp: timestamp,
				Value:     float64(series.bucketHits[index]),
			})
		}
		infiniteLabels := cloneLabelMap(labels)
		infiniteLabels["le"] = "+Inf"
		samples = append(samples,
			storemetrics.Sample{Name: series.name + "_bucket", Labels: infiniteLabels, Timestamp: timestamp, Value: float64(series.count)},
			storemetrics.Sample{Name: series.name + "_sum", Labels: cloneLabelMap(labels), Timestamp: timestamp, Value: series.value},
			storemetrics.Sample{Name: series.name + "_count", Labels: cloneLabelMap(labels), Timestamp: timestamp, Value: float64(series.count)},
		)
		return samples
	default:
		return nil
	}
}

func labelsMap(labels []Label) map[string]string {
	mapped := make(map[string]string, len(labels))
	for _, label := range labels {
		mapped[label.Name] = label.Value
	}
	return mapped
}

func cloneLabelMap(labels map[string]string) map[string]string {
	cloned := make(map[string]string, len(labels)+1)
	maps.Copy(cloned, labels)
	return cloned
}
