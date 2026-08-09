package metrics

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlbackend"
	"github.com/jmoiron/sqlx"
)

const sqlStoreInitializationTimeout = 30 * time.Second

// SQLStore persists metric samples in one SQLite or PostgreSQL table while
// borrowing the caller-owned connection pool.
type SQLStore struct {
	db      *sqlx.DB
	backend sqlbackend.Backend
	quoted  string

	mu     sync.RWMutex
	closed bool
}

// NewSQLStoreWithDB constructs a table-scoped SQL metrics Store.
func NewSQLStoreWithDB(db *sqlx.DB, table string) (*SQLStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sqlStoreInitializationTimeout)
	defer cancel()
	return newSQLStoreWithDB(ctx, db, table)
}

func newSQLStoreWithDB(ctx context.Context, db *sqlx.DB, table string) (*SQLStore, error) {
	backend, err := sqlbackend.Prepare(db, "metrics", table)
	if err != nil {
		return nil, fmt.Errorf("metrics: sql table %q: %w", table, err)
	}
	if err := ensureSQLSchema(ctx, db, backend); err != nil {
		return nil, fmt.Errorf("metrics: initialize sql table %q: %w", table, err)
	}
	store := &SQLStore{db: db, backend: backend, quoted: backend.Quoted}
	if err := store.checkSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *SQLStore) checkSchema(ctx context.Context) error {
	columns, err := sqlbackend.Columns(ctx, store.db, store.backend)
	if err != nil {
		return fmt.Errorf("metrics: inspect sql table %q: %w", store.backend.Table, err)
	}
	sequenceNullable := true
	if store.backend.Dialect == sqlbackend.PostgreSQL {
		sequenceNullable = false
	}
	want := map[string]sqlbackend.Column{
		"sequence":            {Type: "BIGINT", Nullable: sequenceNullable, PrimaryKeyPosition: 1},
		"metric":              {Type: "TEXT", Nullable: false},
		"series_key":          {Type: "TEXT", Nullable: false},
		"labels_json":         {Type: "TEXT", Nullable: false},
		"timestamp_unix_nano": {Type: "BIGINT", Nullable: false},
		"value_bits":          {Type: "BIGINT", Nullable: false},
	}
	if store.backend.Dialect == sqlbackend.SQLite {
		column := want["sequence"]
		column.Type = "INTEGER"
		want["sequence"] = column
	}
	if err := sqlbackend.ValidateColumns(columns, want); err != nil {
		return fmt.Errorf("metrics: incompatible sql table %q: %w", store.backend.Table, err)
	}
	if err := sqlbackend.ValidateSequenceIdentity(ctx, store.db, store.backend, "sequence"); err != nil {
		return fmt.Errorf("metrics: incompatible sql table %q sequence: %w", store.backend.Table, err)
	}
	for _, index := range sqlMetricIndexes {
		name, err := sqlbackend.IndexName(store.backend, index.purpose)
		if err != nil {
			return fmt.Errorf("metrics: derive sql table %q index: %w", store.backend.Table, err)
		}
		if err := sqlbackend.ValidateIndexColumns(ctx, store.db, store.backend, name, index.want); err != nil {
			return fmt.Errorf("metrics: incompatible sql table %q index %q: %w", store.backend.Table, name, err)
		}
	}
	return nil
}

func (store *SQLStore) lock() (func(), error) {
	if store == nil {
		return nil, errors.New("metrics: sql store is nil")
	}
	store.mu.RLock()
	if store.closed || store.db == nil {
		store.mu.RUnlock()
		return nil, errors.New("metrics: sql store is closed")
	}
	return store.mu.RUnlock, nil
}

// Append validates and stores a complete batch atomically.
func (store *SQLStore) Append(ctx context.Context, samples []Sample) error {
	unlock, err := store.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	type preparedSample struct {
		name, seriesKey, labelsJSON string
		timestamp, valueBits        int64
	}
	prepared := make([]preparedSample, 0, len(samples))
	for _, sample := range samples {
		if err := validateSample(sample); err != nil {
			return err
		}
		labels := sample.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		encoded, err := json.Marshal(labels)
		if err != nil {
			return fmt.Errorf("metrics: encode sql labels: %w", err)
		}
		timestamp, err := sqlbackend.UnixNano(sample.Timestamp)
		if err != nil {
			return fmt.Errorf("metrics: sample timestamp: %w", err)
		}
		prepared = append(prepared, preparedSample{
			name: sample.Name, seriesKey: memorySeriesKey(sample.Name, labels), labelsJSON: string(encoded),
			timestamp: timestamp, valueBits: int64(math.Float64bits(sample.Value)),
		})
	}
	if len(prepared) == 0 {
		return nil
	}
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		return sqlbackend.ExternalError("metrics: begin sql append", err)
	}
	defer tx.Rollback()
	query := sqlbackend.Rebind(store.db, "INSERT INTO "+store.quoted+" (metric, series_key, labels_json, timestamp_unix_nano, value_bits) VALUES (?, ?, ?, ?, ?)")
	statement, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return sqlbackend.ExternalError("metrics: prepare sql append", err)
	}
	defer statement.Close()
	for _, sample := range prepared {
		if _, err := statement.ExecContext(ctx, sample.name, sample.seriesKey, sample.labelsJSON, sample.timestamp, sample.valueBits); err != nil {
			return sqlbackend.ExternalError("metrics: append sql sample", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return sqlbackend.ExternalError("metrics: commit sql append", err)
	}
	return nil
}

// Latest returns the newest matching sample at or before the query time.
func (store *SQLStore) Latest(ctx context.Context, query LatestQuery) (SeriesSet, error) {
	if err := validateLatestQuery(query); err != nil {
		return nil, err
	}
	series, err := store.load(ctx, query.Selector, query.At.Add(-query.Lookback), query.At)
	if err != nil {
		return nil, err
	}
	return buildSQLSeries(series, func(points []sqlMetricPoint) []Point {
		if len(points) == 0 {
			return nil
		}
		return []Point{points[len(points)-1].Point}
	}), nil
}

// Range evaluates the backend-neutral last-sample window contract.
func (store *SQLStore) Range(ctx context.Context, query RangeQuery) (SeriesSet, error) {
	if err := validateRangeQuery(query); err != nil {
		return nil, err
	}
	start, end := query.Start.UTC(), query.End.UTC()
	series, err := store.load(ctx, query.Selector, start, end)
	if err != nil {
		return nil, err
	}
	times := []time.Time{start}
	for at := start.Add(query.Step); !at.After(end); at = at.Add(query.Step) {
		times = append(times, at)
	}
	if times[len(times)-1].Before(end) {
		times = append(times, end)
	}
	return buildSQLSeries(series, func(points []sqlMetricPoint) []Point {
		out := make([]Point, 0, len(times))
		for index, at := range times {
			var selected *sqlMetricPoint
			for pointIndex := range points {
				point := &points[pointIndex]
				if point.Timestamp.After(at) {
					break
				}
				if index == 0 {
					if point.Timestamp.Equal(at) {
						selected = point
					}
					continue
				}
				if point.Timestamp.After(at.Add(-query.Step)) {
					selected = point
				}
			}
			if selected != nil {
				out = append(out, Point{Timestamp: at, Value: selected.Value})
			}
		}
		return out
	}), nil
}

// Aggregate evaluates one operation per Start-anchored bucket.
func (store *SQLStore) Aggregate(ctx context.Context, query AggregateQuery) (SeriesSet, error) {
	if err := validateAggregateQuery(query); err != nil {
		return nil, err
	}
	start, end := query.Start.UTC(), query.End.UTC()
	series, err := store.load(ctx, query.Selector, start, end)
	if err != nil {
		return nil, err
	}
	return buildSQLSeries(series, func(points []sqlMetricPoint) []Point {
		out := []Point{}
		first := true
		for bucketStart := start; !bucketStart.After(end); bucketStart = bucketStart.Add(query.Bucket) {
			bucketEnd := bucketStart.Add(query.Bucket)
			if bucketEnd.After(end) {
				bucketEnd = end
			}
			values := []Point{}
			for _, point := range points {
				if point.Timestamp.Before(bucketStart) || (!first && point.Timestamp.Equal(bucketStart)) || point.Timestamp.After(bucketEnd) {
					continue
				}
				values = append(values, point.Point)
			}
			if len(values) > 0 {
				out = append(out, Point{Timestamp: bucketStart, Value: aggregatePoints(query.Operation, values)})
			}
			first = false
			if bucketEnd.Equal(end) {
				break
			}
		}
		return out
	}), nil
}

// Close closes only the logical adapter.
func (store *SQLStore) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	store.closed = true
	store.mu.Unlock()
	return nil
}

type sqlMetricPoint struct {
	Point
	sequence int64
}

type sqlMetricSeries struct {
	name   string
	labels map[string]string
	points []sqlMetricPoint
}

func (store *SQLStore) load(ctx context.Context, selector Selector, start, end time.Time) (map[string]*sqlMetricSeries, error) {
	unlock, err := store.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	startNano, err := sqlbackend.UnixNano(start)
	if err != nil {
		return nil, fmt.Errorf("metrics: query start: %w", err)
	}
	endNano, err := sqlbackend.UnixNano(end)
	if err != nil {
		return nil, fmt.Errorf("metrics: query end: %w", err)
	}
	query := sqlbackend.Rebind(store.db, "SELECT sequence, series_key, labels_json, timestamp_unix_nano, value_bits FROM "+store.quoted+" WHERE metric = ? AND timestamp_unix_nano >= ? AND timestamp_unix_nano <= ? ORDER BY series_key, timestamp_unix_nano, sequence")
	rows, err := store.db.QueryContext(ctx, query, selector.Name, startNano, endNano)
	if err != nil {
		return nil, sqlbackend.ExternalError("metrics: query sql samples", err)
	}
	defer rows.Close()
	series := map[string]*sqlMetricSeries{}
	for rows.Next() {
		var sequence, timestamp, bits int64
		var key, labelsJSON string
		if err := rows.Scan(&sequence, &key, &labelsJSON, &timestamp, &bits); err != nil {
			return nil, sqlbackend.ExternalError("metrics: scan sql sample", err)
		}
		labels := map[string]string{}
		if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
			return nil, fmt.Errorf("metrics: decode sql labels: %w", err)
		}
		if !matchesSQLSelector(labels, selector.Matchers) {
			continue
		}
		item := series[key]
		if item == nil {
			item = &sqlMetricSeries{name: selector.Name, labels: labels}
			series[key] = item
		}
		item.points = append(item.points, sqlMetricPoint{
			Point:    Point{Timestamp: time.Unix(0, timestamp).UTC(), Value: math.Float64frombits(uint64(bits))},
			sequence: sequence,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, sqlbackend.ExternalError("metrics: query sql sample rows", err)
	}
	for _, item := range series {
		slices.SortFunc(item.points, func(left, right sqlMetricPoint) int {
			if order := left.Timestamp.Compare(right.Timestamp); order != 0 {
				return order
			}
			return cmp.Compare(left.sequence, right.sequence)
		})
	}
	return series, nil
}

func matchesSQLSelector(labels map[string]string, matchers []LabelMatcher) bool {
	for _, matcher := range matchers {
		value := labels[matcher.Name]
		matched := false
		switch matcher.Op {
		case MatchEqual:
			matched = value == matcher.Value
		case MatchNotEqual:
			matched = value != matcher.Value
		case MatchRegexp:
			matched = regexp.MustCompile("^(?:" + matcher.Value + ")$").MatchString(value)
		case MatchNotRegexp:
			matched = !regexp.MustCompile("^(?:" + matcher.Value + ")$").MatchString(value)
		}
		if !matched {
			return false
		}
	}
	return true
}

func buildSQLSeries(series map[string]*sqlMetricSeries, evaluate func([]sqlMetricPoint) []Point) SeriesSet {
	keys := make([]string, 0, len(series))
	for key := range series {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := SeriesSet{}
	for _, key := range keys {
		item := series[key]
		points := evaluate(item.points)
		if len(points) == 0 {
			continue
		}
		out = append(out, Series{Name: item.name, Labels: cloneLabels(item.labels), Points: points})
	}
	return out
}

var _ Store = (*SQLStore)(nil)
