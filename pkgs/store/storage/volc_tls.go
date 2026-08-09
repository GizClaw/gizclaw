package storage

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/volcengine/volc-sdk-golang/service/tls"
)

// VolcTLS returns the named physical Volc TLS SDK client.
func (s *Storage) VolcTLS(name string) (tls.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	connector, ok := s.volcs[name]
	if !ok {
		return nil, fmt.Errorf("storage: volc-tls %q not found", name)
	}
	return connector, nil
}

func newVolcTLS(name string, cfg VolcTLSConfig) (tls.Client, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	region := strings.TrimSpace(cfg.Region)
	accessKeyID := strings.TrimSpace(cfg.AccessKeyID)
	accessKeySecret := strings.TrimSpace(cfg.AccessKeySecret)
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "endpoint", value: endpoint},
		{name: "region", value: region},
		{name: "access_key_id", value: accessKeyID},
		{name: "access_key_secret", value: accessKeySecret},
	} {
		if field.value == "" {
			return nil, fmt.Errorf("storage: volc-tls %q requires %s", name, field.name)
		}
	}
	client := tls.NewClient(endpoint, accessKeyID, accessKeySecret, "", region)
	client.SetTimeout(30 * time.Second)
	retryPolicy := client.GetRetryPolicy()
	retryPolicy.TotalTimeout = 30 * time.Second
	client.SetRetryPolicy(retryPolicy)
	return client, nil
}
