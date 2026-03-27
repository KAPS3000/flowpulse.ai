package agent

import (
	"fmt"
	"path/filepath"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/rs/zerolog/log"
)

// BPFFlowKey mirrors struct flow_key in common.h
type BPFFlowKey struct {
	SrcIP    uint32
	DstIP    uint32
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8
	Pad      [3]uint8
}

// BPFFlowValue mirrors struct flow_value in common.h
type BPFFlowValue struct {
	Packets       uint64
	Bytes         uint64
	FirstSeenNs   uint64
	LastSeenNs    uint64
	Direction     uint8
	Pad           [7]uint8
	RDMAQp        uint32
	RDMADestQp    uint32
	RDMAMsgCount  uint64
	RDMARetransmits uint64
	RDMAEcnMarks  uint64
	RDMACnpCount  uint64
}

// BPFFlowEvent mirrors struct flow_event in common.h
type BPFFlowEvent struct {
	Key         BPFFlowKey
	Direction   uint8
	Pad         [7]uint8
	TimestampNs uint64
}

// BPFCpuValue mirrors struct cpu_value in common.h
type BPFCpuValue struct {
	OnCpuNs            uint64
	OffCpuNs           uint64
	VoluntarySwitches   uint64
	InvoluntarySwitches uint64
	LastSwitchNs       uint64
}

// BPFSoftIRQValue mirrors struct softirq_value in common.h
type BPFSoftIRQValue struct {
	TotalNs uint64
	NetRxNs uint64
	NetTxNs uint64
	Count   uint64
}

type ebpfObjects struct {
	FlowTable    *ebpf.Map
	FlowEvents   *ringbuf.Reader
	PktCounter   *ebpf.Map
	CPUStats     *ebpf.Map
	SoftIRQStats *ebpf.Map
	CPUEvents    *ringbuf.Reader

	links []link.Link
}

func loadEBPF(cfg *Config) (*ebpfObjects, error) {
	objs := &ebpfObjects{}

	// Load flow tracker (non-fatal: TC programs may fail verification on some kernels)
	flowSpec, err := ebpf.LoadCollectionSpec(
		filepath.Join(cfg.EBPF.BPFObjectDir, "flow_tracker.o"))
	if err != nil {
		log.Warn().Err(err).Msg("flow_tracker.o not available, flow tracking disabled")
	} else {
		flowColl, err := ebpf.NewCollection(flowSpec)
		if err != nil {
			log.Warn().Err(err).Msg("flow_tracker verification failed (kernel may not support TC BPF), flow tracking disabled")
		} else {
			objs.FlowTable = flowColl.Maps["flow_table"]
			objs.PktCounter = flowColl.Maps["pkt_counter"]

			flowEventsMap := flowColl.Maps["flow_events"]
			if flowEventsMap != nil {
				rd, err := ringbuf.NewReader(flowEventsMap)
				if err != nil {
					log.Warn().Err(err).Msg("flow ring buffer not available")
				} else {
					objs.FlowEvents = rd
				}
			}
			log.Info().Msg("flow_tracker loaded successfully")
		}
	}

	// Load CPU scheduler probes
	cpuSpec, err := ebpf.LoadCollectionSpec(
		filepath.Join(cfg.EBPF.BPFObjectDir, "cpu_sched.o"))
	if err != nil {
		return nil, fmt.Errorf("load cpu_sched.o: %w", err)
	}

	cpuColl, err := ebpf.NewCollection(cpuSpec)
	if err != nil {
		return nil, fmt.Errorf("create cpu collection: %w", err)
	}

	objs.CPUStats = cpuColl.Maps["cpu_stats"]
	objs.SoftIRQStats = cpuColl.Maps["softirq_stats"]

	cpuEventsMap := cpuColl.Maps["cpu_events"]
	if cpuEventsMap != nil {
		rd, err := ringbuf.NewReader(cpuEventsMap)
		if err != nil {
			log.Warn().Err(err).Msg("cpu_events ring buffer not available")
		} else {
			objs.CPUEvents = rd
		}
	}

	// Attach tracepoints for CPU scheduling
	for name, prog := range cpuColl.Programs {
		var l link.Link
		switch name {
		case "handle_sched_switch":
			l, err = link.Tracepoint("sched", "sched_switch", prog, nil)
		case "handle_softirq_entry":
			l, err = link.Tracepoint("irq", "softirq_entry", prog, nil)
		case "handle_softirq_exit":
			l, err = link.Tracepoint("irq", "softirq_exit", prog, nil)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("attach tracepoint %s: %w", name, err)
		}
		objs.links = append(objs.links, l)
		log.Info().Str("program", name).Msg("attached tracepoint")
	}

	log.Info().Msg("eBPF programs loaded and attached")
	return objs, nil
}

func (o *ebpfObjects) Close() {
	for _, l := range o.links {
		l.Close()
	}
	if o.FlowEvents != nil {
		o.FlowEvents.Close()
	}
	if o.CPUEvents != nil {
		o.CPUEvents.Close()
	}
}
