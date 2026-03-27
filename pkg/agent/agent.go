package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"time"

	"github.com/flowpulse/flowpulse/pkg/model"
	"github.com/flowpulse/flowpulse/pkg/transport"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type Agent struct {
	cfg       *Config
	ebpf      *ebpfObjects
	flowTable *FlowTable
	client    *transport.GRPCClient
}

func New(cfg *Config) (*Agent, error) {
	ebpfObjs, err := loadEBPF(cfg)
	if err != nil {
		return nil, fmt.Errorf("load eBPF: %w", err)
	}

	client, err := transport.NewGRPCClient(cfg.Aggregator.Address)
	if err != nil {
		ebpfObjs.Close()
		return nil, fmt.Errorf("create gRPC client: %w", err)
	}

	return &Agent{
		cfg:       cfg,
		ebpf:      ebpfObjs,
		flowTable: NewFlowTable(cfg.EBPF.FlowTimeout, cfg.EBPF.MaxFlows),
		client:    client,
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.pollFlowMaps(ctx)
	})

	g.Go(func() error {
		return a.readFlowEvents(ctx)
	})

	g.Go(func() error {
		return a.pollCPUMaps(ctx)
	})

	g.Go(func() error {
		return a.flushLoop(ctx)
	})

	g.Go(func() error {
		return a.evictionLoop(ctx)
	})

	g.Go(func() error {
		return a.serveHealth(ctx)
	})

	return g.Wait()
}

func (a *Agent) pollFlowMaps(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.EBPF.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.readFlowTable()
		}
	}
}

func (a *Agent) readFlowTable() {
	if a.ebpf.FlowTable == nil {
		return
	}

	var key BPFFlowKey
	var val BPFFlowValue

	iter := a.ebpf.FlowTable.Iterate()
	count := 0
	for iter.Next(&key, &val) {
		a.flowTable.Update(key, val, a.cfg.NodeID, a.cfg.TenantID)
		count++
	}

	if err := iter.Err(); err != nil {
		log.Warn().Err(err).Msg("flow table iteration error")
	}

	if count > 0 {
		log.Debug().Int("flows", count).Msg("polled flow table")
	}
}

func (a *Agent) readFlowEvents(ctx context.Context) error {
	if a.ebpf.FlowEvents == nil {
		log.Warn().Msg("flow events ring buffer not available, skipping")
		<-ctx.Done()
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		record, err := a.ebpf.FlowEvents.Read()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Warn().Err(err).Msg("ring buffer read error")
			continue
		}

		var evt BPFFlowEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &evt); err != nil {
			log.Warn().Err(err).Msg("failed to parse flow event")
			continue
		}

		log.Debug().
			Uint32("src_ip", evt.Key.SrcIP).
			Uint32("dst_ip", evt.Key.DstIP).
			Uint16("src_port", evt.Key.SrcPort).
			Uint16("dst_port", evt.Key.DstPort).
			Msg("new flow detected")
	}
}

func (a *Agent) pollCPUMaps(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.EBPF.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.readCPUStats()
		}
	}
}

func (a *Agent) readCPUStats() {
	if a.ebpf.CPUStats == nil {
		return
	}

	var cpuID uint32
	var vals []BPFCpuValue

	metrics := &model.NodeMetrics{
		NodeID:   a.cfg.NodeID,
		TenantID: a.cfg.TenantID,
	}

	iter := a.ebpf.CPUStats.Iterate()
	for iter.Next(&cpuID, &vals) {
		var totalOn, totalOff, voluntarySw, involuntarySw uint64
		for _, v := range vals {
			totalOn += v.OnCpuNs
			totalOff += v.OffCpuNs
			voluntarySw += v.VoluntarySwitches
			involuntarySw += v.InvoluntarySwitches
		}

		total := totalOn + totalOff
		utilization := 0.0
		if total > 0 {
			utilization = float64(totalOn) / float64(total) * 100
		}

		metrics.CPUMetrics = append(metrics.CPUMetrics, model.CPUMetric{
			CoreID:          cpuID,
			Utilization:     utilization,
			ContextSwitches: voluntarySw + involuntarySw,
			InvoluntaryCS:   involuntarySw,
		})
	}

	if a.ebpf.SoftIRQStats != nil {
		var sirqVals []BPFSoftIRQValue
		sirqIter := a.ebpf.SoftIRQStats.Iterate()
		idx := 0
		for sirqIter.Next(&cpuID, &sirqVals) {
			var totalNs, netRxNs, netTxNs uint64
			for _, sv := range sirqVals {
				totalNs += sv.TotalNs
				netRxNs += sv.NetRxNs
				netTxNs += sv.NetTxNs
			}

			if idx < len(metrics.CPUMetrics) && totalNs > 0 {
				metrics.CPUMetrics[idx].SoftIRQPct = float64(netRxNs+netTxNs) /
					float64(totalNs) * 100
			}
			idx++
		}
	}

	metrics.Timestamp = time.Now()

	if err := a.client.SendMetrics(metrics); err != nil {
		log.Warn().Err(err).Msg("failed to send CPU metrics")
	} else {
		log.Debug().Int("cores", len(metrics.CPUMetrics)).Msg("sent node metrics")
	}
}

func (a *Agent) flushLoop(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.Aggregator.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.flushFlows()
		}
	}
}

func (a *Agent) flushFlows() {
	flows := a.flowTable.Drain()
	if len(flows) == 0 {
		return
	}

	// Send in batches
	batchSize := a.cfg.Aggregator.BatchSize
	for i := 0; i < len(flows); i += batchSize {
		end := i + batchSize
		if end > len(flows) {
			end = len(flows)
		}

		batch := &model.FlowBatch{
			NodeID:      a.cfg.NodeID,
			TenantID:    a.cfg.TenantID,
			Flows:       flows[i:end],
			CollectedAt: time.Now(),
		}

		if err := a.client.SendFlows(batch); err != nil {
			log.Error().Err(err).
				Int("batch_size", len(batch.Flows)).
				Msg("failed to send flow batch")
		}
	}

	log.Info().Int("total_flows", len(flows)).Msg("flushed flows to aggregator")
}

func (a *Agent) evictionLoop(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.EBPF.FlowTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			evicted := a.flowTable.EvictStale(time.Now())
			if evicted > 0 {
				log.Debug().Int("evicted", evicted).
					Int("remaining", a.flowTable.Len()).
					Msg("evicted stale flows")
			}
		}
	}
}

func (a *Agent) serveHealth(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "# HELP flowpulse_agent_flows_tracked Number of tracked flows\n")
		fmt.Fprintf(w, "# TYPE flowpulse_agent_flows_tracked gauge\n")
		fmt.Fprintf(w, "flowpulse_agent_flows_tracked %d\n", a.flowTable.Len())
	})

	srv := &http.Server{
		Addr:              a.cfg.Health.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	log.Info().Str("addr", a.cfg.Health.Listen).Msg("health server listening")
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
