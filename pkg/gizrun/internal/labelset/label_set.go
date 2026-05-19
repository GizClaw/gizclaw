package labelset

import (
	"context"
	"iter"
	"log/slog"
	"maps"

	"github.com/prometheus/client_golang/prometheus"
)

type LabelSet struct {
	name   string
	keys   []string
	values map[string]string
}

type contextKey struct {
	name string
}

// Tag returns a context with the label set updated by key/value pairs.
// It uses context.Background when ctx is nil, creates the label set when it
// does not exist, and copies the existing label set before merging values.
func Tag(ctx context.Context, name string, kvs ...string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if name == "" || len(kvs) < 2 {
		return ctx
	}
	ns, _ := FromContext(ctx, name)
	ns = ns.clone()
	ns.name = name
	for i := 0; i+1 < len(kvs); i += 2 {
		ns.set(kvs[i], kvs[i+1])
	}
	return context.WithValue(ctx, contextKey{name: name}, ns)
}

func FromContext(ctx context.Context, name string) (LabelSet, bool) {
	if ctx != nil {
		if ns, ok := ctx.Value(contextKey{name: name}).(LabelSet); ok {
			return ns, true
		}
	}
	return LabelSet{}, false
}

func (ns *LabelSet) set(key, value string) {
	if key == "" {
		return
	}
	if ns.values == nil {
		ns.values = map[string]string{}
	}
	if _, ok := ns.values[key]; !ok {
		ns.keys = append(ns.keys, key)
	}
	ns.values[key] = value
}

func (ns LabelSet) clone() LabelSet {
	next := LabelSet{
		name:   ns.name,
		keys:   append([]string(nil), ns.keys...),
		values: make(map[string]string, len(ns.values)),
	}
	maps.Copy(next.values, ns.values)
	return next
}

func (ns LabelSet) Name() string {
	return ns.name
}

func (ns LabelSet) Value(key string) (string, bool) {
	if ns.values == nil {
		return "", false
	}
	value, ok := ns.values[key]
	return value, ok
}

func (ns LabelSet) Keys() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, key := range ns.keys {
			if !yield(key) {
				return
			}
		}
	}
}

func (ns LabelSet) Values() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, key := range ns.keys {
			if !yield(ns.values[key]) {
				return
			}
		}
	}
}

func (ns LabelSet) Labels() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for _, key := range ns.keys {
			if !yield(key, ns.values[key]) {
				return
			}
		}
	}
}

func (ns LabelSet) Attr() slog.Attr {
	attrs := make([]slog.Attr, 0, len(ns.keys))
	for key, value := range ns.Labels() {
		attrs = append(attrs, slog.String(key, value))
	}
	return slog.Attr{Key: ns.name, Value: slog.GroupValue(attrs...)}
}

func (ns LabelSet) PrometheusLabels() prometheus.Labels {
	labels := make(prometheus.Labels, len(ns.values))
	maps.Insert(labels, ns.Labels())
	return labels
}
