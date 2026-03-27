package api

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	API struct {
		RESTListen string `yaml:"rest_listen"`
		GRPCListen string `yaml:"grpc_listen"`
		WSPath     string `yaml:"ws_path"`
	} `yaml:"api"`

	Auth struct {
		JWTSecretEnv string        `yaml:"jwt_secret_env"`
		TokenExpiry  time.Duration `yaml:"token_expiry"`
	} `yaml:"auth"`

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

	Redis struct {
		AddrEnv  string `yaml:"addr_env"`
		DB       int    `yaml:"db"`
		PoolSize int    `yaml:"pool_size"`
	} `yaml:"redis"`

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
	if cfg.API.RESTListen == "" {
		cfg.API.RESTListen = ":8080"
	}
	if cfg.API.GRPCListen == "" {
		cfg.API.GRPCListen = ":9090"
	}
	if cfg.API.WSPath == "" {
		cfg.API.WSPath = "/ws"
	}
	if cfg.Auth.TokenExpiry == 0 {
		cfg.Auth.TokenExpiry = 24 * time.Hour
	}
	if cfg.ClickHouse.Database == "" {
		cfg.ClickHouse.Database = "flowpulse"
	}
	if cfg.NATS.Stream == "" {
		cfg.NATS.Stream = "flowpulse"
	}
	return cfg, nil
}

func (c *Config) JWTSecret() []byte {
	if c.Auth.JWTSecretEnv != "" {
		if s := os.Getenv(c.Auth.JWTSecretEnv); s != "" {
			return []byte(s)
		}
	}
	return []byte("flowpulse-dev-secret-change-in-production")
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
