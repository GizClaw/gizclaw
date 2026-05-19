package metrics

import (
	"errors"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	mu         sync.RWMutex
	registerer prometheus.Registerer
	gatherer   prometheus.Gatherer
	collectors map[string]prometheus.Collector
}

func New(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{}
	m.Reset(registerer)
	return m
}

func (m *Metrics) Reset(registerer prometheus.Registerer) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	gatherer, _ := registerer.(prometheus.Gatherer)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.registerer = registerer
	m.gatherer = gatherer
	m.collectors = nil
}

func (m *Metrics) Register(collector prometheus.Collector) error {
	_, err := m.register(collector)
	return err
}

func (m *Metrics) Collector(name string) (prometheus.Collector, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	collector, ok := m.collectors[name]
	return collector, ok
}

func (m *Metrics) register(collector prometheus.Collector) (prometheus.Collector, error) {
	if collector == nil {
		return nil, nil
	}

	m.mu.RLock()
	registerer := m.registerer
	m.mu.RUnlock()
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	if err := registerer.Register(collector); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			return already.ExistingCollector, nil
		}
		return nil, err
	}
	return collector, nil
}

func (m *Metrics) registerNamed(name string, collector prometheus.Collector) (prometheus.Collector, error) {
	registered, err := m.register(collector)
	if err != nil || name == "" || registered == nil {
		return registered, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.collectors == nil {
		m.collectors = map[string]prometheus.Collector{}
	}
	m.collectors[name] = registered
	return registered, nil
}

func (m *Metrics) Handler() http.Handler {
	m.mu.RLock()
	gatherer := m.gatherer
	m.mu.RUnlock()
	if gatherer == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gizrun metrics: registerer does not implement prometheus.Gatherer", http.StatusInternalServerError)
		})
	}
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}

func (m *Metrics) Gatherer() prometheus.Gatherer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gatherer
}
