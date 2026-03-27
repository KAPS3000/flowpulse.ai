package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// NUMATopology maps CPU core IDs to their NUMA node.
type NUMATopology struct {
	CoreToNUMA map[uint32]uint32
}

func DetectNUMATopology() *NUMATopology {
	topo := &NUMATopology{
		CoreToNUMA: make(map[uint32]uint32),
	}

	nodesDir := "/sys/devices/system/node"
	entries, err := os.ReadDir(nodesDir)
	if err != nil {
		log.Debug().Err(err).Msg("NUMA topology not available")
		return topo
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "node") {
			continue
		}
		nodeIDStr := strings.TrimPrefix(entry.Name(), "node")
		nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
		if err != nil {
			continue
		}

		cpulistPath := filepath.Join(nodesDir, entry.Name(), "cpulist")
		data, err := os.ReadFile(cpulistPath)
		if err != nil {
			continue
		}

		cpus := parseCPUList(strings.TrimSpace(string(data)))
		for _, cpu := range cpus {
			topo.CoreToNUMA[cpu] = uint32(nodeID)
		}
	}

	log.Info().Int("cores", len(topo.CoreToNUMA)).Msg("detected NUMA topology")
	return topo
}

// parseCPUList parses a CPU list string like "0-3,8-11" into individual CPU IDs.
func parseCPUList(list string) []uint32 {
	var cpus []uint32
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, "-"); idx >= 0 {
			start, _ := strconv.ParseUint(part[:idx], 10, 32)
			end, _ := strconv.ParseUint(part[idx+1:], 10, 32)
			for i := start; i <= end; i++ {
				cpus = append(cpus, uint32(i))
			}
		} else {
			v, _ := strconv.ParseUint(part, 10, 32)
			cpus = append(cpus, uint32(v))
		}
	}
	return cpus
}
