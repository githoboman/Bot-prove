package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// HTTP surface for webhook management.
//
// Routes are registered on `/v1/webhooks/*` (never on the legacy
// unversioned prefix — webhooks are a post-v1 feature, no deprecation
// noise needed).

type webhookRegisterRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events"`
}

type webhookRegisterResponse struct {
	Subscription *webhookSubscription `json:"subscription"`
}

type webhookListResponse struct {
	Subscriptions []*webhookSubscription `json:"subscriptions"`
	Count         int                    `json:"count"`
}

type webhookDeadLetterResponse struct {
	DeadLetters []*deadLetter `json:"dead_letters"`
	Count       int           `json:"count"`
}

// registerWebhookRoutes wires webhook endpoints onto the mux. Called
// from Server.Start.
func (s *Server) registerWebhookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/webhooks", s.webhooksCreate)
	mux.HandleFunc("GET /v1/webhooks", s.webhooksList)
	mux.HandleFunc("DELETE /v1/webhooks/{id}", s.webhooksDelete)
	mux.HandleFunc("GET /v1/webhooks/dead-letters", s.webhooksDeadLetters)
	mux.HandleFunc("POST /v1/webhooks/dead-letters/{delivery_id}/replay", s.webhooksReplayDeadLetter)
}

// callerHash converts the caller's API key (or "anon" for dev
// traffic) into an opaque hex fingerprint used as the ownership key
// for subscriptions. Keeps raw API keys out of every stored struct.
func callerHash(r *http.Request) string {
	k := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if k == "" {
		k = "anon"
	}
	sum := sha256.Sum256([]byte("cp-webhook-owner|" + k))
	return hex.EncodeToString(sum[:8])
}

func (s *Server) webhooksCreate(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		s.jsonError(w, "webhook subsystem disabled", http.StatusServiceUnavailable)
		return
	}
	var req webhookRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !s.enforceScope(w, r, "webhooks:write") {
		return
	}
	sub, err := s.webhooks.register(callerHash(r), req.URL, req.Secret, req.Events)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Never leak the secret back — the caller already knows it, we
	// don't need to echo it.
	out := *sub
	out.secret = ""
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(webhookRegisterResponse{Subscription: &out})
}

func (s *Server) webhooksList(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		s.jsonError(w, "webhook subsystem disabled", http.StatusServiceUnavailable)
		return
	}
	if !s.enforceScope(w, r, "webhooks:read") {
		return
	}
	subs := s.webhooks.list(callerHash(r))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(webhookListResponse{Subscriptions: subs, Count: len(subs)})
}

func (s *Server) webhooksDelete(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		s.jsonError(w, "webhook subsystem disabled", http.StatusServiceUnavailable)
		return
	}
	if !s.enforceScope(w, r, "webhooks:write") {
		return
	}
	id := r.PathValue("id")
	if err := s.webhooks.unregister(id, callerHash(r)); err != nil {
		s.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) webhooksDeadLetters(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		s.jsonError(w, "webhook subsystem disabled", http.StatusServiceUnavailable)
		return
	}
	if !s.enforceScope(w, r, "webhooks:read") {
		return
	}
	// Filter dead letters to only include those from subscriptions
	// this caller owns — dead-letter admin is per-caller, not global.
	owner := callerHash(r)
	all := s.webhooks.deadLetters()
	owned := make([]*deadLetter, 0, len(all))
	ownedSubs := s.webhooks.list(owner)
	ownedIDs := make(map[string]struct{}, len(ownedSubs))
	for _, sub := range ownedSubs {
		ownedIDs[sub.ID] = struct{}{}
	}
	for _, dl := range all {
		if _, ok := ownedIDs[dl.Event.SubID]; ok {
			owned = append(owned, dl)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(webhookDeadLetterResponse{DeadLetters: owned, Count: len(owned)})
}

type webhookReplayResponse struct {
	DeliveryID string `json:"delivery_id"`
	Status     string `json:"status"`
}

func (s *Server) webhooksReplayDeadLetter(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		s.jsonError(w, "webhook subsystem disabled", http.StatusServiceUnavailable)
		return
	}
	if !s.enforceScope(w, r, "webhooks:write") {
		return
	}
	id := r.PathValue("delivery_id")
	replayedID, err := s.webhooks.replay(id, callerHash(r))
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "unregistered") {
			status = http.StatusConflict
		}
		s.jsonError(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(webhookReplayResponse{DeliveryID: replayedID, Status: "enqueued"})
}

// emitWebhookEvent is the internal fan-out entry point. Domain
// handlers call this after a successful mutation to notify
// subscribers. Non-blocking: the caller does not wait for delivery.
//
// The payload is marshalled once so every subscriber gets the same
// bytes (which matters for signature verification — a re-marshal
// could rewrite field order and break HMAC comparisons on the
// receiver side).
func (s *Server) emitWebhookEvent(kind string, payload any) {
	if s.webhooks == nil {
		return
	}
	envelope := map[string]any{
		"event":     kind,
		"data":      payload,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		s.log.Warn("webhook marshal failed", "kind", kind, "err", err)
		return
	}
	added := s.webhooks.enqueue(kind, body)
	if added > 0 {
		s.log.Debug("webhook enqueued", "kind", kind, "subscribers", added)
	}
}
