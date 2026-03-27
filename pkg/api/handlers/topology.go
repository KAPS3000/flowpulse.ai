package handlers

import (
	"net/http"

	"github.com/flowpulse/flowpulse/pkg/api/middleware"
	"github.com/flowpulse/flowpulse/pkg/store/clickhouse"
)

type TopologyHandler struct {
	reader *clickhouse.Reader
}

func NewTopologyHandler(reader *clickhouse.Reader) *TopologyHandler {
	return &TopologyHandler{reader: reader}
}

func (h *TopologyHandler) GetTopology(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no tenant context"})
		return
	}

	nodes, err := h.reader.GetTopology(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type topologyNode struct {
		NodeID    string  `json:"node_id"`
		CPUAvg    float64 `json:"cpu_avg"`
		IBUtilPct float64 `json:"ib_util_pct"`
		TxBytes   uint64  `json:"tx_bytes"`
		RxBytes   uint64  `json:"rx_bytes"`
		Status    string  `json:"status"`
	}

	result := make([]topologyNode, 0, len(nodes))
	for _, n := range nodes {
		tn := topologyNode{
			NodeID: n.NodeID,
			Status: "healthy",
		}
		if len(n.CPUMetrics) > 0 {
			tn.CPUAvg = n.CPUMetrics[0].Utilization
		}
		if n.IBMetrics != nil {
			tn.IBUtilPct = n.IBMetrics.LinkUtilizationPct
			tn.TxBytes = n.IBMetrics.TxBytes
			tn.RxBytes = n.IBMetrics.RxBytes
		}
		result = append(result, tn)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": result,
	})
}
