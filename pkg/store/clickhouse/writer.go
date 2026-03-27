package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/flowpulse/flowpulse/pkg/model"
	"github.com/rs/zerolog/log"
)

type Writer struct {
	conn     driver.Conn
	database string
}

func NewWriter(dsn, database string) (*Writer, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DSN: %w", err)
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping clickhouse: %w", err)
	}

	w := &Writer{conn: conn, database: database}
	if err := w.ensureSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	return w, nil
}

func (w *Writer) ensureSchema(ctx context.Context) error {
	queries := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", w.database),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.flows (
			tenant_id        String,
			flow_id          String,
			node_id          String,
			src_ip           UInt32,
			dst_ip           UInt32,
			src_port         UInt16,
			dst_port         UInt16,
			protocol         UInt8,
			direction        UInt8,
			packets          UInt64,
			bytes            UInt64,
			first_seen       DateTime64(3),
			last_seen        DateTime64(3),
			rdma_qp          UInt32  DEFAULT 0,
			rdma_dest_qp     UInt32  DEFAULT 0,
			rdma_msg_rate    UInt64  DEFAULT 0,
			rdma_retransmits UInt64  DEFAULT 0,
			rdma_ecn_marks   UInt64  DEFAULT 0,
			rdma_cnp_count   UInt64  DEFAULT 0,
			total_bytes      UInt64,
			total_pkts       UInt64,
			timestamp        DateTime64(3) DEFAULT now64(3)
		) ENGINE = MergeTree()
		PARTITION BY (tenant_id, toDate(timestamp))
		ORDER BY (tenant_id, flow_id, timestamp)
		TTL toDateTime(timestamp) + INTERVAL 30 DAY`, w.database),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.node_metrics (
			tenant_id         String,
			node_id           String,
			cpu_utilization   Float64,
			cpu_kernel_pct    Float64,
			cpu_softirq_pct   Float64,
			context_switches  UInt64,
			ib_tx_bytes       UInt64,
			ib_rx_bytes       UInt64,
			ib_link_util_pct  Float64,
			ib_errors         UInt64,
			timestamp         DateTime64(3) DEFAULT now64(3)
		) ENGINE = MergeTree()
		PARTITION BY (tenant_id, toDate(timestamp))
		ORDER BY (tenant_id, node_id, timestamp)
		TTL toDateTime(timestamp) + INTERVAL 7 DAY`, w.database),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.training_metrics (
			tenant_id                  String,
			straggler_score            Float64,
			bubble_ratio               Float64,
			gradient_sync_overhead_pct Float64,
			network_saturation_index   Float64,
			imbalance_score            Float64,
			timestamp                  DateTime64(3) DEFAULT now64(3)
		) ENGINE = MergeTree()
		PARTITION BY (tenant_id, toDate(timestamp))
		ORDER BY (tenant_id, timestamp)
		TTL toDateTime(timestamp) + INTERVAL 30 DAY`, w.database),

		fmt.Sprintf(`CREATE MATERIALIZED VIEW IF NOT EXISTS %s.metrics_1m
		ENGINE = SummingMergeTree()
		PARTITION BY (tenant_id, toDate(window_start))
		ORDER BY (tenant_id, node_id, window_start)
		AS SELECT
			tenant_id,
			node_id,
			toStartOfMinute(timestamp) AS window_start,
			avg(cpu_utilization)       AS avg_cpu,
			max(cpu_utilization)       AS max_cpu,
			avg(ib_link_util_pct)      AS avg_ib_util,
			sum(ib_tx_bytes)           AS total_tx_bytes,
			sum(ib_rx_bytes)           AS total_rx_bytes,
			sum(context_switches)      AS total_ctx_switches,
			count()                    AS sample_count
		FROM %s.node_metrics
		GROUP BY tenant_id, node_id, window_start`, w.database, w.database),
	}

	for _, q := range queries {
		if err := w.conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("exec schema query: %w", err)
		}
	}

	log.Info().Str("database", w.database).Msg("ClickHouse schema ensured")
	return nil
}

func (w *Writer) WriteFlows(ctx context.Context, flows []*model.CorrelatedFlow) error {
	batch, err := w.conn.PrepareBatch(ctx,
		fmt.Sprintf("INSERT INTO %s.flows", w.database))
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, f := range flows {
		var srcIP, dstIP uint32
		var srcPort, dstPort uint16
		var protocol, direction uint8
		var packets, bytesVal uint64
		var nodeID string

		if f.Forward != nil {
			srcIP = f.Forward.Key.SrcIP
			dstIP = f.Forward.Key.DstIP
			srcPort = f.Forward.Key.SrcPort
			dstPort = f.Forward.Key.DstPort
			protocol = f.Forward.Key.Protocol
			direction = uint8(f.Forward.Direction)
			packets = f.Forward.Packets
			bytesVal = f.Forward.Bytes
			nodeID = f.Forward.NodeID
		}

		var rdmaQP, rdmaDestQP uint32
		var rdmaMsgRate, rdmaRetransmits, rdmaECN, rdmaCNP uint64
		if f.RDMA != nil {
			rdmaQP = f.RDMA.QPNumber
			rdmaDestQP = f.RDMA.DestQP
			rdmaMsgRate = f.RDMA.RDMAMsgRate
			rdmaRetransmits = f.RDMA.Retransmissions
			rdmaECN = f.RDMA.ECNMarks
			rdmaCNP = f.RDMA.CNPCount
		}

		if err := batch.Append(
			f.TenantID, f.FlowID, nodeID,
			srcIP, dstIP, srcPort, dstPort, protocol, direction,
			packets, bytesVal, f.FirstSeen, f.LastSeen,
			rdmaQP, rdmaDestQP, rdmaMsgRate, rdmaRetransmits, rdmaECN, rdmaCNP,
			f.TotalBytes, f.TotalPkts, time.Now(),
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}

	return batch.Send()
}

func (w *Writer) WriteNodeMetrics(ctx context.Context, metrics []model.NodeMetrics) error {
	batch, err := w.conn.PrepareBatch(ctx,
		fmt.Sprintf("INSERT INTO %s.node_metrics", w.database))
	if err != nil {
		return fmt.Errorf("prepare node_metrics batch: %w", err)
	}

	for _, m := range metrics {
		var cpuUtil, kernelPct, softirqPct float64
		var ctxSwitches uint64
		for _, c := range m.CPUMetrics {
			cpuUtil += c.Utilization
			kernelPct += c.KernelPct
			softirqPct += c.SoftIRQPct
			ctxSwitches += c.ContextSwitches
		}
		if len(m.CPUMetrics) > 0 {
			cpuUtil /= float64(len(m.CPUMetrics))
			kernelPct /= float64(len(m.CPUMetrics))
			softirqPct /= float64(len(m.CPUMetrics))
		}

		var ibTx, ibRx uint64
		var ibUtil float64
		var ibErrors uint64
		if m.IBMetrics != nil {
			ibTx = m.IBMetrics.TxBytes
			ibRx = m.IBMetrics.RxBytes
			ibUtil = m.IBMetrics.LinkUtilizationPct
			ibErrors = m.IBMetrics.PortRcvErrors
		}

		if err := batch.Append(
			m.TenantID, m.NodeID,
			cpuUtil, kernelPct, softirqPct, ctxSwitches,
			ibTx, ibRx, ibUtil, ibErrors,
			time.Now(),
		); err != nil {
			return fmt.Errorf("append node_metrics row: %w", err)
		}
	}

	return batch.Send()
}

func (w *Writer) WriteTrainingMetrics(ctx context.Context, tm *model.TrainingMetrics) error {
	batch, err := w.conn.PrepareBatch(ctx,
		fmt.Sprintf("INSERT INTO %s.training_metrics", w.database))
	if err != nil {
		return fmt.Errorf("prepare training_metrics batch: %w", err)
	}

	if err := batch.Append(
		tm.TenantID,
		tm.StragglerScore,
		tm.BubbleRatio,
		tm.GradientSyncOverheadPct,
		tm.NetworkSaturationIndex,
		tm.ImbalanceScore,
		time.Now(),
	); err != nil {
		return fmt.Errorf("append training_metrics row: %w", err)
	}

	return batch.Send()
}

func (w *Writer) Close() error {
	return w.conn.Close()
}
