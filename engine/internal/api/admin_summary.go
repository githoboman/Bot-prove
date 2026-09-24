package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto/keystore"
	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

// AdminSummary is the single-shot payload the FE admin dashboard
// polls. Everything is read-only, aggregated on request, and never
// exposes secrets — the point is to give operators one endpoint that
// answers "what is this engine doing right now?".
//
// Design constraint: no new server state. Every field is computed
// live from existing subsystems.
type AdminSummary struct {
	Version    string           `json:"version"`
	ServerTime time.Time        `json:"server_time"`
	Uptime     string           `json:"uptime"`
	Subsystems SubsystemStatus  `json:"subsystems"`
	Keystore   *keystore.Info   `json:"keystore,omitempty"`
	Keys       []pqcrypto.KeyMeta `json:"keys,omitempty"`
	Webhooks   *WebhookSummary  `json:"webhooks,omitempty"`
	Scopes     *ScopeSummary    `json:"scopes,omitempty"`
	Contracts  map[string]string `json:"contracts"`
}

// SubsystemStatus captures which optional subsystems the engine has
// wired up. Booleans are computed from the same env gates the server
// reads at boot; a subsystem that is off still appears in the map so
// the dashboard can render "disabled" without special-casing.
type SubsystemStatus struct {
	Keyring   bool `json:"keyring"`
	Receipts  bool `json:"receipts"`
	Decision  bool `json:"decision"`
	Quorum    bool `json:"quorum"`
	Scopes    bool `json:"scopes"`
	Postgres  bool `json:"postgres"`
	Metrics   bool `json:"metrics"`
	Webhooks  bool `json:"webhooks"`
}

// WebhookSummary is a compact rollup of webhook subsystem state.
// Owner-agnostic aggregate — per-subscription detail lives at
// /v1/webhooks and /v1/webhooks/dead-letters.
type WebhookSummary struct {
	Subscriptions int `json:"subscriptions"`
	QueueDepth    int `json:"queue_depth"`
	DeadLetters   int `json:"dead_letters"`
	KnownEvents   []string `json:"known_events"`
}

// ScopeSummary reports the loaded scoped-key file state. Never
// includes the keys themselves — those are secrets.
type ScopeSummary struct {
	Loaded      bool     `json:"loaded"`
	KeyCount    int      `json:"key_count"`
	SourcePath  string   `json:"source_path,omitempty"`
}

var serverStartTime = time.Now()

// buildAdminSummary composes the read-only rollup. Nil-safe on every
// optional subsystem — a missing feature just drops out of the
// payload rather than crashing.
func (s *Server) buildAdminSummary() AdminSummary {
	sum := AdminSummary{
		Version:    "post-hackathon/roadmap",
		ServerTime: time.Now().UTC(),
		Uptime:     time.Since(serverStartTime).Round(time.Second).String(),
		Subsystems: SubsystemStatus{
			Keyring:  os.Getenv("CP_KEYRING_ENABLE") == "1",
			Receipts: os.Getenv("CP_RECEIPTS_ENABLE") == "1",
			Decision: os.Getenv("CP_DECISION_ENABLE") == "1",
			Quorum:   os.Getenv("CP_QUORUM_ENABLE") == "1",
			Scopes:   s.scopes != nil,
			Postgres: s.db != nil,
			Metrics:  s.metrics != nil,
			Webhooks: s.webhooks != nil,
		},
		Contracts: map[string]string{
			"proof_registry": s.contracts.ProofRegistry,
		},
	}

	if s.keystore != nil {
		info := s.keystore.Info(context.TODO())
		sum.Keystore = &info
		// List of keys — metadata only, no private key bytes ever
		// leave the keystore's internal storage.
		sum.Keys = s.keystore.List(context.TODO())
	}

	if s.webhooks != nil {
		s.webhooks.mu.RLock()
		sum.Webhooks = &WebhookSummary{
			Subscriptions: len(s.webhooks.subs),
			QueueDepth:    len(s.webhooks.queue),
			DeadLetters:   len(s.webhooks.dead),
			KnownEvents:   append([]string(nil), KnownWebhookEvents...),
		}
		s.webhooks.mu.RUnlock()
	}

	if s.scopes != nil {
		s.scopes.mu.RLock()
		sum.Scopes = &ScopeSummary{
			Loaded:   true,
			KeyCount: len(s.scopes.keys),
			SourcePath: os.Getenv("CP_SCOPED_KEYS_FILE"),
		}
		s.scopes.mu.RUnlock()
	}

	return sum
}

// adminSummaryHandler serves GET /v1/admin/summary. Requires the
// "admin:read" scope when scoped-keys are enabled; falls back to
// open access when the file is not configured (matches the wider
// blanket-auth fallback pattern).
func (s *Server) adminSummaryHandler(w http.ResponseWriter, r *http.Request) {
	if !s.enforceScope(w, r, "admin:read") {
		return
	}
	sum := s.buildAdminSummary()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(sum)
}
