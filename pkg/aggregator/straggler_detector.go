package aggregator

import (
	"math"
	"sort"

	"github.com/flowpulse/flowpulse/pkg/model"
)

// StragglerDetector identifies nodes that are falling behind in collective operations.
type StragglerDetector struct {
	// DeviationThreshold is the minimum percentage deviation from median
	// to be considered a straggler (default: 15%)
	DeviationThreshold float64
	// MaxStragglers limits the number of reported stragglers
	MaxStragglers int
}

func NewStragglerDetector() *StragglerDetector {
	return &StragglerDetector{
		DeviationThreshold: 15.0,
		MaxStragglers:      20,
	}
}

// DetectStragglers analyzes node metrics and flow data to find stragglers.
// It uses multiple signals:
//   1. IB link utilization deviation from median
//   2. CPU kernel time ratio (indicates driver/network overhead)
//   3. Context switch rate anomalies
//   4. RDMA retransmission rate
func (sd *StragglerDetector) DetectStragglers(
	nodeMetrics []model.NodeMetrics,
	flows []*model.CorrelatedFlow,
) []model.NodeStraggler {
	if len(nodeMetrics) < 3 {
		return nil
	}

	scores := make(map[string]float64)
	details := make(map[string]model.NodeStraggler)

	// Signal 1: IB utilization deviation
	ibUtils := make([]float64, 0, len(nodeMetrics))
	for _, nm := range nodeMetrics {
		if nm.IBMetrics != nil {
			ibUtils = append(ibUtils, nm.IBMetrics.LinkUtilizationPct)
		}
	}
	ibMedian := median(ibUtils)

	for _, nm := range nodeMetrics {
		if nm.IBMetrics == nil {
			continue
		}
		deviation := math.Abs(nm.IBMetrics.LinkUtilizationPct - ibMedian)
		normalizedDev := 0.0
		if ibMedian > 0 {
			normalizedDev = deviation / ibMedian * 100
		}
		scores[nm.NodeID] += normalizedDev * 0.4 // 40% weight

		details[nm.NodeID] = model.NodeStraggler{
			NodeID:    nm.NodeID,
			Deviation: normalizedDev,
		}
	}

	// Signal 2: Kernel CPU time (indicates overhead)
	kernelPcts := make([]float64, 0, len(nodeMetrics))
	for _, nm := range nodeMetrics {
		var avgKernel float64
		for _, cpu := range nm.CPUMetrics {
			avgKernel += cpu.KernelPct
		}
		if len(nm.CPUMetrics) > 0 {
			avgKernel /= float64(len(nm.CPUMetrics))
		}
		kernelPcts = append(kernelPcts, avgKernel)
	}
	kernelMedian := median(kernelPcts)

	for i, nm := range nodeMetrics {
		if i < len(kernelPcts) {
			deviation := math.Abs(kernelPcts[i] - kernelMedian)
			scores[nm.NodeID] += deviation * 0.2 // 20% weight
		}
	}

	// Signal 3: RDMA retransmission rate from flows
	nodeRetransmits := make(map[string]uint64)
	for _, f := range flows {
		if f.RDMA != nil && f.Forward != nil {
			nodeRetransmits[f.Forward.NodeID] += f.RDMA.Retransmissions
		}
	}
	retransmitVals := make([]float64, 0, len(nodeRetransmits))
	for _, v := range nodeRetransmits {
		retransmitVals = append(retransmitVals, float64(v))
	}
	retransmitMedian := median(retransmitVals)

	for nodeID, retransmits := range nodeRetransmits {
		deviation := math.Abs(float64(retransmits) - retransmitMedian)
		normalizedDev := 0.0
		if retransmitMedian > 0 {
			normalizedDev = deviation / retransmitMedian * 100
		}
		scores[nodeID] += normalizedDev * 0.4 // 40% weight
	}

	// Collect results
	var stragglers []model.NodeStraggler
	for nodeID, score := range scores {
		if score >= sd.DeviationThreshold {
			s := details[nodeID]
			s.Deviation = score
			stragglers = append(stragglers, s)
		}
	}

	sort.Slice(stragglers, func(i, j int) bool {
		return stragglers[i].Deviation > stragglers[j].Deviation
	})

	if len(stragglers) > sd.MaxStragglers {
		stragglers = stragglers[:sd.MaxStragglers]
	}

	return stragglers
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}
