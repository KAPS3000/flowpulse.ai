package aggregator

import (
	"sync"
	"time"

	"github.com/flowpulse/flowpulse/pkg/model"
)

// FlowCorrelator merges unidirectional flows into bidirectional conversations.
type FlowCorrelator struct {
	mu    sync.RWMutex
	flows map[correlationKey]*model.CorrelatedFlow
}

type correlationKey struct {
	// Canonical key: lower IP first to match forward/reverse
	ipA, ipB     uint32
	portA, portB uint16
	protocol     uint8
}

func makeCorrelationKey(k model.FlowKey) correlationKey {
	if k.SrcIP < k.DstIP || (k.SrcIP == k.DstIP && k.SrcPort < k.DstPort) {
		return correlationKey{
			ipA: k.SrcIP, ipB: k.DstIP,
			portA: k.SrcPort, portB: k.DstPort,
			protocol: k.Protocol,
		}
	}
	return correlationKey{
		ipA: k.DstIP, ipB: k.SrcIP,
		portA: k.DstPort, portB: k.SrcPort,
		protocol: k.Protocol,
	}
}

func NewFlowCorrelator() *FlowCorrelator {
	return &FlowCorrelator{
		flows: make(map[correlationKey]*model.CorrelatedFlow, 4096),
	}
}

func (fc *FlowCorrelator) Ingest(flow model.Flow) *model.CorrelatedFlow {
	ck := makeCorrelationKey(flow.Key)

	fc.mu.Lock()
	defer fc.mu.Unlock()

	cf, exists := fc.flows[ck]
	if !exists {
		cf = &model.CorrelatedFlow{
			FlowID:   flow.FlowID,
			TenantID: flow.TenantID,
		}
		fc.flows[ck] = cf
	}

	isForward := flow.Key.SrcIP == ck.ipA && flow.Key.SrcPort == ck.portA
	if isForward {
		cf.Forward = &flow
	} else {
		cf.Reverse = &flow
	}

	// Aggregate stats
	cf.TotalBytes = 0
	cf.TotalPkts = 0
	cf.FirstSeen = flow.FirstSeen
	cf.LastSeen = flow.LastSeen

	if cf.Forward != nil {
		cf.TotalBytes += cf.Forward.Bytes
		cf.TotalPkts += cf.Forward.Packets
		if cf.Forward.FirstSeen.Before(cf.FirstSeen) {
			cf.FirstSeen = cf.Forward.FirstSeen
		}
		if cf.Forward.LastSeen.After(cf.LastSeen) {
			cf.LastSeen = cf.Forward.LastSeen
		}
		if cf.Forward.RDMA != nil {
			cf.RDMA = cf.Forward.RDMA
		}
	}
	if cf.Reverse != nil {
		cf.TotalBytes += cf.Reverse.Bytes
		cf.TotalPkts += cf.Reverse.Packets
		if cf.Reverse.FirstSeen.Before(cf.FirstSeen) {
			cf.FirstSeen = cf.Reverse.FirstSeen
		}
		if cf.Reverse.LastSeen.After(cf.LastSeen) {
			cf.LastSeen = cf.Reverse.LastSeen
		}
	}

	return cf
}

// DrainAll returns all correlated flows and clears internal state.
func (fc *FlowCorrelator) DrainAll() []*model.CorrelatedFlow {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	result := make([]*model.CorrelatedFlow, 0, len(fc.flows))
	for _, cf := range fc.flows {
		result = append(result, cf)
	}
	return result
}

// EvictStale removes flows not seen since cutoff.
func (fc *FlowCorrelator) EvictStale(cutoff time.Time) int {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	evicted := 0
	for k, cf := range fc.flows {
		if cf.LastSeen.Before(cutoff) {
			delete(fc.flows, k)
			evicted++
		}
	}
	return evicted
}

func (fc *FlowCorrelator) Len() int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return len(fc.flows)
}
