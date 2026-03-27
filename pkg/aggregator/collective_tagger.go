package aggregator

import "github.com/flowpulse/flowpulse/pkg/model"

// CollectiveType identifies the NCCL/MPI collective operation type.
type CollectiveType string

const (
	CollectiveUnknown      CollectiveType = "unknown"
	CollectiveAllReduce    CollectiveType = "all-reduce"
	CollectiveAllGather    CollectiveType = "all-gather"
	CollectiveReduceScatter CollectiveType = "reduce-scatter"
	CollectiveBroadcast    CollectiveType = "broadcast"
	CollectiveAllToAll     CollectiveType = "all-to-all"
)

// CollectiveTag holds the inferred collective operation type and confidence.
type CollectiveTag struct {
	Type       CollectiveType `json:"type"`
	Confidence float64        `json:"confidence"` // 0.0-1.0
}

// CollectiveTagger infers NCCL collective operations from flow patterns.
// Heuristics are based on message size distributions and QP group patterns:
//
//   - AllReduce: symmetric bidirectional traffic, moderate message sizes,
//     ring/tree topology patterns across nodes
//   - AllGather: small sends from each node, large aggregate receives
//   - ReduceScatter: inverse of AllGather
//   - Broadcast: one-to-many pattern (single source, many destinations)
//   - AllToAll: uniform all-pairs traffic
type CollectiveTagger struct {
	// Number of nodes expected in the training job
	ExpectedNodes int
}

func NewCollectiveTagger(expectedNodes int) *CollectiveTagger {
	if expectedNodes == 0 {
		expectedNodes = 8
	}
	return &CollectiveTagger{ExpectedNodes: expectedNodes}
}

// TagFlows analyzes a set of correlated flows and assigns collective tags.
func (ct *CollectiveTagger) TagFlows(flows []*model.CorrelatedFlow) map[string]CollectiveTag {
	tags := make(map[string]CollectiveTag, len(flows))

	if len(flows) == 0 {
		return tags
	}

	// Group flows by source node to analyze traffic patterns
	byNode := make(map[string][]*model.CorrelatedFlow)
	for _, f := range flows {
		if f.Forward != nil {
			byNode[f.Forward.NodeID] = append(byNode[f.Forward.NodeID], f)
		}
	}

	nodeCount := len(byNode)
	if nodeCount < 2 {
		return tags
	}

	for _, f := range flows {
		tag := ct.inferCollective(f, byNode, nodeCount)
		tags[f.FlowID] = tag
	}

	return tags
}

func (ct *CollectiveTagger) inferCollective(
	flow *model.CorrelatedFlow,
	byNode map[string][]*model.CorrelatedFlow,
	nodeCount int,
) CollectiveTag {
	// Bidirectional with roughly symmetric traffic -> AllReduce
	if flow.Forward != nil && flow.Reverse != nil {
		ratio := symmetryRatio(flow)
		if ratio > 0.7 {
			return CollectiveTag{Type: CollectiveAllReduce, Confidence: ratio}
		}
	}

	// Asymmetric: one direction dominates
	if flow.Forward != nil && flow.Reverse == nil {
		// Check if this node sends small, receives large (AllGather) or vice versa
		if flow.Forward.Bytes > 0 {
			srcFlows := byNode[flow.Forward.NodeID]
			avgSendBytes := avgBytes(srcFlows)
			if flow.Forward.Bytes < avgSendBytes/2 {
				return CollectiveTag{Type: CollectiveAllGather, Confidence: 0.6}
			}
			if flow.Forward.Bytes > avgSendBytes*2 {
				return CollectiveTag{Type: CollectiveReduceScatter, Confidence: 0.6}
			}
		}
	}

	// Check for broadcast pattern: single source to many destinations
	if flow.Forward != nil {
		srcFlows := byNode[flow.Forward.NodeID]
		if len(srcFlows) > nodeCount/2 {
			return CollectiveTag{Type: CollectiveBroadcast, Confidence: 0.5}
		}
	}

	return CollectiveTag{Type: CollectiveUnknown, Confidence: 0.0}
}

func symmetryRatio(f *model.CorrelatedFlow) float64 {
	if f.Forward == nil || f.Reverse == nil {
		return 0
	}
	fwd := float64(f.Forward.Bytes)
	rev := float64(f.Reverse.Bytes)
	if fwd == 0 && rev == 0 {
		return 1.0
	}
	total := fwd + rev
	if total == 0 {
		return 0
	}
	// Closer to 0.5 = more symmetric
	balance := fwd / total
	if balance > 0.5 {
		balance = 1.0 - balance
	}
	return balance * 2 // normalize to 0..1
}

func avgBytes(flows []*model.CorrelatedFlow) uint64 {
	if len(flows) == 0 {
		return 0
	}
	var total uint64
	for _, f := range flows {
		total += f.TotalBytes
	}
	return total / uint64(len(flows))
}
