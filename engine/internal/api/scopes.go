package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
)

// Scoped API keys — RBAC-lite for the demo API.
//
// The legacy single-key auth (env API_KEY) grants blanket write
// access. Scoped keys are the opt-in refinement: a caller registered
// with a specific list of scopes (e.g. ["proofs:write", "verify"])
// can call only endpoints whose declared scope is in that list.
//
// Registration lives in the file $CP_SCOPES_FILE (default:
// unset — falls back to blanket auth for backward compatibility with
// existing deployments). The file format is intentionally boring
// JSON so operators can eyeball / diff it:
//
//     {
//       "keys": [
//         {"key": "sk_prover_...", "tenant_id": "acme", "scopes": ["proofs:*", "verify"]},
//         {"key": "sk_readonly_...", "tenant_id": "monitoring", "scopes": ["proofs:read", "stats"]}
//       ]
//     }
//
// If the file is not present, or a caller presents a key that isn't
// in the registry, the middleware falls back to the legacy check
// against $API_KEY (blanket write access). A caller with a scoped
// key that lacks the required scope for an endpoint gets 403.
//
// The scope grammar is minimal:
//   * "resource:verb"      — proofs:write, verify:*, kyc:read
//   * "resource:*"         — all verbs on that resource
//   * "*"                  — root, all scopes
// Endpoints declare their scope with routeScopes below.

type scopedKey struct {
	Key      string   `json:"key"`
	TenantID string   `json:"tenant_id"`
	Scopes   []string `json:"scopes"`
}

type scopedKeyFile struct {
	Keys []scopedKey `json:"keys"`
}

type scopeRegistry struct {
	mu   sync.RWMutex
	keys map[string]*scopedKey // key value → entry
}

func newScopeRegistry() *scopeRegistry {
	return &scopeRegistry{keys: make(map[string]*scopedKey)}
}

// loadFromFile reads the JSON file and replaces the in-memory
// registry atomically. Missing file → empty registry, no error
// (blanket-auth fallback path).
func (r *scopeRegistry) loadFromFile(path string) error {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()
	var parsed scopedKeyFile
	if err := json.NewDecoder(f).Decode(&parsed); err != nil {
		return err
	}
	next := make(map[string]*scopedKey, len(parsed.Keys))
	for i := range parsed.Keys {
		entry := parsed.Keys[i]
		if entry.Key == "" {
			return errors.New("scope file: empty key entry")
		}
		next[entry.Key] = &entry
	}
	r.mu.Lock()
	r.keys = next
	r.mu.Unlock()
	return nil
}

// lookup returns the scoped-key entry for a given raw key, or nil if
// the key isn't scoped (fallback to blanket auth).
func (r *scopeRegistry) lookup(key string) *scopedKey {
	if key == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.keys[key]
}

// hasScope returns true if `granted` covers `required`. Grammar:
// "*" wins, "resource:*" wins for that resource, exact match wins.
func hasScope(granted []string, required string) bool {
	if required == "" {
		return true
	}
	for _, g := range granted {
		if g == "*" {
			return true
		}
		if g == required {
			return true
		}
		if strings.HasSuffix(g, ":*") {
			prefix := strings.TrimSuffix(g, ":*")
			if strings.HasPrefix(required, prefix+":") {
				return true
			}
		}
	}
	return false
}

// routeScopes maps a normalized endpoint pattern to the scope it
// requires. The lookup key is METHOD + " " + path (after /v1 rewrite).
// A missing entry means the endpoint requires no scope beyond auth
// (health, versioning, etc.).
var routeScopes = map[string]string{
	"POST /proofs":                  "proofs:write",
	"POST /proofs/batch":            "proofs:write",
	"POST /verify":                  "verify",
	"POST /proofs/{id}/revoke":      "proofs:write",
	"GET /proofs":                   "proofs:read",
	"GET /proofs/{id}":              "proofs:read",
	"GET /proofs/{id}/export":       "proofs:read",
	"GET /stats":                    "stats",
	"POST /kyc/check":               "kyc:read",
	"POST /kyc/grant":               "kyc:write",
	"GET /kyc/whitelist/{user}":     "kyc:read",
	"POST /inference/prove":         "inference:write",
	"POST /inference/verify":        "inference:read",
	"POST /inference/register-model": "inference:write",
	"GET /inference/model/{id}":     "inference:read",
	"POST /aggregation/create-batch": "aggregation:write",
	"POST /aggregation/add-proof":   "aggregation:write",
	"POST /aggregation/finalize":    "aggregation:write",
	"GET /aggregation/batch/{id}":   "aggregation:read",
	"GET /aggregation/verify-batch/{id}": "aggregation:read",
	"POST /zk/verify-groth16":       "zk:read",
	"POST /zk/batch-verify":         "zk:read",
	"POST /zk/groth16-real/prove":   "zk:write",
	"POST /zk/groth16-real/verify":  "zk:read",
	"POST /zk/challenge":            "zk:write",
	"GET /zk/challenge/{id}":        "zk:read",
	"POST /proof-chain/validate":    "proofs:read",
	"POST /pq/sign-sphincs":         "pq:write",
	"POST /pq/verify-sphincs":       "pq:read",
	"POST /pq/hybrid-sign":          "pq:write",
	"POST /pq/hybrid-verify":        "pq:read",
	"POST /v1/webhooks":             "webhooks:write",
	"GET /v1/webhooks":              "webhooks:read",
	"DELETE /v1/webhooks/{id}":      "webhooks:write",
	"GET /v1/webhooks/dead-letters": "webhooks:read",
	"POST /v1/webhooks/dead-letters/{delivery_id}/replay": "webhooks:write",

	// Admin dashboard rollup (read-only)
	"GET /v1/admin/summary":            "admin:read",
	"GET /v1/circuits":              "zk:read",
	"GET /v1/circuits/{id}":         "zk:read",
	"GET /v1/circuits/{id}/vk":      "zk:read",
	"POST /v1/zk/prove":             "zk:write",
	"POST /v1/zk/verify":            "zk:read",
	"POST /v1/zk/anchor-verdict":    "zk:write",

	// PQ key rotation + versioning
	"POST /v1/pq/keys":                 "pq:write",
	"POST /v1/pq/keys/{algo}/rotate":   "pq:write",
	"GET /v1/pq/keys":                  "pq:read",
	"GET /v1/pq/keys/{id}":             "pq:read",
	"POST /v1/pq/keys/sign":            "pq:write",
	"POST /v1/pq/keys/verify":          "pq:read",
	"POST /v1/pq/keys/migrate":         "pq:write",
	"GET /v1/pq/keystore/info":         "pq:read",

	// Nova / folding aggregation harness
	"POST /v1/aggregation/fold":        "aggregation:write",
	"POST /v1/aggregation/verify-fold": "aggregation:read",

	// Decision / A2A / HITL pipeline (Pack AQ)
	"POST /v1/decision/evaluate":         "decision:write",
	"GET /v1/decision/pool":              "decision:read",
	"GET /v1/hitl/tickets":               "decision:read",
	"GET /v1/hitl/tickets/{id}":          "decision:read",
	"POST /v1/hitl/tickets/{id}/resolve": "decision:write",
	"POST /v1/receipts/emit":             "receipts:write",
	"GET /v1/receipts/{id}":              "receipts:read",
	"GET /v1/receipts/{id}/lineage":      "receipts:read",
	"GET /v1/receipts/{id}/w3c-vc":       "receipts:read",
	"GET /v1/receipts/{id}/agent-receipt": "receipts:read",
}

// enforceScope checks that the caller (identified by X-API-Key) has
// the required scope for this request. Returns true on allow, false
// on deny (in which case a 403 has already been written). If the
// caller's key is not scoped, we fall through to the pre-existing
// blanket-auth model — that path is checked by authMiddleware.
func (s *Server) enforceScope(w http.ResponseWriter, r *http.Request, required string) bool {
	if s.scopes == nil {
		return true // subsystem disabled → skip
	}
	key := strings.TrimSpace(r.Header.Get("X-API-Key"))
	entry := s.scopes.lookup(key)
	if entry == nil {
		// Unscoped key — blanket auth already covered admission. If
		// blanket auth was disabled (no API_KEY) we allow through
		// (dev mode). If it was enabled the request would not have
		// reached us.
		return true
	}
	if !hasScope(entry.Scopes, required) {
		s.jsonError(w, "forbidden: missing scope "+required, http.StatusForbidden)
		return false
	}
	// Tag the request so downstream loggers can attribute activity.
	r.Header.Set("X-CP-Tenant", entry.TenantID)
	return true
}

// scopeMiddleware inspects the METHOD+path pair and enforces the
// declared scope for it. It runs AFTER version rewrite so the map
// keys never need a /v1 prefix (except for webhooks endpoints, which
// live only under /v1).
//
// Go 1.22+'s ServeMux.Handler(r) returns (handler, pattern) — we use
// pattern to look up the declared scope BEFORE routing so a 403
// short-circuits without ever hitting the handler.
func (s *Server) scopeMiddleware(mux *http.ServeMux) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.scopes == nil {
				next.ServeHTTP(w, r)
				return
			}
			_, pat := mux.Handler(r)
			pat = strings.TrimSpace(pat)
			if pat == "" {
				next.ServeHTTP(w, r)
				return
			}
			required := routeScopes[pat]
			if required == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !s.enforceScope(w, r, required) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
