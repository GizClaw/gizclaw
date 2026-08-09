package storage

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
)

type prometheusResource struct {
	client         api.Client
	remoteWriteURL string
}

// Prometheus returns the named API client and validated remote-write URL.
func (s *Storage) Prometheus(name string) (api.Client, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", errors.New("storage: registry is closed")
	}
	resource, ok := s.prometheus[name]
	if !ok {
		return nil, "", fmt.Errorf("storage: prometheus %q not found", name)
	}
	return resource.client, resource.remoteWriteURL, nil
}

func newPrometheus(name string, cfg PrometheusConfig) (prometheusResource, error) {
	remoteWriteURL := strings.TrimSpace(cfg.RemoteWriteURL)
	queryURL := strings.TrimSpace(cfg.QueryURL)
	bearerToken := cfg.BearerToken
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "remote_write_url", value: remoteWriteURL},
		{name: "query_url", value: queryURL},
	} {
		u, err := url.ParseRequestURI(field.value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return prometheusResource{}, fmt.Errorf("storage: prometheus %q invalid %s", name, field.name)
		}
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	client, err := api.NewClient(api.Config{
		Address: queryURL,
		Client: &http.Client{
			Transport: bearerRoundTripper{token: bearerToken, next: transport},
			Timeout:   30 * time.Second,
		},
	})
	if err != nil {
		return prometheusResource{}, fmt.Errorf("storage: prometheus %q: %w", name, err)
	}
	return prometheusResource{client: client, remoteWriteURL: remoteWriteURL}, nil
}

type bearerRoundTripper struct {
	token string
	next  http.RoundTripper
}

func (r bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	if r.token != "" {
		clone.Header.Set("Authorization", "Bearer "+r.token)
	}
	return r.next.RoundTrip(clone)
}
