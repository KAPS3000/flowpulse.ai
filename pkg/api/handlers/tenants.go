package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/flowpulse/flowpulse/pkg/api/middleware"
	"github.com/flowpulse/flowpulse/pkg/model"
	"github.com/flowpulse/flowpulse/pkg/tenant"
	"github.com/go-chi/chi/v5"
)

type TenantHandler struct {
	manager *tenant.Manager
}

func NewTenantHandler(mgr *tenant.Manager) *TenantHandler {
	return &TenantHandler{manager: mgr}
}

func (h *TenantHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.Role != model.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return
	}
	tenants := h.manager.ListTenants(r.Context())
	writeJSON(w, http.StatusOK, map[string]interface{}{"tenants": tenants})
}

func (h *TenantHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.Role != model.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return
	}

	var t model.Tenant
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if t.ID == "" || t.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id and name required"})
		return
	}

	if err := h.manager.CreateTenant(r.Context(), t); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, t)
}

func (h *TenantHandler) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || (claims.Role != model.RoleAdmin && claims.TenantID != id) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	t, err := h.manager.GetTenant(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *TenantHandler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || claims.Role != model.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.manager.DeleteTenant(r.Context(), id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
