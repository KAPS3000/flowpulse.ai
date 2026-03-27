package tenant

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// Resolver maps cgroup IDs (from K8s pods or systemd slices) to tenant IDs.
type Resolver struct {
	mu       sync.RWMutex
	mappings map[uint64]string   // cgroup_id -> tenant_id
	patterns map[string]string   // cgroup path pattern -> tenant_id
	config   *ResolverConfig
}

type ResolverConfig struct {
	DefaultTenant string            `yaml:"default_tenant"`
	CgroupMap     map[string]string `yaml:"cgroup_map"` // cgroup path prefix -> tenant_id
}

func NewResolver(configPath string) (*Resolver, error) {
	r := &Resolver{
		mappings: make(map[uint64]string),
		patterns: make(map[string]string),
	}

	if configPath != "" {
		cfg, err := loadResolverConfig(configPath)
		if err != nil {
			log.Warn().Err(err).Msg("tenant resolver config not found, using defaults")
			r.config = &ResolverConfig{DefaultTenant: "default"}
		} else {
			r.config = cfg
			for pattern, tenantID := range cfg.CgroupMap {
				r.patterns[pattern] = tenantID
			}
		}
	} else {
		r.config = &ResolverConfig{DefaultTenant: "default"}
	}

	return r, nil
}

func loadResolverConfig(path string) (*ResolverConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tenant config: %w", err)
	}
	cfg := &ResolverConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse tenant config: %w", err)
	}
	if cfg.DefaultTenant == "" {
		cfg.DefaultTenant = "default"
	}
	return cfg, nil
}

// ResolveByID looks up a tenant ID by cgroup ID (numeric).
func (r *Resolver) ResolveByID(cgroupID uint64) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if tenantID, ok := r.mappings[cgroupID]; ok {
		return tenantID
	}
	return r.config.DefaultTenant
}

// ResolveByPath matches a cgroup path against configured patterns.
// For K8s: /kubepods/pod<uid>/container<id>
// For systemd: /system.slice/myapp.service
func (r *Resolver) ResolveByPath(cgroupPath string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for pattern, tenantID := range r.patterns {
		if strings.HasPrefix(cgroupPath, pattern) || strings.Contains(cgroupPath, pattern) {
			return tenantID
		}
	}
	return r.config.DefaultTenant
}

// Register adds a cgroup ID -> tenant ID mapping (for dynamic registration).
func (r *Resolver) Register(cgroupID uint64, tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mappings[cgroupID] = tenantID
}

// RegisterPattern adds a cgroup path pattern -> tenant mapping.
func (r *Resolver) RegisterPattern(pattern, tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patterns[pattern] = tenantID
}
