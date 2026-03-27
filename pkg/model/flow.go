package model

import (
	"fmt"
	"time"
)

type FlowKey struct {
	SrcIP    uint32 `json:"src_ip"`
	DstIP    uint32 `json:"dst_ip"`
	SrcPort  uint16 `json:"src_port"`
	DstPort  uint16 `json:"dst_port"`
	Protocol uint8  `json:"protocol"`
}

func (k FlowKey) String() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d->%d.%d.%d.%d:%d/p%d",
		k.SrcIP>>24, (k.SrcIP>>16)&0xff, (k.SrcIP>>8)&0xff, k.SrcIP&0xff, k.SrcPort,
		k.DstIP>>24, (k.DstIP>>16)&0xff, (k.DstIP>>8)&0xff, k.DstIP&0xff, k.DstPort,
		k.Protocol)
}

type FlowDirection uint8

const (
	FlowDirectionUnknown FlowDirection = iota
	FlowDirectionIngress
	FlowDirectionEgress
)

type RDMAInfo struct {
	QPNumber       uint32 `json:"qp_number"`
	DestQP         uint32 `json:"dest_qp"`
	RDMAMsgRate    uint64 `json:"rdma_msg_rate"`
	Retransmissions uint64 `json:"retransmissions"`
	ECNMarks       uint64 `json:"ecn_marks"`
	CNPCount       uint64 `json:"cnp_count"`
}

type Flow struct {
	Key       FlowKey       `json:"key"`
	FlowID    string        `json:"flow_id"`
	TenantID  string        `json:"tenant_id"`
	NodeID    string        `json:"node_id"`
	Packets   uint64        `json:"packets"`
	Bytes     uint64        `json:"bytes"`
	FirstSeen time.Time     `json:"first_seen"`
	LastSeen  time.Time     `json:"last_seen"`
	Direction FlowDirection `json:"direction"`
	RDMA      *RDMAInfo     `json:"rdma,omitempty"`
}

type FlowBatch struct {
	NodeID      string    `json:"node_id"`
	TenantID    string    `json:"tenant_id"`
	Flows       []Flow    `json:"flows"`
	CollectedAt time.Time `json:"collected_at"`
}

// CorrelatedFlow represents a bidirectional flow (two unidirectional flows merged).
type CorrelatedFlow struct {
	FlowID     string    `json:"flow_id"`
	TenantID   string    `json:"tenant_id"`
	Forward    *Flow     `json:"forward"`
	Reverse    *Flow     `json:"reverse,omitempty"`
	TotalBytes uint64    `json:"total_bytes"`
	TotalPkts  uint64    `json:"total_pkts"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	RDMA       *RDMAInfo `json:"rdma,omitempty"`
}
