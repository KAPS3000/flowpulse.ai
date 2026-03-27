package handlers

import (
	"net/http"

	"github.com/flowpulse/flowpulse/pkg/api/middleware"
	"github.com/flowpulse/flowpulse/pkg/store/clickhouse"
)

type MetricsHandler struct {
	reader *clickhouse.Reader
}

func NewMetricsHandler(reader *clickhouse.Reader) *MetricsHandler {
	return &MetricsHandler{reader: reader}
}

func (h *MetricsHandler) GetTrainingMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no tenant context"})
		return
	}

	window := r.URL.Query().Get("window")
	if window == "" {
		window = "5m"
	}

	tm, err := h.reader.GetTrainingMetrics(r.Context(), tenantID, window)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, tm)
}
