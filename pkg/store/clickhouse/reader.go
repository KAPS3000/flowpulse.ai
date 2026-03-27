package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/flowpulse/flowpulse/pkg/model"
)

type Reader struct {
	conn     driver.Conn
	database string
}

func NewReader(dsn, database string) (*Reader, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DSN: %w", err)
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	return &Reader{conn: conn, database: database}, nil
}

type FlowQuery struct {
	TenantID  string
	NodeID    string
	SrcIP     string
	DstIP     string
	Protocol  uint8
	StartTime time.Time
	EndTime   time.Time
	Limit     uint32
	Offset    uint32
	SortBy    string
	SortOrder string
}

type FlowResult struct {
	Flows      []model.Flow
	TotalCount uint64
}

func (r *Reader) QueryFlows(ctx context.Context, q FlowQuery) (*FlowResult, error) {
	where := "WHERE tenant_id = $1"
	args := []interface{}{q.TenantID}
	argIdx := 2

	if q.NodeID != "" {
		where += fmt.Sprintf(" AND node_id = $%d", argIdx)
		args = append(args, q.NodeID)
		argIdx++
	}
	if !q.StartTime.IsZero() {
		where += fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		args = append(args, q.StartTime)
		argIdx++
	}
	if !q.EndTime.IsZero() {
		where += fmt.Sprintf(" AND timestamp <= $%d", argIdx)
		args = append(args, q.EndTime)
		argIdx++
	}
	if q.Protocol != 0 {
		where += fmt.Sprintf(" AND protocol = $%d", argIdx)
		args = append(args, q.Protocol)
		argIdx++
	}

	countQuery := fmt.Sprintf("SELECT count() FROM %s.flows %s", r.database, where)
	var totalCount uint64
	row := r.conn.QueryRow(ctx, countQuery, args...)
	if err := row.Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count query: %w", err)
	}

	orderBy := "ORDER BY timestamp DESC"
	allowedSorts := map[string]bool{
		"bytes": true, "packets": true, "timestamp": true,
		"total_bytes": true, "first_seen": true, "last_seen": true,
	}
	if q.SortBy != "" && allowedSorts[q.SortBy] {
		dir := "DESC"
		if q.SortOrder == "asc" {
			dir = "ASC"
		}
		orderBy = fmt.Sprintf("ORDER BY %s %s", q.SortBy, dir)
	}

	limit := uint32(100)
	if q.Limit > 0 && q.Limit <= 10000 {
		limit = q.Limit
	}

	dataQuery := fmt.Sprintf(
		`SELECT tenant_id, flow_id, node_id, src_ip, dst_ip, src_port, dst_port,
		        protocol, direction, packets, bytes, first_seen, last_seen,
		        rdma_qp, rdma_dest_qp, rdma_msg_rate, rdma_retransmits, rdma_ecn_marks, rdma_cnp_count
		 FROM %s.flows %s %s LIMIT %d OFFSET %d`,
		r.database, where, orderBy, limit, q.Offset,
	)

	rows, err := r.conn.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("data query: %w", err)
	}
	defer rows.Close()

	var flows []model.Flow
	for rows.Next() {
		var f model.Flow
		var rdmaQP, rdmaDestQP uint32
		var rdmaMsgRate, rdmaRetransmits, rdmaECN, rdmaCNP uint64
		var dir uint8

		if err := rows.Scan(
			&f.TenantID, &f.FlowID, &f.NodeID,
			&f.Key.SrcIP, &f.Key.DstIP, &f.Key.SrcPort, &f.Key.DstPort,
			&f.Key.Protocol, &dir, &f.Packets, &f.Bytes,
			&f.FirstSeen, &f.LastSeen,
			&rdmaQP, &rdmaDestQP, &rdmaMsgRate, &rdmaRetransmits, &rdmaECN, &rdmaCNP,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		f.Direction = model.FlowDirection(dir)
		if rdmaQP != 0 {
			f.RDMA = &model.RDMAInfo{
				QPNumber:        rdmaQP,
				DestQP:          rdmaDestQP,
				RDMAMsgRate:     rdmaMsgRate,
				Retransmissions: rdmaRetransmits,
				ECNMarks:        rdmaECN,
				CNPCount:        rdmaCNP,
			}
		}
		flows = append(flows, f)
	}

	return &FlowResult{Flows: flows, TotalCount: totalCount}, nil
}

func (r *Reader) GetTopology(ctx context.Context, tenantID string) ([]model.NodeMetrics, error) {
	query := fmt.Sprintf(
		`SELECT node_id, avg(cpu_utilization), avg(ib_link_util_pct),
		        sum(ib_tx_bytes), sum(ib_rx_bytes), sum(ib_errors)
		 FROM %s.node_metrics
		 WHERE tenant_id = $1 AND timestamp >= now() - INTERVAL 5 MINUTE
		 GROUP BY node_id`, r.database)

	rows, err := r.conn.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("topology query: %w", err)
	}
	defer rows.Close()

	var result []model.NodeMetrics
	for rows.Next() {
		var nm model.NodeMetrics
		nm.TenantID = tenantID
		nm.IBMetrics = &model.IBPortMetrics{}

		var avgCPU, avgIBUtil float64
		var txBytes, rxBytes, errors uint64

		if err := rows.Scan(&nm.NodeID, &avgCPU, &avgIBUtil, &txBytes, &rxBytes, &errors); err != nil {
			return nil, fmt.Errorf("scan topology: %w", err)
		}

		nm.CPUMetrics = []model.CPUMetric{{Utilization: avgCPU}}
		nm.IBMetrics.LinkUtilizationPct = avgIBUtil
		nm.IBMetrics.TxBytes = txBytes
		nm.IBMetrics.RxBytes = rxBytes
		nm.Timestamp = time.Now()
		result = append(result, nm)
	}

	return result, nil
}

func (r *Reader) GetTrainingMetrics(ctx context.Context, tenantID, window string) (*model.TrainingMetrics, error) {
	interval := "5 MINUTE"
	switch window {
	case "1m":
		interval = "1 MINUTE"
	case "1h":
		interval = "1 HOUR"
	}

	query := fmt.Sprintf(
		`SELECT straggler_score, bubble_ratio, gradient_sync_overhead_pct,
		        network_saturation_index, imbalance_score, timestamp
		 FROM %s.training_metrics
		 WHERE tenant_id = $1 AND timestamp >= now() - INTERVAL %s
		 ORDER BY timestamp DESC LIMIT 1`, r.database, interval)

	var tm model.TrainingMetrics
	tm.TenantID = tenantID
	row := r.conn.QueryRow(ctx, query, tenantID)
	if err := row.Scan(
		&tm.StragglerScore, &tm.BubbleRatio, &tm.GradientSyncOverheadPct,
		&tm.NetworkSaturationIndex, &tm.ImbalanceScore, &tm.Timestamp,
	); err != nil {
		return nil, fmt.Errorf("training metrics query: %w", err)
	}

	return &tm, nil
}

func (r *Reader) Close() error {
	return r.conn.Close()
}
