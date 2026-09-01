package storage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

const redisReadinessTimeout = 5 * time.Second

// Redis returns the named physical Redis client. The Storage registry owns
// the client and closes it with the other physical resources.
func (s *Storage) Redis(name string) (*redis.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	client, ok := s.redis[name]
	if !ok {
		return nil, fmt.Errorf("storage: redis %q not found", name)
	}
	return client, nil
}

func newRedis(name string, cfg RedisConfig) (*redis.Client, error) {
	options, err := redisOptions(name, cfg)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), redisReadinessTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, &externalOperationError{
			operation: fmt.Sprintf("storage: redis %q ping", name),
			err:       err,
		}
	}
	return client, nil
}

// ValidateRedisURL applies the same single-node URL and TLS requirements used
// when Storage opens Redis, without reading CA files or connecting to Redis.
func ValidateRedisURL(raw string) error {
	_, err := parseRedisURL(raw)
	return err
}

func parseRedisURL(raw string) (*redis.Options, error) {
	parsed, err := url.Parse(raw)
	if raw == "" || err != nil || parsed.Scheme != "redis" && parsed.Scheme != "rediss" ||
		parsed.Hostname() == "" || strings.Contains(parsed.Hostname(), ",") {
		return nil, errors.New("invalid single-endpoint Redis URL")
	}
	options, err := redis.ParseURL(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "rediss" {
		if options.TLSConfig == nil {
			options.TLSConfig = &tls.Config{}
		}
		if options.TLSConfig.InsecureSkipVerify {
			return nil, errors.New("certificate verification must remain enabled")
		}
		options.TLSConfig.MinVersion = tls.VersionTLS12
	}
	return options, nil
}

func redisOptions(name string, cfg RedisConfig) (*redis.Options, error) {
	options, err := parseRedisURL(cfg.URL)
	if err != nil {
		return nil, redisConfigError(name, "parse url", err)
	}
	if cfg.TLSCAFile == "" {
		return options, nil
	}
	if options.TLSConfig == nil {
		return nil, redisConfigError(name, "configure tls", errors.New("tls_ca_file requires a rediss URL"))
	}
	caPEM, err := os.ReadFile(cfg.TLSCAFile)
	if err != nil {
		return nil, redisConfigError(name, "read tls ca file", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, redisConfigError(name, "parse tls ca file", errors.New("no PEM certificates found"))
	}
	options.TLSConfig.RootCAs = roots
	return options, nil
}

func redisConfigError(name, operation string, err error) error {
	return &externalOperationError{
		operation: fmt.Sprintf("storage: redis %q %s", name, operation),
		err:       err,
	}
}
