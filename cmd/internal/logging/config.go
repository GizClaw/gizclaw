package logging

import (
	"fmt"
	"os"
	"strings"
)

// Config is the minimal server logging configuration surface.
type Config struct {
	Level string     `yaml:"level"`
	Volc  VolcConfig `yaml:"volc"`
}

// VolcConfig configures optional Volc TLS log forwarding.
type VolcConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	TopicID         string `yaml:"topic_id"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
}

// DefaultConfig returns the default server logging config.
func DefaultConfig() Config {
	return Config{Level: "info"}
}

// IsZero reports whether no logging fields were explicitly set.
func (c Config) IsZero() bool {
	return strings.TrimSpace(c.Level) == "" && c.Volc == (VolcConfig{})
}

// PrepareConfig applies defaults, expands environment variables, and validates
// the public logging config.
func PrepareConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.Level) == "" {
		cfg.Level = DefaultConfig().Level
	} else {
		cfg.Level = strings.TrimSpace(os.ExpandEnv(cfg.Level))
	}
	cfg.Volc.Endpoint = strings.TrimSpace(os.ExpandEnv(cfg.Volc.Endpoint))
	cfg.Volc.Region = strings.TrimSpace(os.ExpandEnv(cfg.Volc.Region))
	cfg.Volc.TopicID = strings.TrimSpace(os.ExpandEnv(cfg.Volc.TopicID))
	cfg.Volc.AccessKeyID = strings.TrimSpace(os.ExpandEnv(cfg.Volc.AccessKeyID))
	cfg.Volc.AccessKeySecret = strings.TrimSpace(os.ExpandEnv(cfg.Volc.AccessKeySecret))
	if _, err := ParseLevel(cfg.Level); err != nil {
		return Config{}, fmt.Errorf("log.level: %w", err)
	}
	if cfg.Volc.Enabled {
		if cfg.Volc.Endpoint == "" {
			return Config{}, fmt.Errorf("log.volc.endpoint is required when log.volc.enabled is true")
		}
		if cfg.Volc.Region == "" {
			return Config{}, fmt.Errorf("log.volc.region is required when log.volc.enabled is true")
		}
		if cfg.Volc.TopicID == "" {
			return Config{}, fmt.Errorf("log.volc.topic_id is required when log.volc.enabled is true")
		}
		if cfg.Volc.AccessKeyID == "" {
			return Config{}, fmt.Errorf("log.volc.access_key_id is required when log.volc.enabled is true")
		}
		if cfg.Volc.AccessKeySecret == "" {
			return Config{}, fmt.Errorf("log.volc.access_key_secret is required when log.volc.enabled is true")
		}
	}
	return cfg, nil
}
