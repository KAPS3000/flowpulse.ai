package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/flowpulse/flowpulse/pkg/api/middleware"
	"github.com/flowpulse/flowpulse/pkg/store/clickhouse"
)

type FlowHandler struct {
	reader *clickhouse.Reader
}

func NewFlowHandler(reader *clickhouse.Reader) *FlowHandler {
	return &FlowHandler{reader: reader}
}

func (h *FlowHandler) ListFlows(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no tenant context"})
		return
	}

	q := clickhouse.FlowQuery{
		TenantID:  tenantID,
		NodeID:    r.URL.Query().Get("node_id"),
		SrcIP:     r.URL.Query().Get("src_ip"),
		DstIP:     r.URL.Query().Get("dst_ip"),
		SortBy:    r.URL.Query().Get("sort_by"),
		SortOrder: r.URL.Query().Get("sort_order"),
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			q.Limit = uint32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			q.Offset = uint32(n)
		}
	}
	if v := r.URL.Query().Get("protocol"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 8); err == nil {
			q.Protocol = uint8(n)
		}
	}
	if v := r.URL.Query().Get("start_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.StartTime = t
		}
	}
	if v := r.URL.Query().Get("end_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.EndTime = t
		}
	}

	result, err := h.reader.QueryFlows(r.Context(), q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"flows":       result.Flows,
		"total_count": result.TotalCount,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
