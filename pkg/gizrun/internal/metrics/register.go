package metrics

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "gizclaw"

var defaultMetrics = New(prometheus.DefaultRegisterer)

func Reset(registerer prometheus.Registerer) {
	defaultMetrics.Reset(registerer)
}

func Register(collector prometheus.Collector) error {
	return defaultMetrics.Register(collector)
}

func RegisterCounter(name string, labels ...string) (*prometheus.CounterVec, error) {
	if name == "" {
		return nil, errors.New("gizrun metrics: counter name is empty")
	}
	if counter := Counter(name); counter != nil {
		return counter, nil
	}
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      name,
		Help:      defaultHelp(name),
	}, labels)
	registered, err := defaultMetrics.registerNamed(name, counter)
	if err != nil {
		return nil, err
	}
	if registeredCounter, ok := registered.(*prometheus.CounterVec); ok {
		return registeredCounter, nil
	}
	return nil, errors.New("gizrun metrics: metric " + name + " is not a counter")
}

func RegisterGauge(name string, labels ...string) (*prometheus.GaugeVec, error) {
	if name == "" {
		return nil, errors.New("gizrun metrics: gauge name is empty")
	}
	if gauge := Gauge(name); gauge != nil {
		return gauge, nil
	}
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      name,
		Help:      defaultHelp(name),
	}, labels)
	registered, err := defaultMetrics.registerNamed(name, gauge)
	if err != nil {
		return nil, err
	}
	if registeredGauge, ok := registered.(*prometheus.GaugeVec); ok {
		return registeredGauge, nil
	}
	return nil, errors.New("gizrun metrics: metric " + name + " is not a gauge")
}

func RegisterHistogram(name string, labels ...string) (*prometheus.HistogramVec, error) {
	if name == "" {
		return nil, errors.New("gizrun metrics: histogram name is empty")
	}
	if histogram := Histogram(name); histogram != nil {
		return histogram, nil
	}
	histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      name,
		Help:      defaultHelp(name),
		Buckets:   prometheus.DefBuckets,
	}, labels)
	registered, err := defaultMetrics.registerNamed(name, histogram)
	if err != nil {
		return nil, err
	}
	if registeredHistogram, ok := registered.(*prometheus.HistogramVec); ok {
		return registeredHistogram, nil
	}
	return nil, errors.New("gizrun metrics: metric " + name + " is not a histogram")
}

func RegisterSummary(name string, labels ...string) (*prometheus.SummaryVec, error) {
	if name == "" {
		return nil, errors.New("gizrun metrics: summary name is empty")
	}
	if summary := Summary(name); summary != nil {
		return summary, nil
	}
	summary := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: namespace,
		Name:      name,
		Help:      defaultHelp(name),
	}, labels)
	registered, err := defaultMetrics.registerNamed(name, summary)
	if err != nil {
		return nil, err
	}
	if registeredSummary, ok := registered.(*prometheus.SummaryVec); ok {
		return registeredSummary, nil
	}
	return nil, errors.New("gizrun metrics: metric " + name + " is not a summary")
}

func Handler() http.Handler {
	return defaultMetrics.Handler()
}

func Gatherer() prometheus.Gatherer {
	return defaultMetrics.Gatherer()
}

func Counter(name string) *prometheus.CounterVec {
	collector, ok := defaultMetrics.Collector(name)
	if !ok {
		return nil
	}
	counter, _ := collector.(*prometheus.CounterVec)
	return counter
}

func Gauge(name string) *prometheus.GaugeVec {
	collector, ok := defaultMetrics.Collector(name)
	if !ok {
		return nil
	}
	gauge, _ := collector.(*prometheus.GaugeVec)
	return gauge
}

func Histogram(name string) *prometheus.HistogramVec {
	collector, ok := defaultMetrics.Collector(name)
	if !ok {
		return nil
	}
	histogram, _ := collector.(*prometheus.HistogramVec)
	return histogram
}

func Summary(name string) *prometheus.SummaryVec {
	collector, ok := defaultMetrics.Collector(name)
	if !ok {
		return nil
	}
	summary, _ := collector.(*prometheus.SummaryVec)
	return summary
}

func defaultHelp(name string) string {
	return "GizClaw metric " + name + "."
}
