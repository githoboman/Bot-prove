package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAdminSummary_Shape asserts the endpoint returns a well-formed
// rollup for a default (memory-backed) server. We don't spin up a
// full ProofEngine — the summary code takes what it can from the
// server struct and drops missing subsystems out of the payload.
func TestAdminSummary_Shape(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/summary", nil)

	s.adminSummaryHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	var sum AdminSummary
	if err := json.NewDecoder(rec.Body).Decode(&sum); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Version and server_time are always present.
	if sum.Version == "" {
		t.Error("version missing")
	}
	if sum.ServerTime.IsZero() {
		t.Error("server_time zero")
	}
	if sum.Uptime == "" {
		t.Error("uptime missing")
	}
	// Contracts map is always present, even if empty.
	if sum.Contracts == nil {
		t.Error("contracts map missing")
	}
	// Subsystems map always exists.
	// (Every field is a bool; we just need the struct to have been
	// populated — non-nil is implicit from the value type.)

	// On a bare Server (no wired keystore/webhooks/scopes/db), those
	// optional fields must be absent from the payload rather than
	// zero-valued.
	if sum.Keystore != nil {
		t.Errorf("expected keystore to be omitted on empty Server; got %+v", sum.Keystore)
	}
	if sum.Webhooks != nil {
		t.Errorf("expected webhooks to be omitted on empty Server; got %+v", sum.Webhooks)
	}
	if sum.Scopes != nil {
		t.Errorf("expected scopes to be omitted on empty Server; got %+v", sum.Scopes)
	}
}

// TestAdminSummary_WithWebhooks wires a real webhookStore into the
// server and confirms the rollup reflects its state.
func TestAdminSummary_WithWebhooks(t *testing.T) {
	s := &Server{webhooks: newWebhookStore()}

	// Seed one subscription (via public register).
	_, err := s.webhooks.register("owner-hash", "https://example.test/hook", "shh", []string{"proof.verified"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/summary", nil)
	s.adminSummaryHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var sum AdminSummary
	if err := json.NewDecoder(rec.Body).Decode(&sum); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sum.Webhooks == nil {
		t.Fatal("expected webhooks summary")
	}
	if sum.Webhooks.Subscriptions != 1 {
		t.Errorf("Subscriptions = %d, want 1", sum.Webhooks.Subscriptions)
	}
	if sum.Webhooks.QueueDepth != 0 {
		t.Errorf("QueueDepth = %d, want 0", sum.Webhooks.QueueDepth)
	}
	if sum.Webhooks.DeadLetters != 0 {
		t.Errorf("DeadLetters = %d, want 0", sum.Webhooks.DeadLetters)
	}
	if len(sum.Webhooks.KnownEvents) != len(KnownWebhookEvents) {
		t.Errorf("KnownEvents len = %d, want %d",
			len(sum.Webhooks.KnownEvents), len(KnownWebhookEvents))
	}
}

// TestAdminSummary_NoSecretsInPayload guards against an easy mistake
// — someone extending buildAdminSummary and accidentally including
// key material or secret headers. Serialise the payload and grep for
// obvious leak markers.
func TestAdminSummary_NoSecretsInPayload(t *testing.T) {
	s := &Server{webhooks: newWebhookStore()}
	// register a subscription with a secret; the secret must never
	// appear in the summary.
	const knownSecret = "top-secret-hmac-xyz"
	_, _ = s.webhooks.register("owner-hash", "https://example.test/hook", knownSecret, []string{"proof.anchored"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/summary", nil)
	s.adminSummaryHandler(rec, req)
	body := rec.Body.String()

	for _, marker := range []string{
		knownSecret,
		"BEGIN PRIVATE KEY", "BEGIN EC PRIVATE",
		"X-API-Key", "authorization",
	} {
		if strings.Contains(body, marker) {
			t.Errorf("admin summary leaked %q: %s", marker, body)
		}
	}
}
