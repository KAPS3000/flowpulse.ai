package agent

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	NodeID   string `yaml:"node_id"`
	TenantID string `yaml:"tenant_id"`

	EBPF struct {
		PollInterval time.Duration `yaml:"poll_interval"`
		FlowTimeout  time.Duration `yaml:"flow_timeout"`
		MaxFlows     int           `yaml:"max_flows"`
		Interfaces   []string      `yaml:"interfaces"`
		BPFObjectDir string        `yaml:"bpf_object_dir"`
	} `yaml:"ebpf"`

	Aggregator struct {
		Address       string        `yaml:"address"`
		BatchSize     int           `yaml:"batch_size"`
		FlushInterval time.Duration `yaml:"flush_interval"`
		MaxRetry      int           `yaml:"max_retry"`
	} `yaml:"aggregator"`

	Health struct {
		Listen string `yaml:"listen"`
	} `yaml:"health"`

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

	if cfg.NodeID == "" {
		hostname, _ := os.Hostname()
		cfg.NodeID = hostname
	}
	if cfg.TenantID == "" {
		cfg.TenantID = "default"
	}
	if cfg.EBPF.PollInterval == 0 {
		cfg.EBPF.PollInterval = time.Second
	}
	if cfg.EBPF.FlowTimeout == 0 {
		cfg.EBPF.FlowTimeout = 30 * time.Second
	}
	if cfg.EBPF.MaxFlows == 0 {
		cfg.EBPF.MaxFlows = 1000000
	}
	if cfg.EBPF.BPFObjectDir == "" {
		cfg.EBPF.BPFObjectDir = "/bpf"
	}
	if cfg.Aggregator.Address == "" {
		cfg.Aggregator.Address = "localhost:9091"
	}
	if cfg.Aggregator.BatchSize == 0 {
		cfg.Aggregator.BatchSize = 1000
	}
	if cfg.Aggregator.FlushInterval == 0 {
		cfg.Aggregator.FlushInterval = time.Second
	}
	if cfg.Aggregator.MaxRetry == 0 {
		cfg.Aggregator.MaxRetry = 3
	}
	if cfg.Health.Listen == "" {
		cfg.Health.Listen = ":8081"
	}

	return cfg, nil
}
