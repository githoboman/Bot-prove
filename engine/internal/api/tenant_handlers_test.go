package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/api/tenant"
)

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

// newTenantTestServer builds a bare-bones *Server wired up with a
// tenant store and mounts only the tenant middleware + admin routes.
// It deliberately does NOT stand up the prover / DB / submitter
// pipeline — those are not exercised here.
func newTenantTestServer(t *testing.T, adminToken string, seed func(*tenant.Store)) (*Server, http.Handler) {
	t.Helper()
	ts := tenant.NewStore()
	if seed != nil {
		seed(ts)
	}
	s := &Server{tenants: ts}
	// Force admin token via env in-process.
	t.Setenv("TENANT_ADMIN_TOKEN", adminToken)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/tenants", s.tenantList)
	mux.HandleFunc("POST /admin/tenants", s.tenantCreate)
	mux.HandleFunc("POST /admin/tenants/{id}/keys", s.tenantAddKey)
	mux.HandleFunc("POST /admin/tenants/{id}/keys/revoke", s.tenantRevokeKeys)
	mux.HandleFunc("GET /admin/tenants/{id}/audit", s.tenantAudit)
	mux.HandleFunc("GET /admin/tenants/audit", s.tenantAudit)
	return s, mux
}

func tenantDoJSON(t *testing.T, h http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// -----------------------------------------------------------------------------
// Admin endpoints
// -----------------------------------------------------------------------------

func TestAdminListRequiresAdminToken(t *testing.T) {
	_, h := newTenantTestServer(t, "adm-secret", nil)
	// Missing token.
	rr := tenantDoJSON(t, h, http.MethodGet, "/admin/tenants", nil, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("missing token: got %d, want 403", rr.Code)
	}
	// Wrong token.
	rr = tenantDoJSON(t, h, http.MethodGet, "/admin/tenants", nil, map[string]string{"X-Tenant-Admin-Token": "wrong"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("wrong token: got %d, want 403", rr.Code)
	}
	// Correct token.
	rr = tenantDoJSON(t, h, http.MethodGet, "/admin/tenants", nil, map[string]string{"X-Tenant-Admin-Token": "adm-secret"})
	if rr.Code != http.StatusOK {
		t.Fatalf("correct token: got %d, want 200", rr.Code)
	}
}

func TestAdminCreateThenList(t *testing.T) {
	_, h := newTenantTestServer(t, "adm", nil)
	body := tenantCreateReq{
		ID:                "acme",
		DisplayName:       "ACME Corp",
		Keys:              []string{"raw-a", "raw-b"},
		RatePerSecond:     3,
		MonthlyProofQuota: 100,
	}
	rr := tenantDoJSON(t, h, http.MethodPost, "/admin/tenants", body, map[string]string{"X-Tenant-Admin-Token": "adm"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", rr.Code, rr.Body.String())
	}
	// Create response must not leak keys.
	var created struct {
		Tenant tenant.Tenant `json:"tenant"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(created.Tenant.KeyHashes) != 0 {
		t.Fatalf("create leaked key hashes: %+v", created.Tenant.KeyHashes)
	}

	// List returns the tenant without hashes.
	rr = tenantDoJSON(t, h, http.MethodGet, "/admin/tenants", nil, map[string]string{"X-Tenant-Admin-Token": "adm"})
	if rr.Code != http.StatusOK {
		t.Fatalf("list: %d", rr.Code)
	}
	var listed struct {
		Tenants []tenant.Tenant `json:"tenants"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listed.Tenants) != 1 || listed.Tenants[0].ID != "acme" {
		t.Fatalf("listed: %+v", listed.Tenants)
	}
	if len(listed.Tenants[0].KeyHashes) != 0 {
		t.Fatalf("list leaked key hashes")
	}
}

func TestAdminCreateRejectsMissingFields(t *testing.T) {
	_, h := newTenantTestServer(t, "adm", nil)
	rr := tenantDoJSON(t, h, http.MethodPost, "/admin/tenants", tenantCreateReq{}, map[string]string{"X-Tenant-Admin-Token": "adm"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty body: %d", rr.Code)
	}
}

func TestAdminRotateAddThenRevoke(t *testing.T) {
	_, h := newTenantTestServer(t, "adm", func(s *tenant.Store) {
		_ = s.Add(&tenant.Tenant{ID: "acme", KeyHashes: []string{tenant.HashKey("old")}})
	})
	// Add.
	rr := tenantDoJSON(t, h, http.MethodPost, "/admin/tenants/acme/keys",
		map[string]string{"key": "new"},
		map[string]string{"X-Tenant-Admin-Token": "adm"})
	if rr.Code != http.StatusOK {
		t.Fatalf("add key: %d", rr.Code)
	}
	// Revoke keeping last 1.
	rr = tenantDoJSON(t, h, http.MethodPost, "/admin/tenants/acme/keys/revoke",
		map[string]int{"keep_last": 1},
		map[string]string{"X-Tenant-Admin-Token": "adm"})
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: %d", rr.Code)
	}
}

func TestAdminAuditPerTenantScoped(t *testing.T) {
	_, h := newTenantTestServer(t, "adm", func(s *tenant.Store) {
		_ = s.Add(&tenant.Tenant{ID: "acme", KeyHashes: []string{tenant.HashKey("k")}})
		_ = s.Add(&tenant.Tenant{ID: "beta", KeyHashes: []string{tenant.HashKey("k2")}})
	})
	rr := tenantDoJSON(t, h, http.MethodGet, "/admin/tenants/acme/audit", nil,
		map[string]string{"X-Tenant-Admin-Token": "adm"})
	if rr.Code != http.StatusOK {
		t.Fatalf("audit: %d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Events []tenant.AuditEvent `json:"events"`
		Count  int                 `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Count == 0 {
		t.Fatal("no events")
	}
	for _, e := range got.Events {
		if e.TenantID != "acme" {
			t.Fatalf("audit leaked events across tenants: %+v", e)
		}
	}
}

func TestAdminEndpointsAbsentInCompatMode(t *testing.T) {
	// Setup a Server without the tenant store to prove the guard.
	s := &Server{tenants: nil}
	mux := http.NewServeMux()
	// We register a handler that goes through requireTenantAdmin —
	// which must refuse when tenants is nil.
	mux.HandleFunc("GET /admin/tenants", s.tenantList)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("compat mode should refuse admin: got %d", rr.Code)
	}
}

// -----------------------------------------------------------------------------
// Middleware — tenant resolution + rate limit + audit
// -----------------------------------------------------------------------------

func TestAuthMiddlewareTenantModeResolvesKey(t *testing.T) {
	ts := tenant.NewStore()
	_ = ts.Add(&tenant.Tenant{ID: "acme", Namespace: "ns_acme", KeyHashes: []string{tenant.HashKey("k1")}})
	s := &Server{tenants: ts}
	var seen *tenant.Tenant
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = tenantFromCtx(r)
		w.WriteHeader(http.StatusOK)
	})
	h := s.authMiddleware(inner)

	// No key → 401.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/proofs", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing key: %d", rr.Code)
	}

	// Wrong key → 401.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/proofs", nil)
	req.Header.Set("X-API-Key", "nope")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d", rr.Code)
	}

	// Correct key → 200 and tenant in context.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/proofs", nil)
	req.Header.Set("X-API-Key", "k1")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("valid key: %d", rr.Code)
	}
	if seen == nil || seen.ID != "acme" {
		t.Fatalf("tenant not in context: %+v", seen)
	}
}

func TestAuthMiddlewareTenantModeEnforcesRate(t *testing.T) {
	ts := tenant.NewStore()
	_ = ts.Add(&tenant.Tenant{
		ID:            "acme",
		Namespace:     "ns_acme",
		KeyHashes:     []string{tenant.HashKey("k1")},
		RatePerSecond: 2,
	})
	s := &Server{tenants: ts}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := s.authMiddleware(inner)

	do := func() int {
		req := httptest.NewRequest(http.MethodPost, "/proofs", nil)
		req.Header.Set("X-API-Key", "k1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	if got := do(); got != http.StatusOK {
		t.Fatalf("1st: %d", got)
	}
	if got := do(); got != http.StatusOK {
		t.Fatalf("2nd: %d", got)
	}
	if got := do(); got != http.StatusTooManyRequests {
		t.Fatalf("3rd: %d, want 429", got)
	}
}

func TestAuthMiddlewareCompatModeUsesLegacyKey(t *testing.T) {
	// tenants=nil, apiKey set — must fall through to legacy path.
	s := &Server{apiKey: "shared-secret"}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := s.authMiddleware(inner)

	// GET passes without header.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/proofs", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET should be allowed: %d", rr.Code)
	}

	// POST without header rejected.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/proofs", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("POST no key: %d", rr.Code)
	}

	// POST with legacy shared key allowed.
	req := httptest.NewRequest(http.MethodPost, "/proofs", nil)
	req.Header.Set("X-API-Key", "shared-secret")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST with legacy key: %d", rr.Code)
	}
	// A resolved tenant must NOT appear in compat mode.
	req = httptest.NewRequest(http.MethodPost, "/proofs", nil)
	req.Header.Set("X-API-Key", "shared-secret")
	req = req.WithContext(context.Background())
	rr = httptest.NewRecorder()
	var seen *tenant.Tenant
	inner2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = tenantFromCtx(r)
		w.WriteHeader(http.StatusOK)
	})
	s.authMiddleware(inner2).ServeHTTP(rr, req)
	if seen != nil {
		t.Fatalf("compat mode leaked a tenant to handler: %+v", seen)
	}
}
