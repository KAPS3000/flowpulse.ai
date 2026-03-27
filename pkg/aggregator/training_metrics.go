package aggregator

import (
	"math"
	"sort"
	"time"

	"github.com/flowpulse/flowpulse/pkg/model"
)

// MetricsComputer calculates training efficiency metrics from aggregated flow and node data.
type MetricsComputer struct {
	// Theoretical peak bandwidth per IB rail in bytes/sec.
	// NDR (400Gbps) = 50GB/s; HDR (200Gbps) = 25GB/s
	PeakBandwidthBps uint64
}

func NewMetricsComputer(peakBandwidthBps uint64) *MetricsComputer {
	if peakBandwidthBps == 0 {
		peakBandwidthBps = 50_000_000_000 // 400Gbps NDR default
	}
	return &MetricsComputer{PeakBandwidthBps: peakBandwidthBps}
}

// ComputeTrainingMetrics produces aggregate training metrics from per-node data.
func (mc *MetricsComputer) ComputeTrainingMetrics(
	tenantID string,
	nodeMetrics []model.NodeMetrics,
	flows []*model.CorrelatedFlow,
	window time.Duration,
) model.TrainingMetrics {
	tm := model.TrainingMetrics{
		TenantID:  tenantID,
		Timestamp: time.Now(),
	}

	if len(nodeMetrics) == 0 {
		return tm
	}

	tm.StragglerScore = mc.computeStragglerScore(nodeMetrics, &tm)
	tm.BubbleRatio = mc.computeBubbleRatio(nodeMetrics)
	tm.GradientSyncOverheadPct = mc.computeGradSyncOverhead(nodeMetrics)
	tm.NetworkSaturationIndex = mc.computeNetworkSaturation(flows, window)
	tm.ImbalanceScore = mc.computeImbalanceScore(flows)

	return tm
}

func (mc *MetricsComputer) computeStragglerScore(
	metrics []model.NodeMetrics,
	tm *model.TrainingMetrics,
) float64 {
	if len(metrics) == 0 {
		return 0
	}

	// Use IB link utilization as a proxy for collective completion time.
	utils := make([]float64, 0, len(metrics))
	for _, m := range metrics {
		if m.IBMetrics != nil {
			utils = append(utils, m.IBMetrics.LinkUtilizationPct)
		}
	}

	if len(utils) == 0 {
		return 0
	}

	sort.Float64s(utils)
	median := utils[len(utils)/2]

	var maxDeviation float64
	stragglers := make([]model.NodeStraggler, 0)

	for _, m := range metrics {
		if m.IBMetrics == nil {
			continue
		}
		deviation := math.Abs(m.IBMetrics.LinkUtilizationPct - median)
		if deviation > maxDeviation {
			maxDeviation = deviation
		}

		// Flag nodes deviating more than 20% from median
		if deviation > 20 {
			stragglers = append(stragglers, model.NodeStraggler{
				NodeID:    m.NodeID,
				Deviation: deviation,
			})
		}
	}

	// Sort stragglers by deviation (worst first)
	sort.Slice(stragglers, func(i, j int) bool {
		return stragglers[i].Deviation > stragglers[j].Deviation
	})

	if len(stragglers) > 10 {
		stragglers = stragglers[:10]
	}
	tm.Stragglers = stragglers

	// Normalize to 0-100
	if median > 0 {
		return maxDeviation / median * 100
	}
	return 0
}

func (mc *MetricsComputer) computeBubbleRatio(metrics []model.NodeMetrics) float64 {
	// Bubble ratio: fraction of total CPU time spent idle (off-CPU)
	// while network operations are pending.
	// High softirq + low user = GPU waiting on network = bubbles
	var totalSoftIRQ, totalUser float64
	count := 0

	for _, m := range metrics {
		for _, cpu := range m.CPUMetrics {
			totalSoftIRQ += cpu.SoftIRQPct
			totalUser += cpu.UserPct
			count++
		}
	}

	if count == 0 || (totalSoftIRQ+totalUser) == 0 {
		return 0
	}

	// Bubble ratio is the proportion of softirq time vs total active time
	return totalSoftIRQ / (totalSoftIRQ + totalUser) * 100
}

func (mc *MetricsComputer) computeGradSyncOverhead(metrics []model.NodeMetrics) float64 {
	// Gradient sync overhead: kernel CPU time as a proxy for NCCL collective time.
	var totalKernel, totalActive float64
	for _, m := range metrics {
		for _, cpu := range m.CPUMetrics {
			totalKernel += cpu.KernelPct
			totalActive += cpu.Utilization
		}
	}

	if totalActive == 0 {
		return 0
	}
	return totalKernel / totalActive * 100
}

func (mc *MetricsComputer) computeNetworkSaturation(
	flows []*model.CorrelatedFlow,
	window time.Duration,
) float64 {
	if len(flows) == 0 || window == 0 {
		return 0
	}

	var totalBytes uint64
	for _, f := range flows {
		totalBytes += f.TotalBytes
	}

	actualBps := float64(totalBytes) / window.Seconds()
	return actualBps / float64(mc.PeakBandwidthBps) * 100
}

func (mc *MetricsComputer) computeImbalanceScore(flows []*model.CorrelatedFlow) float64 {
	if len(flows) < 2 {
		return 0
	}

	byteCounts := make([]float64, len(flows))
	var total float64
	for i, f := range flows {
		byteCounts[i] = float64(f.TotalBytes)
		total += byteCounts[i]
	}

	if total == 0 {
		return 0
	}

	mean := total / float64(len(byteCounts))
	var variance float64
	for _, b := range byteCounts {
		diff := b - mean
		variance += diff * diff
	}
	variance /= float64(len(byteCounts))

	// Coefficient of variation (normalized standard deviation)
	stddev := math.Sqrt(variance)
	return stddev / mean * 100
}
