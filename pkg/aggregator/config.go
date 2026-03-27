package aggregator

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Aggregator struct {
		GRPCListen    string        `yaml:"grpc_listen"`
		ShardID       int           `yaml:"shard_id"`
		BatchSize     int           `yaml:"batch_size"`
		FlushInterval time.Duration `yaml:"flush_interval"`
	} `yaml:"aggregator"`

	ClickHouse struct {
		DSNEnv       string `yaml:"dsn_env"`
		Database     string `yaml:"database"`
		MaxOpenConns int    `yaml:"max_open_conns"`
		MaxIdleConns int    `yaml:"max_idle_conns"`
	} `yaml:"clickhouse"`

	NATS struct {
		URLEnv     string `yaml:"url_env"`
		Stream     string `yaml:"stream"`
		MaxPending int    `yaml:"max_pending"`
	} `yaml:"nats"`

	Log struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"log"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Aggregator.GRPCListen == "" {
		cfg.Aggregator.GRPCListen = ":9091"
	}
	if cfg.Aggregator.BatchSize == 0 {
		cfg.Aggregator.BatchSize = 10000
	}
	if cfg.Aggregator.FlushInterval == 0 {
		cfg.Aggregator.FlushInterval = 5 * time.Second
	}
	if cfg.ClickHouse.Database == "" {
		cfg.ClickHouse.Database = "flowpulse"
	}
	if cfg.ClickHouse.MaxOpenConns == 0 {
		cfg.ClickHouse.MaxOpenConns = 10
	}
	if cfg.NATS.Stream == "" {
		cfg.NATS.Stream = "flowpulse"
	}
	if cfg.NATS.MaxPending == 0 {
		cfg.NATS.MaxPending = 65536
	}

	return cfg, nil
}

func (c *Config) ClickHouseDSN() string {
	if c.ClickHouse.DSNEnv != "" {
		if dsn := os.Getenv(c.ClickHouse.DSNEnv); dsn != "" {
			return dsn
		}
	}
	return "clickhouse://localhost:9000/flowpulse"
}

func (c *Config) NATSURL() string {
	if c.NATS.URLEnv != "" {
		if url := os.Getenv(c.NATS.URLEnv); url != "" {
			return url
		}
	}
	return "nats://localhost:4222"
}
