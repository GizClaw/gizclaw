package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	if cfg.DSN == "" {
		return nil, fmt.Errorf("storage: redis %q requires dsn", name)
	}
	parsed, err := url.Parse(cfg.DSN)
	if err != nil || parsed.Scheme != "redis" && parsed.Scheme != "rediss" || parsed.Hostname() == "" || strings.Contains(parsed.Hostname(), ",") {
		return nil, &externalOperationError{
			operation: fmt.Sprintf("storage: redis %q parse dsn", name),
			err:       errors.New("invalid single-endpoint Redis DSN"),
		}
	}
	options, err := redis.ParseURL(cfg.DSN)
	if err != nil {
		return nil, &externalOperationError{
			operation: fmt.Sprintf("storage: redis %q parse dsn", name),
			err:       err,
		}
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
