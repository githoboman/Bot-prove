// Package tenant implements namespace-per-tenant isolation, per-tenant
// quota + rate limiting, per-tenant API-key rotation, and an in-memory
// audit trail for every tenant-observable event (BA / backlog 10.1 +
// 10.2).
//
// # Design
//
// A tenant is a first-class caller identity. Every mutating API
// request carries an X-API-Key header; the store maps that key to
// exactly one tenant. A tenant has:
//
//   - id (short, human-readable, immutable — e.g. "acme_prod")
//   - display_name (human, mutable)
//   - one or more active API keys (rotation-friendly; old-and-new can
//     coexist for a grace window so callers don't outage during roll)
//   - a per-second and per-minute rate ceiling
//   - a monthly proof-write quota (resets on the wall-clock month
//     boundary in UTC)
//   - an isolated namespace prefix that scopes every derived record
//     (webhook subscriptions, proofs, batches, KYC entries, …)
//   - an append-only audit log (in-memory, ring-buffered)
//
// The store is intentionally in-memory + JSON-file backed. That
// matches the rest of the engine's design posture (single-node,
// non-durable state) and keeps the hackathon submission honest — a
// durable multi-tenant store is a follow-up tracked in
// KNOWN_LIMITATIONS.md.
//
// # Backwards compatibility
//
// If no TENANTS_FILE env var is set (or the file is empty), the store
// operates in "single-tenant compat" mode: all requests are attributed
// to a synthetic tenant id "_default" and existing tests / callers
// keep working unchanged. Multi-tenant behaviour is opt-in.
//
// # Honesty ladder
//
// REAL — every guard listed above is enforced in code and covered by
// tests. The audit log is a real append-only ring, not a stub.
// NOT-DURABLE — process restart flushes rate counters, quota
// counters, and the audit log. Persisting them is post-hackathon.
// NOT-ON-CHAIN — tenants are a Service-layer concept; nothing about
// them is anchored.
// NO-PAID-SERVICES — pure Go, no new module deps.
package tenant

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// -----------------------------------------------------------------------------
// Model
// -----------------------------------------------------------------------------

// Tenant is one caller identity. Zero value is not usable — always
// construct via Store.Add.
type Tenant struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Namespace   string    `json:"namespace"`
	CreatedAt   time.Time `json:"created_at"`

	// KeyHashes are SHA-256(hex) of every active API key. We never
	// store the raw key. Rotation adds a new hash, then (after the
	// grace window) revokes the old one.
	KeyHashes []string `json:"key_hashes"`

	// RatePerSecond and RatePerMinute are hard ceilings enforced by
	// the surrounding rate limiter. Values <=0 mean "no ceiling",
	// which is a deliberate escape hatch, not the default.
	RatePerSecond int `json:"rate_per_second"`
	RatePerMinute int `json:"rate_per_minute"`

	// MonthlyProofQuota is the maximum number of proof-write attempts
	// (POST /proofs, POST /proofs/batch) allowed per UTC month. <=0
	// means "unlimited". The counter is not persisted — process
	// restart resets it.
	MonthlyProofQuota int `json:"monthly_proof_quota"`
}

// The synthetic tenant every request falls back to when the store is
// in single-tenant compat mode (no TENANTS_FILE, empty registry).
const DefaultTenantID = "_default"

// -----------------------------------------------------------------------------
// Store
// -----------------------------------------------------------------------------

// Store holds every tenant and their runtime counters. Safe for
// concurrent use.
type Store struct {
	mu       sync.RWMutex
	tenants  map[string]*Tenant   // by id
	byKey    map[string]string    // sha256(key) -> tenant id
	rateSec  map[string]*window   // tenant id -> per-second window
	rateMin  map[string]*window   // tenant id -> per-minute window
	quotaMon map[string]*monthCtr // tenant id -> monthly counter
	audit    []AuditEvent
	auditCap int
	now      func() time.Time
}

// window is a fixed-window rate counter. We deliberately pick a
// fixed window over a sliding one — for the hackathon threshold it's
// simpler, deterministic to test, and the "half-second-of-burst"
// pathology is bounded by the per-minute ceiling.
type window struct {
	start time.Time
	count int
}

type monthCtr struct {
	year  int
	month time.Month
	count int
}

// NewStore constructs an empty store. Callers wire it up via Add or
// LoadFile.
func NewStore() *Store {
	return &Store{
		tenants:  make(map[string]*Tenant),
		byKey:    make(map[string]string),
		rateSec:  make(map[string]*window),
		rateMin:  make(map[string]*window),
		quotaMon: make(map[string]*monthCtr),
		auditCap: 4096,
		now:      time.Now,
	}
}

// LoadFile reads a JSON file with schema:
//
//	{ "tenants": [ { "id":..., "display_name":..., ... } ] }
//
// The API keys are provided inline as a "keys" array of raw strings;
// they are hashed on load and dropped from memory (only the hashes
// survive). Missing file / empty file leaves the store empty.
func (s *Store) LoadFile(path string) error {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read tenants file: %w", err)
	}
	if len(b) == 0 {
		return nil
	}
	var payload struct {
		Tenants []struct {
			Tenant
			Keys []string `json:"keys"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return fmt.Errorf("parse tenants file: %w", err)
	}
	for _, t := range payload.Tenants {
		if t.ID == "" || len(t.Keys) == 0 {
			return fmt.Errorf("tenant %q: id and >=1 key required", t.ID)
		}
		hashes := make([]string, 0, len(t.Keys))
		for _, k := range t.Keys {
			hashes = append(hashes, HashKey(k))
		}
		src := t.Tenant // copy
		src.KeyHashes = hashes
		if src.Namespace == "" {
			src.Namespace = "ns_" + t.ID
		}
		if src.CreatedAt.IsZero() {
			src.CreatedAt = s.now()
		}
		if err := s.Add(&src); err != nil {
			return fmt.Errorf("add tenant %q: %w", t.ID, err)
		}
	}
	return nil
}

// Add registers a new tenant. The tenant's KeyHashes must already be
// set (raw keys are never accepted by this function — hash first).
func (s *Store) Add(t *Tenant) error {
	if t == nil || t.ID == "" {
		return errors.New("tenant id required")
	}
	if strings.ContainsAny(t.ID, " \t\n/\\") {
		return errors.New("tenant id must be a bare identifier")
	}
	if len(t.KeyHashes) == 0 {
		return errors.New("tenant must have at least one key hash")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tenants[t.ID]; exists {
		return fmt.Errorf("tenant %q already exists", t.ID)
	}
	for _, h := range t.KeyHashes {
		if other, taken := s.byKey[h]; taken {
			return fmt.Errorf("key already registered to tenant %q", other)
		}
	}
	if t.Namespace == "" {
		t.Namespace = "ns_" + t.ID
	}
	s.tenants[t.ID] = t
	for _, h := range t.KeyHashes {
		s.byKey[h] = t.ID
	}
	s.appendAuditLocked(AuditEvent{
		At:       s.now(),
		TenantID: t.ID,
		Kind:     AuditCreated,
		Detail:   fmt.Sprintf("namespace=%s keys=%d", t.Namespace, len(t.KeyHashes)),
	})
	return nil
}

// Resolve returns the tenant that owns rawKey, or nil if none do. In
// single-tenant compat mode (no tenants registered at all), it always
// returns the synthetic _default tenant so callers keep working.
func (s *Store) Resolve(rawKey string) *Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.tenants) == 0 {
		return &Tenant{ID: DefaultTenantID, Namespace: "ns_" + DefaultTenantID}
	}
	if rawKey == "" {
		return nil
	}
	id, ok := s.byKey[HashKey(rawKey)]
	if !ok {
		return nil
	}
	return s.tenants[id]
}

// List returns every tenant, sorted by id, with the KeyHashes stripped
// so a caller with tenant-admin scope cannot inadvertently leak them
// via a list handler.
func (s *Store) List() []Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Tenant, 0, len(s.tenants))
	for _, t := range s.tenants {
		cp := *t
		cp.KeyHashes = nil
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Namespace returns the namespace prefix for a tenant id, falling
// back to the default if the tenant is not registered. Used by the
// server to scope subscription lists, proof lookups, etc.
func (s *Store) Namespace(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if t, ok := s.tenants[id]; ok {
		return t.Namespace
	}
	return "ns_" + DefaultTenantID
}

// -----------------------------------------------------------------------------
// Rate + quota checks
// -----------------------------------------------------------------------------

// Decision is the outcome of a tenant-scoped rate + quota check.
type Decision struct {
	Allowed bool
	Reason  string // populated when Allowed=false; safe to expose to callers.
}

// CheckRate books one request against the tenant's per-second and
// per-minute windows. Returns Decision{Allowed:false} with a reason
// string safe to return to the caller.
func (s *Store) CheckRate(tenantID string) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[tenantID]
	if !ok {
		// Compat mode / unknown tenant — no rate ceiling.
		return Decision{Allowed: true}
	}
	now := s.now()
	if t.RatePerSecond > 0 {
		w, ok := s.rateSec[tenantID]
		if !ok || now.Sub(w.start) >= time.Second {
			s.rateSec[tenantID] = &window{start: now, count: 1}
		} else {
			w.count++
			if w.count > t.RatePerSecond {
				return Decision{Allowed: false, Reason: "rate limit per second exceeded"}
			}
		}
	}
	if t.RatePerMinute > 0 {
		w, ok := s.rateMin[tenantID]
		if !ok || now.Sub(w.start) >= time.Minute {
			s.rateMin[tenantID] = &window{start: now, count: 1}
		} else {
			w.count++
			if w.count > t.RatePerMinute {
				return Decision{Allowed: false, Reason: "rate limit per minute exceeded"}
			}
		}
	}
	return Decision{Allowed: true}
}

// CheckAndConsumeMonthlyQuota books one proof-write against the
// tenant's monthly quota. The counter resets on the first request
// received in a new UTC month.
func (s *Store) CheckAndConsumeMonthlyQuota(tenantID string) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[tenantID]
	if !ok || t.MonthlyProofQuota <= 0 {
		return Decision{Allowed: true}
	}
	now := s.now().UTC()
	c, ok := s.quotaMon[tenantID]
	if !ok || c.year != now.Year() || c.month != now.Month() {
		c = &monthCtr{year: now.Year(), month: now.Month()}
		s.quotaMon[tenantID] = c
	}
	c.count++
	if c.count > t.MonthlyProofQuota {
		c.count-- // do not book a rejected request
		return Decision{Allowed: false, Reason: "monthly proof quota exceeded"}
	}
	return Decision{Allowed: true}
}

// -----------------------------------------------------------------------------
// Rotation
// -----------------------------------------------------------------------------

// RotateAddKey adds a new API key to the tenant. Both old and new
// keys work until RotateRevokeOldKeys is called; that grace window is
// the whole point of the two-phase design.
func (s *Store) RotateAddKey(tenantID, newRawKey string) error {
	if newRawKey == "" {
		return errors.New("empty key")
	}
	h := HashKey(newRawKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[tenantID]
	if !ok {
		return fmt.Errorf("tenant %q not found", tenantID)
	}
	if _, taken := s.byKey[h]; taken {
		return errors.New("key already registered")
	}
	t.KeyHashes = append(t.KeyHashes, h)
	s.byKey[h] = tenantID
	s.appendAuditLocked(AuditEvent{
		At:       s.now(),
		TenantID: tenantID,
		Kind:     AuditKeyAdded,
		Detail:   fmt.Sprintf("keys_active=%d", len(t.KeyHashes)),
	})
	return nil
}

// RotateRevokeOldKeys keeps only the newest keepLast key hashes on
// the tenant and drops the rest. keepLast<1 is treated as 1 (a
// tenant with zero live keys is uncallable and would be a mistake).
func (s *Store) RotateRevokeOldKeys(tenantID string, keepLast int) error {
	if keepLast < 1 {
		keepLast = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[tenantID]
	if !ok {
		return fmt.Errorf("tenant %q not found", tenantID)
	}
	if len(t.KeyHashes) <= keepLast {
		return nil
	}
	drop := t.KeyHashes[:len(t.KeyHashes)-keepLast]
	keep := t.KeyHashes[len(t.KeyHashes)-keepLast:]
	for _, h := range drop {
		delete(s.byKey, h)
	}
	t.KeyHashes = append([]string(nil), keep...)
	s.appendAuditLocked(AuditEvent{
		At:       s.now(),
		TenantID: tenantID,
		Kind:     AuditKeyRevoked,
		Detail:   fmt.Sprintf("dropped=%d kept=%d", len(drop), len(keep)),
	})
	return nil
}

// -----------------------------------------------------------------------------
// Audit
// -----------------------------------------------------------------------------

type AuditKind string

const (
	AuditCreated      AuditKind = "tenant.created"
	AuditKeyAdded     AuditKind = "tenant.key.added"
	AuditKeyRevoked   AuditKind = "tenant.key.revoked"
	AuditRateBlocked  AuditKind = "tenant.rate.blocked"
	AuditQuotaBlocked AuditKind = "tenant.quota.blocked"
	AuditAuthAccepted AuditKind = "tenant.auth.accepted"
	AuditAuthRejected AuditKind = "tenant.auth.rejected"
)

type AuditEvent struct {
	At       time.Time `json:"at"`
	TenantID string    `json:"tenant_id"`
	Kind     AuditKind `json:"kind"`
	Detail   string    `json:"detail,omitempty"`
}

// Log records an audit event. Safe for concurrent use.
func (s *Store) Log(ev AuditEvent) {
	if ev.At.IsZero() {
		ev.At = s.now()
	}
	s.mu.Lock()
	s.appendAuditLocked(ev)
	s.mu.Unlock()
}

// appendAuditLocked assumes s.mu is held (writer).
func (s *Store) appendAuditLocked(ev AuditEvent) {
	if ev.At.IsZero() {
		ev.At = s.now()
	}
	s.audit = append(s.audit, ev)
	if len(s.audit) > s.auditCap {
		// Drop oldest — ring buffer.
		s.audit = s.audit[len(s.audit)-s.auditCap:]
	}
}

// Audit returns a snapshot of every recorded event, optionally
// scoped to one tenant when tenantID != "".
func (s *Store) Audit(tenantID string) []AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AuditEvent, 0, len(s.audit))
	for _, e := range s.audit {
		if tenantID == "" || e.TenantID == tenantID {
			out = append(out, e)
		}
	}
	return out
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// HashKey returns the SHA-256(hex) of a raw API key. Deliberately
// SHA-256 and not a memory-hard KDF: API keys are already
// high-entropy random strings by convention, and the perf budget for
// per-request hashing is on the microsecond scale.
func HashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// SetNowForTest lets tests substitute a deterministic clock. Not
// exported for production callers.
func (s *Store) SetNowForTest(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}
