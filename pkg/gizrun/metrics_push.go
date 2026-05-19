package gizrun

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

type metricsPushConfig struct {
	Enabled     bool                    `yaml:"enabled"`
	URL         string                  `yaml:"url"`
	Job         string                  `yaml:"job"`
	Username    string                  `yaml:"username"`
	Password    string                  `yaml:"password"`
	BearerToken string                  `yaml:"bearer_token"`
	Interval    string                  `yaml:"interval"`
	Grouping    metricsPushGroupingFlag `yaml:"grouping"`
}

type metricsPushGroupingFlag map[string]string

type metricsPusher struct {
	config   metricsPushConfig
	gatherer prometheus.Gatherer
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
}

func (f *metricsPushGroupingFlag) String() string {
	if f == nil || len(*f) == 0 {
		return ""
	}
	values := make([]string, 0, len(*f))
	for key, value := range *f {
		values = append(values, key+"="+value)
	}
	return strings.Join(values, ",")
}

func (f *metricsPushGroupingFlag) Set(value string) error {
	key, labelValue, ok := strings.Cut(value, "=")
	key = strings.TrimSpace(key)
	labelValue = strings.TrimSpace(labelValue)
	if !ok || key == "" {
		return errors.New("gizrun metrics push: grouping must be key=value")
	}
	if *f == nil {
		*f = map[string]string{}
	}
	(*f)[key] = labelValue
	return nil
}

func (f *metricsPushGroupingFlag) UnmarshalYAML(data []byte) error {
	values := map[string]string{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return err
	}
	grouping := metricsPushGroupingFlag{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			return errors.New("gizrun metrics push: grouping key is empty")
		}
		grouping[key] = value
	}
	*f = grouping
	return nil
}

func (c metricsPushConfig) normalize() (metricsPushConfig, error) {
	c.URL = strings.TrimSpace(os.ExpandEnv(c.URL))
	c.Job = strings.TrimSpace(os.ExpandEnv(c.Job))
	c.Username = strings.TrimSpace(os.ExpandEnv(c.Username))
	c.Password = os.ExpandEnv(c.Password)
	c.BearerToken = strings.TrimSpace(os.ExpandEnv(c.BearerToken))
	c.Interval = strings.TrimSpace(os.ExpandEnv(c.Interval))
	if c.Job == "" {
		c.Job = "gizrun"
	}
	if c.Interval == "" {
		c.Interval = "15s"
	}
	grouping := make(map[string]string, len(c.Grouping))
	for key, value := range c.Grouping {
		key = strings.TrimSpace(key)
		if key == "" {
			return metricsPushConfig{}, errors.New("gizrun metrics push: grouping key is empty")
		}
		grouping[key] = strings.TrimSpace(os.ExpandEnv(value))
	}
	c.Grouping = grouping
	if !c.Enabled {
		return c, nil
	}
	if c.URL == "" {
		return metricsPushConfig{}, errors.New("gizrun metrics push: url is required")
	}
	if c.BearerToken != "" && (c.Username != "" || c.Password != "") {
		return metricsPushConfig{}, errors.New("gizrun metrics push: bearer token and basic auth are mutually exclusive")
	}
	if _, err := c.pushInterval(); err != nil {
		return metricsPushConfig{}, err
	}
	return c, nil
}

func (c metricsPushConfig) pushInterval() (time.Duration, error) {
	interval, err := time.ParseDuration(c.Interval)
	if err != nil {
		return 0, fmt.Errorf("gizrun metrics push: parse interval: %w", err)
	}
	if interval <= 0 {
		return 0, errors.New("gizrun metrics push: interval must be positive")
	}
	return interval, nil
}

func newMetricsPusher(config metricsPushConfig, gatherer prometheus.Gatherer) (*metricsPusher, error) {
	if _, err := config.pushInterval(); err != nil {
		return nil, err
	}
	if gatherer == nil {
		return nil, errors.New("gizrun metrics push: gatherer is nil")
	}
	return &metricsPusher{
		config:   config,
		gatherer: gatherer,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}, nil
}

func (p *metricsPusher) start() {
	go p.run()
}

func (p *metricsPusher) stopAndWait() {
	p.once.Do(func() {
		close(p.stop)
		<-p.done
	})
}

func (p *metricsPusher) run() {
	defer close(p.done)
	interval, err := p.config.pushInterval()
	if err != nil {
		slog.Error("gizrun metrics push interval invalid", "error", err)
		return
	}
	p.push()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.push()
		case <-p.stop:
			return
		}
	}
}

func (p *metricsPusher) push() {
	pusher := push.New(p.config.URL, p.config.Job).Gatherer(p.gatherer)
	if p.config.BearerToken != "" {
		pusher.Client(&http.Client{Transport: bearerTokenTransport{token: p.config.BearerToken}})
	} else if p.config.Username != "" || p.config.Password != "" {
		pusher.BasicAuth(p.config.Username, p.config.Password)
	}
	for key, value := range p.config.Grouping {
		pusher.Grouping(key, value)
	}
	if err := pusher.Push(); err != nil {
		slog.Error("gizrun metrics push failed", "error", err)
	}
}

type bearerTokenTransport struct {
	base  http.RoundTripper
	token string
}

func (t bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	next := req.Clone(req.Context())
	next.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(next)
}
