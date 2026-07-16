package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// ClickHouseConfig configures a ClickHouse metrics backend.
type ClickHouseConfig struct {
	DSN   string `yaml:"dsn"`
	Table string `yaml:"table"`
}

// ClickHouseStore persists metrics in a ClickHouse MergeTree table.
type ClickHouseStore struct {
	db    *sql.DB
	table string
}

var clickHouseTableRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// NewClickHouseStore connects to ClickHouse, ensures the table, and validates its schema.
func NewClickHouseStore(cfg ClickHouseConfig) (*ClickHouseStore, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("metrics: clickhouse dsn is required")
	}
	if !clickHouseTableRE.MatchString(cfg.Table) {
		return nil, fmt.Errorf("metrics: invalid clickhouse table %q", cfg.Table)
	}
	db, err := sql.Open("clickhouse", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("metrics: open clickhouse: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("metrics: ping clickhouse: %w", err)
	}
	ddl := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (metric String, series_key String, labels Map(String, String), timestamp DateTime64(3, 'UTC'), value Float64) ENGINE = MergeTree PARTITION BY toYYYYMM(timestamp) ORDER BY (metric, series_key, timestamp)", cfg.Table)
	if _, err = db.ExecContext(ctx, ddl); err != nil {
		db.Close()
		return nil, fmt.Errorf("metrics: create clickhouse table: %w", err)
	}
	store := &ClickHouseStore{db: db, table: cfg.Table}
	if err := store.checkSchema(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}
func (s *ClickHouseStore) checkSchema(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "SELECT name, type FROM system.columns WHERE database=currentDatabase() AND table=?", s.table)
	if err != nil {
		return fmt.Errorf("metrics: inspect clickhouse schema: %w", err)
	}
	defer rows.Close()
	want := map[string]string{"metric": "String", "series_key": "String", "labels": "Map(String, String)", "timestamp": "DateTime64(3, 'UTC')", "value": "Float64"}
	got := map[string]string{}
	for rows.Next() {
		var n, t string
		if err := rows.Scan(&n, &t); err != nil {
			return err
		}
		got[n] = t
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for n, t := range want {
		if got[n] != t {
			return fmt.Errorf("metrics: incompatible clickhouse column %s: got %q, want %q", n, got[n], t)
		}
	}
	return nil
}

// Append writes all samples through one driver batch.
func (s *ClickHouseStore) Append(ctx context.Context, samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	for _, v := range samples {
		if err := validateSample(v); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("metrics: begin clickhouse batch: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf("INSERT INTO `%s` (metric, series_key, labels, timestamp, value) SETTINGS async_insert=1, wait_for_async_insert=1 VALUES (?, ?, ?, ?, ?)", s.table))
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("metrics: prepare clickhouse batch: %w", err)
	}
	defer stmt.Close()
	for _, v := range samples {
		if _, err = stmt.ExecContext(ctx, v.Name, memorySeriesKey(v.Name, v.Labels), v.Labels, v.Timestamp.UTC(), v.Value); err != nil {
			tx.Rollback()
			return fmt.Errorf("metrics: append clickhouse batch: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("metrics: commit clickhouse batch: %w", err)
	}
	return nil
}

// Latest returns the newest matching sample with its stored timestamp.
func (s *ClickHouseStore) Latest(ctx context.Context, q LatestQuery) (SeriesSet, error) {
	if err := validateLatestQuery(q); err != nil {
		return nil, err
	}
	where, args := s.where(q.Selector, q.At.Add(-q.Lookback), q.At, true)
	query := fmt.Sprintf("SELECT any(metric), any(labels), argMax(value, timestamp), max(timestamp) FROM `%s` WHERE %s GROUP BY series_key", s.table, where)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("metrics: query clickhouse latest: %w", err)
	}
	defer rows.Close()
	out := SeriesSet{}
	for rows.Next() {
		var item Series
		var point Point
		if err := rows.Scan(&item.Name, &item.Labels, &point.Value, &point.Timestamp); err != nil {
			return nil, err
		}
		point.Timestamp = point.Timestamp.UTC()
		item.Points = []Point{point}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortSeries(out)
	return out, nil
}

// Range evaluates last-sample windows with ClickHouse argMax grouping.
func (s *ClickHouseStore) Range(ctx context.Context, q RangeQuery) (SeriesSet, error) {
	if err := validateRangeQuery(q); err != nil {
		return nil, err
	}
	if q.Step < time.Millisecond || q.Step%time.Millisecond != 0 {
		return nil, fmt.Errorf("metrics: clickhouse range step must have positive millisecond precision")
	}
	start, end := q.Start.UTC(), q.End.UTC()
	alignedEnd := start.Add(end.Sub(start) / q.Step * q.Step)
	where, args := s.where(q.Selector, start, alignedEnd, true)
	stepMS := q.Step.Milliseconds()
	innerArgs := []any{start, start, stepMS}
	innerArgs = append(innerArgs, args...)
	query := fmt.Sprintf("SELECT any(metric), any(labels), evaluation_index, argMax(value, timestamp) FROM (SELECT metric, series_key, labels, timestamp, value, if(timestamp = ?, 0, intDiv(dateDiff('millisecond', ?, timestamp) - 1, ?) + 1) AS evaluation_index FROM `%s` WHERE %s) GROUP BY series_key, evaluation_index ORDER BY series_key, evaluation_index", s.table, where)
	rows, err := s.db.QueryContext(ctx, query, innerArgs...)
	if err != nil {
		return nil, fmt.Errorf("metrics: query clickhouse range: %w", err)
	}
	byKey := map[string]*Series{}
	for rows.Next() {
		var name string
		var labels map[string]string
		var index int64
		var value float64
		if err := rows.Scan(&name, &labels, &index, &value); err != nil {
			rows.Close()
			return nil, err
		}
		appendSeriesPoint(byKey, name, labels, Point{Timestamp: start.Add(time.Duration(index) * q.Step), Value: value})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if alignedEnd.Before(end) {
		tailWhere, tailArgs := s.where(q.Selector, end.Add(-q.Step), end, false)
		tailQuery := fmt.Sprintf("SELECT any(metric), any(labels), argMax(value, timestamp) FROM `%s` WHERE %s GROUP BY series_key", s.table, tailWhere)
		tailRows, err := s.db.QueryContext(ctx, tailQuery, tailArgs...)
		if err != nil {
			return nil, fmt.Errorf("metrics: query clickhouse range tail: %w", err)
		}
		for tailRows.Next() {
			var name string
			var labels map[string]string
			var value float64
			if err := tailRows.Scan(&name, &labels, &value); err != nil {
				tailRows.Close()
				return nil, err
			}
			appendSeriesPoint(byKey, name, labels, Point{Timestamp: end, Value: value})
		}
		if err := tailRows.Err(); err != nil {
			tailRows.Close()
			return nil, err
		}
		if err := tailRows.Close(); err != nil {
			return nil, err
		}
	}
	out := make(SeriesSet, 0, len(byKey))
	for _, item := range byKey {
		out = append(out, *item)
	}
	sortSeries(out)
	return out, nil
}

// Aggregate evaluates bucket operations with ClickHouse aggregation functions.
func (s *ClickHouseStore) Aggregate(ctx context.Context, q AggregateQuery) (SeriesSet, error) {
	if err := validateAggregateQuery(q); err != nil {
		return nil, err
	}
	if q.Bucket < time.Millisecond || q.Bucket%time.Millisecond != 0 {
		return nil, fmt.Errorf("metrics: clickhouse aggregate bucket must have positive millisecond precision")
	}
	function := map[Aggregation]string{AggregationAvg: "avg(value)", AggregationMin: "min(value)", AggregationMax: "max(value)", AggregationSum: "sum(value)", AggregationCount: "toFloat64(count())", AggregationLast: "argMax(value, timestamp)"}[q.Operation]
	where, args := s.where(q.Selector, q.Start, q.End, true)
	bucketMS := q.Bucket.Milliseconds()
	innerArgs := []any{q.Start.UTC(), q.Start.UTC(), bucketMS}
	innerArgs = append(innerArgs, args...)
	query := fmt.Sprintf("SELECT any(metric), any(labels), bucket_index, %s FROM (SELECT metric, series_key, labels, timestamp, value, if(timestamp = ?, 0, intDiv(dateDiff('millisecond', ?, timestamp) - 1, ?)) AS bucket_index FROM `%s` WHERE %s) GROUP BY series_key, bucket_index ORDER BY series_key, bucket_index", function, s.table, where)
	rows, err := s.db.QueryContext(ctx, query, innerArgs...)
	if err != nil {
		return nil, fmt.Errorf("metrics: query clickhouse aggregate: %w", err)
	}
	defer rows.Close()
	byKey := map[string]*Series{}
	order := []string{}
	for rows.Next() {
		var name string
		var labels map[string]string
		var index int64
		var value float64
		if err := rows.Scan(&name, &labels, &index, &value); err != nil {
			return nil, err
		}
		key := memorySeriesKey(name, labels)
		item := byKey[key]
		if item == nil {
			item = &Series{Name: name, Labels: labels}
			byKey[key] = item
			order = append(order, key)
		}
		item.Points = append(item.Points, Point{Timestamp: q.Start.UTC().Add(time.Duration(index) * q.Bucket), Value: value})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(SeriesSet, 0, len(order))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	return out, nil
}

func (s *ClickHouseStore) where(sel Selector, start, end time.Time, inclusiveStart bool) (string, []any) {
	parts := []string{"metric = ?"}
	args := []any{sel.Name}
	if inclusiveStart {
		parts = append(parts, "timestamp >= ?")
	} else {
		parts = append(parts, "timestamp > ?")
	}
	args = append(args, start.UTC())
	parts = append(parts, "timestamp <= ?")
	args = append(args, end.UTC())
	for _, m := range sel.Matchers {
		switch m.Op {
		case MatchEqual:
			parts = append(parts, "labels[?] = ?")
			args = append(args, m.Name, m.Value)
		case MatchNotEqual:
			parts = append(parts, "labels[?] != ?")
			args = append(args, m.Name, m.Value)
		case MatchRegexp:
			parts = append(parts, "match(labels[?], ?)")
			args = append(args, m.Name, "^(?:"+m.Value+")$")
		case MatchNotRegexp:
			parts = append(parts, "NOT match(labels[?], ?)")
			args = append(args, m.Name, "^(?:"+m.Value+")$")
		}
	}
	return strings.Join(parts, " AND "), args
}

func appendSeriesPoint(items map[string]*Series, name string, labels map[string]string, point Point) {
	key := memorySeriesKey(name, labels)
	item := items[key]
	if item == nil {
		item = &Series{Name: name, Labels: labels}
		items[key] = item
	}
	item.Points = append(item.Points, point)
}

// Close closes the ClickHouse connection pool.
func (s *ClickHouseStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
