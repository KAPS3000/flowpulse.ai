package agent

import (
	"sync"
	"time"

	"github.com/flowpulse/flowpulse/pkg/model"
	"github.com/google/uuid"
)

// FlowTable maintains a local cache of active flows with LRU aging.
type FlowTable struct {
	mu       sync.RWMutex
	flows    map[model.FlowKey]*model.Flow
	timeout  time.Duration
	maxFlows int
}

func NewFlowTable(timeout time.Duration, maxFlows int) *FlowTable {
	return &FlowTable{
		flows:    make(map[model.FlowKey]*model.Flow, 4096),
		timeout:  timeout,
		maxFlows: maxFlows,
	}
}

func (ft *FlowTable) Update(key BPFFlowKey, val BPFFlowValue, nodeID, tenantID string) {
	mkey := model.FlowKey{
		SrcIP:    key.SrcIP,
		DstIP:    key.DstIP,
		SrcPort:  key.SrcPort,
		DstPort:  key.DstPort,
		Protocol: key.Protocol,
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()

	f, exists := ft.flows[mkey]
	if !exists {
		f = &model.Flow{
			Key:      mkey,
			FlowID:   uuid.New().String(),
			TenantID: tenantID,
			NodeID:   nodeID,
		}
		ft.flows[mkey] = f
	}

	f.Packets = val.Packets
	f.Bytes = val.Bytes
	f.FirstSeen = time.Unix(0, int64(val.FirstSeenNs))
	f.LastSeen = time.Unix(0, int64(val.LastSeenNs))
	f.Direction = model.FlowDirection(val.Direction)

	if val.RDMAQp != 0 {
		f.RDMA = &model.RDMAInfo{
			QPNumber:        val.RDMAQp,
			DestQP:          val.RDMADestQp,
			RDMAMsgRate:     val.RDMAMsgCount,
			Retransmissions: val.RDMARetransmits,
			ECNMarks:        val.RDMAEcnMarks,
			CNPCount:        val.RDMACnpCount,
		}
	}
}

// Drain returns all current flows and resets the table.
// Used for batch sending to the aggregator.
func (ft *FlowTable) Drain() []model.Flow {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	flows := make([]model.Flow, 0, len(ft.flows))
	for _, f := range ft.flows {
		flows = append(flows, *f)
	}

	return flows
}

// EvictStale removes flows older than the configured timeout.
func (ft *FlowTable) EvictStale(now time.Time) int {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	cutoff := now.Add(-ft.timeout)
	evicted := 0
	for key, f := range ft.flows {
		if f.LastSeen.Before(cutoff) {
			delete(ft.flows, key)
			evicted++
		}
	}
	return evicted
}

func (ft *FlowTable) Len() int {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	return len(ft.flows)
}
