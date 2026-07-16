package logging

import (
	"fmt"
	"os"
	"strings"
)

const (
	SinkStderr = "stderr"
	SinkStore  = "store"
)

// Config controls process system logging independently from product LogStore writes.
type Config struct {
	Level      string       `yaml:"level"`
	QueryStore string       `yaml:"query_store"`
	Sinks      []SinkConfig `yaml:"sinks"`
}

// SinkConfig selects one ordered process-log destination.
type SinkConfig struct {
	Kind  string `yaml:"kind"`
	Store string `yaml:"store,omitempty"`
	Level string `yaml:"level,omitempty"`
}

// DefaultConfig returns the credential-free stderr process logger.
func DefaultConfig() Config {
	return Config{Level: "info", Sinks: []SinkConfig{{Kind: SinkStderr}}}
}

// IsZero reports whether no system logging fields were explicitly set.
func (c Config) IsZero() bool {
	return strings.TrimSpace(c.Level) == "" && strings.TrimSpace(c.QueryStore) == "" && c.Sinks == nil
}

// PrepareConfig applies defaults, expands environment variables, and validates
// the system logging config.
func PrepareConfig(cfg Config) (Config, error) {
	if cfg.Sinks == nil {
		cfg.Sinks = append([]SinkConfig(nil), DefaultConfig().Sinks...)
	} else {
		cfg.Sinks = append([]SinkConfig(nil), cfg.Sinks...)
		if len(cfg.Sinks) == 0 {
			return Config{}, fmt.Errorf("system_log.sinks must contain at least one sink")
		}
	}
	if strings.TrimSpace(cfg.Level) == "" {
		cfg.Level = DefaultConfig().Level
	} else {
		cfg.Level = strings.TrimSpace(os.ExpandEnv(cfg.Level))
	}
	if _, err := ParseLevel(cfg.Level); err != nil {
		return Config{}, fmt.Errorf("system_log.level: %w", err)
	}
	cfg.QueryStore = strings.TrimSpace(os.ExpandEnv(cfg.QueryStore))
	seen := make(map[string]struct{}, len(cfg.Sinks))
	storeSinks := make(map[string]struct{})
	for index := range cfg.Sinks {
		sink := &cfg.Sinks[index]
		sink.Kind = strings.TrimSpace(os.ExpandEnv(sink.Kind))
		sink.Store = strings.TrimSpace(os.ExpandEnv(sink.Store))
		sink.Level = strings.TrimSpace(os.ExpandEnv(sink.Level))
		if sink.Level == "" {
			sink.Level = cfg.Level
		}
		if _, err := ParseLevel(sink.Level); err != nil {
			return Config{}, fmt.Errorf("system_log.sinks[%d].level: %w", index, err)
		}
		key := sink.Kind
		switch sink.Kind {
		case SinkStderr:
			if sink.Store != "" {
				return Config{}, fmt.Errorf("system_log.sinks[%d].store is invalid for stderr", index)
			}
		case SinkStore:
			if sink.Store == "" {
				return Config{}, fmt.Errorf("system_log.sinks[%d].store is required", index)
			}
			key += ":" + sink.Store
			storeSinks[sink.Store] = struct{}{}
		default:
			return Config{}, fmt.Errorf("system_log.sinks[%d].kind must be stderr or store", index)
		}
		if _, duplicate := seen[key]; duplicate {
			return Config{}, fmt.Errorf("system_log.sinks[%d] duplicates %q", index, key)
		}
		seen[key] = struct{}{}
	}
	if cfg.QueryStore != "" {
		if _, exists := storeSinks[cfg.QueryStore]; !exists {
			return Config{}, fmt.Errorf("system_log.query_store %q must reference a configured store sink", cfg.QueryStore)
		}
	}
	return cfg, nil
}
