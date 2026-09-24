// Package api — scope allowlist middleware.
//
// Extends the single-shared-secret X-API-Key model into a lightweight
// role model: each API key can be tagged with one or more *scopes*, and
// each protected route declares the scopes it requires. Requests
// carrying an X-API-Key that is missing any required scope are rejected
// with 403 forbidden. The middleware sits inside authMiddleware — auth
// runs first, scope second.
//
// Scopes are string tokens the operator chooses (e.g. "proof:write",
// "kyc:admin"). They are configured out-of-band from an in-memory
// registry populated at startup, keyed by the api-key value. When no
// registry is configured, all authenticated requests are allowed —
// preserves the local-dev / demo default.
//
// Closes: 7.11 (role allowlist / scope middleware — API-key → {scopes})
package api

import (
	"context"
	"net/http"
	"sync"
)

// APIKeyRecord is the row stored per key value in the scope registry.
// TenantID is a free-form label kept alongside so audit logs can
// name the caller without leaking the secret.
type APIKeyRecord struct {
	TenantID string
	Scopes   []string
}

// ScopeRegistry maps an X-API-Key value → its authorized scopes.
// The zero-value registry contains no keys and enforces nothing (the
// legacy single-shared-secret path continues to work). Populate via
// Set / SetMany at startup from a config source of your choosing.
type ScopeRegistry struct {
	mu    sync.RWMutex
	byKey map[string]APIKeyRecord
}

// NewScopeRegistry constructs an empty registry.
func NewScopeRegistry() *ScopeRegistry {
	return &ScopeRegistry{byKey: map[string]APIKeyRecord{}}
}

// Set upserts a record for one API key.
func (r *ScopeRegistry) Set(key string, rec APIKeyRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byKey == nil {
		r.byKey = map[string]APIKeyRecord{}
	}
	r.byKey[key] = rec
}

// SetMany bulk-loads records. Overwrites any existing entry.
func (r *ScopeRegistry) SetMany(rows map[string]APIKeyRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byKey == nil {
		r.byKey = map[string]APIKeyRecord{}
	}
	for k, v := range rows {
		r.byKey[k] = v
	}
}

// Lookup returns the record for an API key. `found=false` means no
// scope constraint applies to this key (fall through to auth-only).
func (r *ScopeRegistry) Lookup(key string) (APIKeyRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.byKey[key]
	return rec, ok
}

// HasScope reports whether the given record grants a specific scope.
// "*" wildcard grants every scope (super-admin key).
func (r APIKeyRecord) HasScope(want string) bool {
	for _, s := range r.Scopes {
		if s == "*" || s == want {
			return true
		}
	}
	return false
}

// contextKey is unexported so callers can't inject fake tenants.
type contextKey int

const (
	ctxTenantID contextKey = iota
)

// TenantIDFromContext pulls the tenant label the scope middleware
// attached. Returns "" when no scoped key was matched (either scope
// enforcement was off, or the API key had no registry row).
func TenantIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxTenantID).(string); ok {
		return v
	}
	return ""
}

// scopeGate wraps a handler with a required-scope check. Returns the
// wrapping handler unchanged when the registry is nil or empty — i.e.
// the middleware is a no-op unless the operator opts in by populating
// the registry at startup.
func (s *Server) scopeGate(required string, next http.Handler) http.Handler {
	if s.scopeReg == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			// Read-only routes don't require scope enforcement — same
			// policy the auth middleware follows.
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get("X-API-Key")
		rec, ok := s.scopeReg.Lookup(key)
		if !ok {
			// Key has no registry row → scope enforcement doesn't apply.
			// (auth middleware already validated the key value itself.)
			next.ServeHTTP(w, r)
			return
		}
		if !rec.HasScope(required) {
			s.jsonError(w, "api key lacks required scope: "+required, http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), ctxTenantID, rec.TenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
