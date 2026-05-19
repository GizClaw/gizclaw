package gizrun

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

type envConfig map[string]string

type gizrunConfig struct {
	DebugPort *int   `yaml:"debug_port"`
	Addr      string `yaml:"addr"`
	Pprof     *bool  `yaml:"pprof"`
	Metrics   *struct {
		Push *metricsPushConfig `yaml:"push"`
	} `yaml:"metrics"`
}

type volcLogConfig struct {
	Enabled bool `yaml:"enabled"`

	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
	SecurityToken   string `yaml:"security_token"`
	APIKey          string `yaml:"api_key"`

	TopicID   string `yaml:"topic_id"`
	Source    string `yaml:"source"`
	FileName  string `yaml:"file_name"`
	ShardHash string `yaml:"shard_hash"`

	Level            slog.Leveler
	AddSource        bool `yaml:"add_source"`
	EnableNanosecond bool `yaml:"enable_nanosecond"`
}

func (c envConfig) apply() error {
	if len(c) == 0 {
		return nil
	}
	values := make(map[string]string, len(c))
	for key, value := range c {
		key = strings.TrimSpace(key)
		if key == "" {
			return errors.New("gizrun env: key is empty")
		}
		values[key] = os.ExpandEnv(value)
	}
	for key, value := range values {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("gizrun env: set %s: %w", key, err)
		}
	}
	return nil
}

func (c volcLogConfig) withLevel(level slog.Leveler) volcLogConfig {
	c.Level = level
	return c
}

func (c volcLogConfig) expandEnv() volcLogConfig {
	return volcLogConfig{
		Enabled:          c.Enabled,
		Endpoint:         os.ExpandEnv(c.Endpoint),
		Region:           os.ExpandEnv(c.Region),
		AccessKeyID:      os.ExpandEnv(c.AccessKeyID),
		AccessKeySecret:  os.ExpandEnv(c.AccessKeySecret),
		SecurityToken:    os.ExpandEnv(c.SecurityToken),
		APIKey:           os.ExpandEnv(c.APIKey),
		TopicID:          os.ExpandEnv(c.TopicID),
		Source:           os.ExpandEnv(c.Source),
		FileName:         os.ExpandEnv(c.FileName),
		ShardHash:        os.ExpandEnv(c.ShardHash),
		Level:            c.Level,
		AddSource:        c.AddSource,
		EnableNanosecond: c.EnableNanosecond,
	}
}

func parseConfig[T any](data []byte) (any, error) {
	var config T
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return config, nil
}
