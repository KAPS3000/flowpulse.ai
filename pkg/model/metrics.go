package model

import "time"

type CPUMetric struct {
	CoreID          uint32  `json:"core_id"`
	NUMANode        uint32  `json:"numa_node"`
	Utilization     float64 `json:"utilization"`
	KernelPct       float64 `json:"kernel_pct"`
	UserPct         float64 `json:"user_pct"`
	SoftIRQPct      float64 `json:"softirq_pct"`
	ContextSwitches uint64  `json:"context_switches"`
	InvoluntaryCS   uint64  `json:"involuntary_cs"`
}

type IBPortMetrics struct {
	PortName          string  `json:"port_name"`
	TxBytes           uint64  `json:"tx_bytes"`
	RxBytes           uint64  `json:"rx_bytes"`
	TxPackets         uint64  `json:"tx_packets"`
	RxPackets         uint64  `json:"rx_packets"`
	SymbolErrors      uint64  `json:"symbol_errors"`
	LinkRecoveries    uint64  `json:"link_recoveries"`
	CRCErrors         uint64  `json:"crc_errors"`
	PortRcvErrors     uint64  `json:"port_rcv_errors"`
	ActiveQPs         uint32  `json:"active_qps"`
	LinkUtilizationPct float64 `json:"link_utilization_pct"`
}

type NodeMetrics struct {
	NodeID     string        `json:"node_id"`
	TenantID   string        `json:"tenant_id"`
	CPUMetrics []CPUMetric   `json:"cpu_metrics"`
	IBMetrics  *IBPortMetrics `json:"ib_metrics,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

type TrainingMetrics struct {
	TenantID                string           `json:"tenant_id"`
	StragglerScore          float64          `json:"straggler_score"`
	BubbleRatio             float64          `json:"bubble_ratio"`
	GradientSyncOverheadPct float64          `json:"gradient_sync_overhead_pct"`
	NetworkSaturationIndex  float64          `json:"network_saturation_index"`
	ImbalanceScore          float64          `json:"imbalance_score"`
	Stragglers              []NodeStraggler  `json:"stragglers"`
	Timestamp               time.Time        `json:"timestamp"`
}

type NodeStraggler struct {
	NodeID    string  `json:"node_id"`
	Deviation float64 `json:"deviation"`
	LatencyP99 float64 `json:"latency_p99"`
}
