package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/api/tenant"
)

// Tenant admin endpoints (BA / backlog 10.1 + 10.2).
//
// All endpoints under /admin/tenants require the shared admin token
// configured via env TENANT_ADMIN_TOKEN. This is a deliberately-
// separate credential from the per-tenant X-API-Key headers so a
// tenant that gets its own key rotated does not automatically also
// have admin authority.
//
// Endpoints:
//
//	GET  /admin/tenants                  -> list (key hashes stripped)
//	POST /admin/tenants                  -> create ({id, display_name, keys[], quotas...})
//	POST /admin/tenants/{id}/keys        -> rotate: add new key ({key})
//	POST /admin/tenants/{id}/keys/revoke -> rotate: drop old keys ({keep_last})
//	GET  /admin/tenants/{id}/audit       -> per-tenant audit log
//	GET  /admin/tenants/audit            -> whole-store audit log
//
// The endpoints are only registered when tenant mode is ON (see
// server.go). When TENANTS_FILE is unset, the routes are absent from
// the mux entirely so any accidental probes 404 out.

func (s *Server) requireTenantAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.tenants == nil {
		s.jsonError(w, "tenant mode disabled", http.StatusNotFound)
		return false
	}
	want := strings.TrimSpace(os.Getenv("TENANT_ADMIN_TOKEN"))
	if want == "" {
		// Explicit refusal: admin endpoints must not be reachable
		// without an admin token, even in dev.
		s.jsonError(w, "tenant admin disabled: TENANT_ADMIN_TOKEN not set", http.StatusForbidden)
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-Tenant-Admin-Token"))
	if got == "" || got != want {
		s.tenants.Log(tenant.AuditEvent{
			Kind:   tenant.AuditAuthRejected,
			Detail: "admin token mismatch on " + r.URL.Path,
		})
		s.jsonError(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) tenantList(w http.ResponseWriter, r *http.Request) {
	if !s.requireTenantAdmin(w, r) {
		return
	}
	list := s.tenants.List()
	writeJSON(w, http.StatusOK, map[string]any{"tenants": list})
}

type tenantCreateReq struct {
	ID                string   `json:"id"`
	DisplayName       string   `json:"display_name"`
	Namespace         string   `json:"namespace"`
	Keys              []string `json:"keys"`
	RatePerSecond     int      `json:"rate_per_second"`
	RatePerMinute     int      `json:"rate_per_minute"`
	MonthlyProofQuota int      `json:"monthly_proof_quota"`
}

func (s *Server) tenantCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireTenantAdmin(w, r) {
		return
	}
	var req tenantCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if req.ID == "" || len(req.Keys) == 0 {
		s.jsonError(w, "id and >=1 key required", http.StatusBadRequest)
		return
	}
	hashes := make([]string, 0, len(req.Keys))
	for _, k := range req.Keys {
		if k == "" {
			s.jsonError(w, "empty key not allowed", http.StatusBadRequest)
			return
		}
		hashes = append(hashes, tenant.HashKey(k))
	}
	tn := &tenant.Tenant{
		ID:                req.ID,
		DisplayName:       req.DisplayName,
		Namespace:         req.Namespace,
		KeyHashes:         hashes,
		RatePerSecond:     req.RatePerSecond,
		RatePerMinute:     req.RatePerMinute,
		MonthlyProofQuota: req.MonthlyProofQuota,
	}
	if err := s.tenants.Add(tn); err != nil {
		s.jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	// Strip hashes on the way out so we don't echo credentials back.
	tn.KeyHashes = nil
	writeJSON(w, http.StatusCreated, map[string]any{"tenant": tn})
}

func (s *Server) tenantAddKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireTenantAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		s.jsonError(w, "tenant id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if body.Key == "" {
		s.jsonError(w, "key required", http.StatusBadRequest)
		return
	}
	if err := s.tenants.RotateAddKey(id, body.Key); err != nil {
		s.jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "added"})
}

func (s *Server) tenantRevokeKeys(w http.ResponseWriter, r *http.Request) {
	if !s.requireTenantAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		s.jsonError(w, "tenant id required", http.StatusBadRequest)
		return
	}
	var body struct {
		KeepLast int `json:"keep_last"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if body.KeepLast < 1 {
		body.KeepLast = 1
	}
	if err := s.tenants.RotateRevokeOldKeys(id, body.KeepLast); err != nil {
		s.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "revoked", "keep_last": body.KeepLast})
}

func (s *Server) tenantAudit(w http.ResponseWriter, r *http.Request) {
	if !s.requireTenantAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		id = ""
	}
	events := s.tenants.Audit(id)
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "count": len(events)})
}
