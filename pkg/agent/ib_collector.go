package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/flowpulse/flowpulse/pkg/model"
	"github.com/rs/zerolog/log"
)

const sysfsIBPath = "/sys/class/infiniband"

// IBCollector reads InfiniBand port counters from sysfs.
type IBCollector struct{}

func NewIBCollector() *IBCollector {
	return &IBCollector{}
}

// CollectAll enumerates all IB devices and ports, returning metrics for each.
func (c *IBCollector) CollectAll() []model.IBPortMetrics {
	entries, err := os.ReadDir(sysfsIBPath)
	if err != nil {
		log.Debug().Err(err).Msg("no InfiniBand devices found in sysfs")
		return nil
	}

	var allMetrics []model.IBPortMetrics
	for _, dev := range entries {
		if !dev.IsDir() {
			continue
		}
		portsDir := filepath.Join(sysfsIBPath, dev.Name(), "ports")
		ports, err := os.ReadDir(portsDir)
		if err != nil {
			continue
		}
		for _, port := range ports {
			m := c.collectPort(dev.Name(), port.Name())
			if m != nil {
				allMetrics = append(allMetrics, *m)
			}
		}
	}
	return allMetrics
}

func (c *IBCollector) collectPort(device, port string) *model.IBPortMetrics {
	countersDir := filepath.Join(sysfsIBPath, device, "ports", port, "counters")
	hw := filepath.Join(sysfsIBPath, device, "ports", port, "hw_counters")

	m := &model.IBPortMetrics{
		PortName: fmt.Sprintf("%s/%s", device, port),
	}

	m.TxBytes = readCounter(countersDir, "port_xmit_data") * 4 // IB counts in 32-bit words
	m.RxBytes = readCounter(countersDir, "port_rcv_data") * 4
	m.TxPackets = readCounter(countersDir, "port_xmit_packets")
	m.RxPackets = readCounter(countersDir, "port_rcv_packets")
	m.SymbolErrors = readCounter(countersDir, "symbol_error_counter")
	m.LinkRecoveries = readCounter(countersDir, "link_error_recovery_counter")
	m.CRCErrors = readCounter(countersDir, "port_rcv_remote_physical_errors")
	m.PortRcvErrors = readCounter(countersDir, "port_rcv_errors")

	// hw_counters may have additional useful counters
	if _, err := os.Stat(hw); err == nil {
		ecnMarked := readCounter(hw, "np_ecn_marked_roce_packets")
		cnpSent := readCounter(hw, "np_cnp_sent")
		_ = ecnMarked
		_ = cnpSent
	}

	// Read link speed to calculate utilization
	ratePath := filepath.Join(sysfsIBPath, device, "ports", port, "rate")
	if rateBytes, err := os.ReadFile(ratePath); err == nil {
		rate := strings.TrimSpace(string(rateBytes))
		speedGbps := parseIBRate(rate)
		if speedGbps > 0 {
			// Rough utilization: actual bytes/sec vs theoretical max
			// This is a snapshot; the agent computes delta over poll intervals
			peakBytesPerSec := float64(speedGbps) * 1e9 / 8
			if peakBytesPerSec > 0 {
				m.LinkUtilizationPct = float64(m.TxBytes+m.RxBytes) / peakBytesPerSec * 100
			}
		}
	}

	return m
}

func readCounter(dir, name string) uint64 {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return 0
	}
	val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// parseIBRate parses rate strings like "100 Gb/sec (4X HDR)" -> 100
func parseIBRate(rate string) float64 {
	parts := strings.Fields(rate)
	if len(parts) < 2 {
		return 0
	}
	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	return val
}

// QP state tracking structures
type QPState struct {
	QPNumber    uint32
	Port        uint32
	SendCount   uint64
	RecvCount   uint64
	SendBytes   uint64
	RecvBytes   uint64
	CompLatNs   uint64
	CompCount   uint64
	ErrorCount  uint64
	LastOpNs    uint64
}

// BPFIBOpKey mirrors struct ib_op_key in common.h
type BPFIBOpKey struct {
	QPNumber uint32
	Port     uint32
}

// BPFIBOpValue mirrors struct ib_op_value in common.h
type BPFIBOpValue struct {
	SendCount      uint64
	RecvCount      uint64
	SendBytes      uint64
	RecvBytes      uint64
	CompletionCount uint64
	CompletionLatNs uint64
	ErrorCount     uint64
	LastOpNs       uint64
}

// ReadQPStats reads QP-level statistics from the eBPF ib_qp_stats map.
func ReadQPStats(ibQPStats interface{ Iterate() *mapIterator }) []QPState {
	// This would iterate the eBPF map if available.
	// Placeholder for when ib_verbs.o is loaded.
	return nil
}

// mapIterator is a minimal interface matching ebpf.Map.Iterate()
type mapIterator struct{}
