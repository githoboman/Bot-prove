package tenant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

func newTestStore(t *testing.T, now time.Time) *Store {
	t.Helper()
	s := NewStore()
	fixed := now
	s.SetNowForTest(func() time.Time { return fixed })
	return s
}

func mkTenant(id string, keys []string, opts func(*Tenant)) *Tenant {
	t := &Tenant{
		ID:          id,
		DisplayName: strings.ToUpper(id),
		Namespace:   "ns_" + id,
		KeyHashes:   make([]string, 0, len(keys)),
	}
	for _, k := range keys {
		t.KeyHashes = append(t.KeyHashes, HashKey(k))
	}
	if opts != nil {
		opts(t)
	}
	return t
}

// -----------------------------------------------------------------------------
// Add + Resolve
// -----------------------------------------------------------------------------

func TestAddAndResolve(t *testing.T) {
	s := newTestStore(t, time.Now())
	tn := mkTenant("acme", []string{"key-a", "key-b"}, nil)
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := s.Resolve("key-a"); got == nil || got.ID != "acme" {
		t.Fatalf("Resolve(key-a) = %+v", got)
	}
	if got := s.Resolve("key-b"); got == nil || got.ID != "acme" {
		t.Fatalf("Resolve(key-b) = %+v", got)
	}
	if got := s.Resolve("key-none"); got != nil {
		t.Fatalf("Resolve(key-none) = %+v, want nil", got)
	}
}

func TestCompatModeReturnsDefaultTenant(t *testing.T) {
	s := newTestStore(t, time.Now())
	got := s.Resolve("anything")
	if got == nil || got.ID != DefaultTenantID {
		t.Fatalf("compat resolve = %+v", got)
	}
	// Once we register a real tenant, compat mode is off — unknown
	// keys should return nil.
	tn := mkTenant("acme", []string{"key-a"}, nil)
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := s.Resolve("unknown"); got != nil {
		t.Fatalf("post-registration unknown key: %+v, want nil", got)
	}
}

func TestAddRejectsCollisions(t *testing.T) {
	s := newTestStore(t, time.Now())
	tn := mkTenant("acme", []string{"key-a"}, nil)
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add(mkTenant("acme", []string{"key-b"}, nil)); err == nil {
		t.Fatal("expected duplicate tenant id error")
	}
	// Same key on a different tenant should also fail.
	other := mkTenant("beta", []string{"key-a"}, nil)
	if err := s.Add(other); err == nil {
		t.Fatal("expected duplicate key error")
	}
}

func TestAddRejectsBadInput(t *testing.T) {
	s := newTestStore(t, time.Now())
	cases := []struct {
		name string
		in   *Tenant
	}{
		{"nil", nil},
		{"empty id", &Tenant{ID: "", KeyHashes: []string{HashKey("k")}}},
		{"whitespace id", &Tenant{ID: "bad id", KeyHashes: []string{HashKey("k")}}},
		{"no keys", &Tenant{ID: "acme"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := s.Add(c.in); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// -----------------------------------------------------------------------------
// LoadFile
// -----------------------------------------------------------------------------

func TestLoadFileSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenants.json")
	payload := map[string]any{
		"tenants": []map[string]any{
			{
				"id":                  "acme",
				"display_name":        "ACME Corp",
				"rate_per_second":     10,
				"rate_per_minute":     300,
				"monthly_proof_quota": 1000,
				"keys":                []string{"raw-secret-a", "raw-secret-b"},
			},
			{
				"id":   "beta",
				"keys": []string{"other-secret"},
			},
		},
	}
	b, _ := json.Marshal(payload)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := newTestStore(t, time.Now())
	if err := s.LoadFile(path); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := s.Resolve("raw-secret-a"); got == nil || got.ID != "acme" || got.RatePerSecond != 10 {
		t.Fatalf("acme resolve: %+v", got)
	}
	if got := s.Resolve("other-secret"); got == nil || got.ID != "beta" {
		t.Fatalf("beta resolve: %+v", got)
	}
	// Raw keys must not survive load — grep the store's tenants for
	// any presence.
	for _, tn := range s.tenants {
		for _, h := range tn.KeyHashes {
			if h == "raw-secret-a" || h == "other-secret" {
				t.Fatalf("raw key survived load: %q", h)
			}
			if len(h) != 64 {
				t.Fatalf("key hash length = %d, want 64 hex", len(h))
			}
		}
	}
}

func TestLoadFileMissingFileIsQuiet(t *testing.T) {
	s := newTestStore(t, time.Now())
	if err := s.LoadFile("/nonexistent/does/not/exist.json"); err != nil {
		t.Fatalf("expected nil on missing file, got %v", err)
	}
}

func TestLoadFileEmptyPathIsQuiet(t *testing.T) {
	s := newTestStore(t, time.Now())
	if err := s.LoadFile(""); err != nil {
		t.Fatalf("expected nil on empty path, got %v", err)
	}
}

func TestLoadFileRejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path, []byte(`{ this is not json`), 0o600)
	s := newTestStore(t, time.Now())
	if err := s.LoadFile(path); err == nil {
		t.Fatal("expected parse error")
	}
}

// -----------------------------------------------------------------------------
// Rate limiter
// -----------------------------------------------------------------------------

func TestRateLimiterPerSecond(t *testing.T) {
	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t, fixed)
	tn := mkTenant("acme", []string{"k"}, func(tn *Tenant) {
		tn.RatePerSecond = 3
	})
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	for i := 0; i < 3; i++ {
		if d := s.CheckRate("acme"); !d.Allowed {
			t.Fatalf("request %d: unexpected reject %+v", i, d)
		}
	}
	if d := s.CheckRate("acme"); d.Allowed {
		t.Fatal("expected 4th request to be rate-limited")
	}
	// Advance one full second — window rolls, first request in the
	// new window is accepted.
	s.SetNowForTest(func() time.Time { return fixed.Add(time.Second) })
	if d := s.CheckRate("acme"); !d.Allowed {
		t.Fatalf("post-roll: %+v", d)
	}
}

func TestRateLimiterPerMinute(t *testing.T) {
	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t, fixed)
	tn := mkTenant("acme", []string{"k"}, func(tn *Tenant) {
		tn.RatePerMinute = 5
	})
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	for i := 0; i < 5; i++ {
		if d := s.CheckRate("acme"); !d.Allowed {
			t.Fatalf("request %d: %+v", i, d)
		}
	}
	if d := s.CheckRate("acme"); d.Allowed {
		t.Fatal("expected 6th request to hit per-minute cap")
	}
}

func TestRateLimiterZeroMeansNoCeiling(t *testing.T) {
	s := newTestStore(t, time.Now())
	tn := mkTenant("acme", []string{"k"}, nil) // both rates left 0
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	for i := 0; i < 1000; i++ {
		if d := s.CheckRate("acme"); !d.Allowed {
			t.Fatalf("request %d unexpectedly rate-limited: %+v", i, d)
		}
	}
}

func TestRateLimiterUnknownTenant(t *testing.T) {
	s := newTestStore(t, time.Now())
	// No tenants registered. Compat mode — check is allowed.
	if d := s.CheckRate("does-not-exist"); !d.Allowed {
		t.Fatalf("compat mode should allow: %+v", d)
	}
}

// -----------------------------------------------------------------------------
// Monthly quota
// -----------------------------------------------------------------------------

func TestMonthlyQuota(t *testing.T) {
	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t, fixed)
	tn := mkTenant("acme", []string{"k"}, func(tn *Tenant) {
		tn.MonthlyProofQuota = 3
	})
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	for i := 0; i < 3; i++ {
		if d := s.CheckAndConsumeMonthlyQuota("acme"); !d.Allowed {
			t.Fatalf("req %d: %+v", i, d)
		}
	}
	if d := s.CheckAndConsumeMonthlyQuota("acme"); d.Allowed {
		t.Fatal("expected quota exhaustion on 4th write")
	}
	// Advance to next UTC month — counter resets.
	next := time.Date(2026, 8, 1, 0, 0, 1, 0, time.UTC)
	s.SetNowForTest(func() time.Time { return next })
	if d := s.CheckAndConsumeMonthlyQuota("acme"); !d.Allowed {
		t.Fatalf("post-month-roll: %+v", d)
	}
}

func TestMonthlyQuotaUnknownTenantAllowed(t *testing.T) {
	s := newTestStore(t, time.Now())
	if d := s.CheckAndConsumeMonthlyQuota("nobody"); !d.Allowed {
		t.Fatalf("compat quota check should allow: %+v", d)
	}
}

// -----------------------------------------------------------------------------
// Rotation
// -----------------------------------------------------------------------------

func TestRotateAddKeyGraceWindow(t *testing.T) {
	s := newTestStore(t, time.Now())
	tn := mkTenant("acme", []string{"old-key"}, nil)
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.RotateAddKey("acme", "new-key"); err != nil {
		t.Fatalf("RotateAddKey: %v", err)
	}
	// Both keys resolve to the same tenant during the grace window.
	if got := s.Resolve("old-key"); got == nil || got.ID != "acme" {
		t.Fatalf("old-key: %+v", got)
	}
	if got := s.Resolve("new-key"); got == nil || got.ID != "acme" {
		t.Fatalf("new-key: %+v", got)
	}
	if err := s.RotateRevokeOldKeys("acme", 1); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if got := s.Resolve("old-key"); got != nil {
		t.Fatalf("old-key should not resolve after revoke: %+v", got)
	}
	if got := s.Resolve("new-key"); got == nil {
		t.Fatal("new-key must survive")
	}
}

func TestRotateNeverEmpty(t *testing.T) {
	// Even a caller passing keepLast=0 must leave the tenant with
	// at least one key — otherwise the tenant becomes unreachable.
	s := newTestStore(t, time.Now())
	if err := s.Add(mkTenant("acme", []string{"k1", "k2", "k3"}, nil)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.RotateRevokeOldKeys("acme", 0); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if got := s.Resolve("k3"); got == nil || got.ID != "acme" {
		t.Fatalf("newest key should survive keepLast=0 : %+v", got)
	}
}

func TestRotateRejectsUnknownTenant(t *testing.T) {
	s := newTestStore(t, time.Now())
	if err := s.RotateAddKey("nobody", "new"); err == nil {
		t.Fatal("expected error for unknown tenant")
	}
	if err := s.RotateRevokeOldKeys("nobody", 1); err == nil {
		t.Fatal("expected error for unknown tenant")
	}
}

func TestRotateRejectsDuplicateKey(t *testing.T) {
	s := newTestStore(t, time.Now())
	if err := s.Add(mkTenant("acme", []string{"k1"}, nil)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add(mkTenant("beta", []string{"k2"}, nil)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.RotateAddKey("acme", "k2"); err == nil {
		t.Fatal("expected error: k2 already registered")
	}
}

// -----------------------------------------------------------------------------
// Audit log
// -----------------------------------------------------------------------------

func TestAuditLogCapturesLifecycle(t *testing.T) {
	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t, fixed)
	if err := s.Add(mkTenant("acme", []string{"k1"}, nil)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.RotateAddKey("acme", "k2"); err != nil {
		t.Fatalf("RotateAddKey: %v", err)
	}
	if err := s.RotateRevokeOldKeys("acme", 1); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	s.Log(AuditEvent{TenantID: "acme", Kind: AuditAuthAccepted, Detail: "req_id=abc"})
	events := s.Audit("acme")
	if len(events) != 4 {
		t.Fatalf("audit len = %d, want 4", len(events))
	}
	wantKinds := []AuditKind{AuditCreated, AuditKeyAdded, AuditKeyRevoked, AuditAuthAccepted}
	for i, k := range wantKinds {
		if events[i].Kind != k {
			t.Fatalf("event %d kind = %q, want %q", i, events[i].Kind, k)
		}
	}
}

func TestAuditLogRingBuffer(t *testing.T) {
	s := newTestStore(t, time.Now())
	s.auditCap = 8
	for i := 0; i < 20; i++ {
		s.Log(AuditEvent{TenantID: "acme", Kind: AuditAuthAccepted})
	}
	if got := s.Audit(""); len(got) != 8 {
		t.Fatalf("ring buffer len = %d, want 8", len(got))
	}
}

func TestAuditLogFilterByTenant(t *testing.T) {
	s := newTestStore(t, time.Now())
	s.Log(AuditEvent{TenantID: "acme", Kind: AuditAuthAccepted})
	s.Log(AuditEvent{TenantID: "beta", Kind: AuditAuthAccepted})
	s.Log(AuditEvent{TenantID: "acme", Kind: AuditRateBlocked})
	if got := s.Audit("acme"); len(got) != 2 {
		t.Fatalf("filtered = %d, want 2", len(got))
	}
	if got := s.Audit(""); len(got) != 3 {
		t.Fatalf("unfiltered = %d, want 3", len(got))
	}
}

// -----------------------------------------------------------------------------
// Namespace + List
// -----------------------------------------------------------------------------

func TestNamespaceScopedToTenant(t *testing.T) {
	s := newTestStore(t, time.Now())
	if err := s.Add(mkTenant("acme", []string{"k"}, nil)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := s.Namespace("acme"); got != "ns_acme" {
		t.Fatalf("namespace = %q, want ns_acme", got)
	}
	if got := s.Namespace("unknown"); got != "ns_"+DefaultTenantID {
		t.Fatalf("unknown namespace = %q, want compat default", got)
	}
}

func TestListStripsKeyHashes(t *testing.T) {
	s := newTestStore(t, time.Now())
	tn := mkTenant("acme", []string{"k1", "k2"}, nil)
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}
	if len(list[0].KeyHashes) != 0 {
		t.Fatalf("List() leaked key hashes: %+v", list[0].KeyHashes)
	}
}

// -----------------------------------------------------------------------------
// Concurrency smoke — race under -race would catch bad locking
// -----------------------------------------------------------------------------

func TestConcurrentRateAndAudit(t *testing.T) {
	s := newTestStore(t, time.Now())
	tn := mkTenant("acme", []string{"k"}, func(tn *Tenant) {
		tn.RatePerSecond = 100000
	})
	if err := s.Add(tn); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				s.CheckRate("acme")
				s.Log(AuditEvent{TenantID: "acme", Kind: AuditAuthAccepted})
			}
		}()
	}
	wg.Wait()
	// No assertion beyond "did not race / did not deadlock". A
	// smoke test — the ring buffer and locking correctness is the
	// point.
	_ = s.Audit("acme")
}
