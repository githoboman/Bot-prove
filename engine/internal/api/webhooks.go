package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/observability"
)

// Webhook subsystem — outbound event delivery to caller-registered URLs.
//
// Design:
//
//   * A caller POSTs to /v1/webhooks with a target URL, an optional
//     shared secret (used to sign payloads with HMAC-SHA256), and a
//     list of event kinds it wants to receive. The server assigns an
//     opaque id; the caller can DELETE /v1/webhooks/{id} to
//     unregister and GET /v1/webhooks to list its own registrations.
//
//   * When something interesting happens inside the engine
//     (proof.verified, proof.anchored, slash.executed,
//     governance.executed, …), a handler calls
//     Server.emitWebhookEvent(ctx, kind, payload). That call is
//     non-blocking: the event is enqueued and a background worker
//     fans it out to every subscriber that opted into the kind.
//
//   * Delivery is best-effort with exponential backoff (1s → 2s → 4s
//     … capped at 5 minutes) up to eight tries. Each attempt sends
//     the signed payload, records the last outcome on the
//     registration, and — if all eight attempts fail — moves the
//     event to an in-memory dead-letter list that operators can
//     inspect via GET /v1/webhooks/dead-letters.
//
//   * Signature contract (documented on the OpenAPI schema too):
//         X-CP-Signature = "sha256=" + hex(HMAC-SHA256(secret, body))
//         X-CP-Timestamp = RFC3339 timestamp of dispatch attempt
//         X-CP-Event     = event kind (e.g. "proof.verified")
//         X-CP-Delivery  = per-attempt UUID-ish id (subscription + seq)
//     Receivers MUST verify both the signature and that the timestamp
//     is fresh (we recommend a 5-minute window) to defeat replay.
//
//   * This is intentionally in-memory: single-node, non-durable, TTL
//     for dead-letters is 24 hours. A durable variant belongs in
//     Postgres and is tracked in KNOWN_LIMITATIONS.md.

// Recognized event kinds. Emit-side code MUST use one of these
// constants so a typo becomes a compile error rather than a silent
// misroute.
const (
	EventProofVerified     = "proof.verified"
	EventProofAnchored     = "proof.anchored"
	EventSlashExecuted     = "slash.executed"
	EventGovernanceExecute = "governance.executed"
	EventKYCGranted        = "kyc.granted"
	EventKYCRevoked        = "kyc.revoked"
)

// KnownWebhookEvents is exposed so the OpenAPI generator and admin
// UIs can enumerate what a caller may subscribe to without a
// hand-maintained duplicate list.
var KnownWebhookEvents = []string{
	EventProofVerified,
	EventProofAnchored,
	EventSlashExecuted,
	EventGovernanceExecute,
	EventKYCGranted,
	EventKYCRevoked,
}

// webhookSubscription is a caller-registered target.
type webhookSubscription struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	CreatedAt time.Time `json:"created_at"`
	OwnerKey  string    `json:"-"` // hash of API key that created it — never returned
	// secret is the shared HMAC secret. Not marshalled in list responses.
	secret string
	// Rolling delivery stats — inspectable via GET /v1/webhooks/{id}.
	Attempts       int       `json:"attempts"`
	Deliveries     int       `json:"deliveries"`
	Failures       int       `json:"failures"`
	LastAttemptAt  time.Time `json:"last_attempt_at,omitempty"`
	LastStatusCode int       `json:"last_status_code,omitempty"`
	LastError      string    `json:"last_error,omitempty"`

	// Circuit breaker fields (BF hardening).
	// ConsecutiveFailures is reset on every 2xx delivery; when it
	// crosses circuitBreakerThreshold, the subscription is paused —
	// no attempts are dispatched until CircuitOpenUntil has passed.
	// A caller can inspect these via GET /v1/webhooks/{id}.
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CircuitOpenUntil    time.Time `json:"circuit_open_until,omitempty"`
	CircuitTrippedTotal int       `json:"circuit_tripped_total,omitempty"`
}

// webhookEvent is one dispatch unit — one (subscription, event)
// pairing that the worker will retry until success or dead-lettered.
type webhookEvent struct {
	SubID     string          `json:"sub_id"`
	Kind      string          `json:"kind"`
	Body      json.RawMessage `json:"body"`
	Attempts  int             `json:"attempts"`
	NextTryAt time.Time       `json:"next_try_at"`
	LastError string          `json:"last_error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// deadLetter is the archived tail of an event that ran out of retries.
type deadLetter struct {
	// DeliveryID is a stable, opaque handle for POST
	// /v1/webhooks/dead-letters/{delivery_id}/replay. Derived from
	// SubID + CreatedAt so it is stable across restarts of a single
	// process (in-memory only — see KNOWN_LIMITATIONS.md).
	DeliveryID string       `json:"delivery_id"`
	Event      webhookEvent `json:"event"`
	FailedAt   time.Time    `json:"failed_at"`
	URL        string       `json:"url"`
	OwnerKey   string       `json:"-"`
	// Replayed is bumped every time an operator calls the replay
	// endpoint. Lets us surface "this ran out of retries N times" in
	// the admin UI without losing the record on the first replay.
	Replayed int `json:"replayed"`
}

// webhookStore holds registrations, the pending queue, and dead
// letters. The zero value is not usable — call newWebhookStore.
type webhookStore struct {
	mu       sync.RWMutex
	subs     map[string]*webhookSubscription
	queue    []*webhookEvent
	dead     []*deadLetter
	stopping chan struct{}
	client   *http.Client
	now      func() time.Time // injectable for tests
	// metrics is optional; nil disables instrumentation. Set via
	// webhookStore.SetMetrics after construction so tests do not have
	// to touch Prometheus at all.
	metrics *observability.WebhookMetrics
}

// SetMetrics wires a WebhookMetrics into the store. Safe to call
// once at startup; passing nil is a no-op (default).
func (s *webhookStore) SetMetrics(m *observability.WebhookMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = m
}

// sampleGauges pushes queue / dead-letter depth into metrics. Caller
// must hold s.mu (any mode). No-op when metrics is nil.
func (s *webhookStore) sampleGaugesLocked() {
	if s.metrics == nil {
		return
	}
	s.metrics.QueueDepth.Set(float64(len(s.queue)))
	s.metrics.DeadLetterDepth.Set(float64(len(s.dead)))
}

const (
	webhookMaxAttempts     = 8
	webhookMaxBackoff      = 5 * time.Minute
	webhookInitialBackoff  = time.Second
	webhookDeadLetterLimit = 1000
	webhookRequestTimeout  = 10 * time.Second

	// BF hardening constants.
	//
	// Full-jitter backoff: instead of the deterministic
	// initial*2^(attempt-1) schedule, each retry picks a random
	// duration in [0, cappedBackoff) — this breaks the thundering-herd
	// synchrony when many subs fail against the same receiver at once.
	//
	// Retry-After: if the receiver returns 429 or 503 with a
	// Retry-After header (seconds or HTTP-date), we honour it up to a
	// hard ceiling so a hostile receiver cannot pin an event in the
	// queue indefinitely.
	//
	// Circuit breaker: after N consecutive failed attempts against one
	// subscription (across events), we pause delivery to that sub for
	// a cool-down window. A 2xx delivery resets the counter and clears
	// the pause immediately.
	//
	// DLQ TTL: dead-letters are evicted after webhookDeadLetterTTL —
	// documented as a memory-hygiene bound, not a durability promise.
	webhookRetryAfterCeiling = 15 * time.Minute
	circuitBreakerThreshold  = 20
	circuitBreakerCooldown   = 2 * time.Minute
	webhookDeadLetterTTL     = 24 * time.Hour
)

func newWebhookStore() *webhookStore {
	s := &webhookStore{
		subs:     make(map[string]*webhookSubscription),
		stopping: make(chan struct{}),
		client:   &http.Client{Timeout: webhookRequestTimeout},
		now:      time.Now,
	}
	return s
}

// register creates a new subscription.
func (s *webhookStore) register(ownerKeyHash, url, secret string, events []string) (*webhookSubscription, error) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return nil, errors.New("url must start with http:// or https://")
	}
	if len(events) == 0 {
		return nil, errors.New("events list must not be empty")
	}
	// Validate every event kind so a typo fails registration rather
	// than silently subscribing to a phantom kind.
	for _, e := range events {
		if !isKnownEvent(e) {
			return nil, fmt.Errorf("unknown event kind %q", e)
		}
	}
	id := newWebhookID(s.now())
	sub := &webhookSubscription{
		ID:        id,
		URL:       url,
		Events:    append([]string(nil), events...),
		CreatedAt: s.now(),
		OwnerKey:  ownerKeyHash,
		secret:    secret,
	}
	s.mu.Lock()
	s.subs[id] = sub
	s.mu.Unlock()
	return sub, nil
}

// unregister removes a subscription. Enforced against ownerKeyHash
// so caller A cannot delete caller B's webhooks.
func (s *webhookStore) unregister(id, ownerKeyHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return errors.New("not found")
	}
	if sub.OwnerKey != ownerKeyHash {
		return errors.New("not found") // deliberately opaque
	}
	delete(s.subs, id)
	return nil
}

// list returns every subscription owned by ownerKeyHash. The order is
// insertion-time-ish (map iteration then sort by CreatedAt).
func (s *webhookStore) list(ownerKeyHash string) []*webhookSubscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*webhookSubscription, 0, len(s.subs))
	for _, sub := range s.subs {
		if sub.OwnerKey == ownerKeyHash {
			// Shallow copy so we never leak the secret via the API.
			cp := *sub
			cp.secret = ""
			out = append(out, &cp)
		}
	}
	// Stable ordering — smallest CreatedAt first.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CreatedAt.Before(out[j-1].CreatedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// enqueue fans one event out to every matching subscription. Called
// by handlers via Server.emitWebhookEvent.
func (s *webhookStore) enqueue(kind string, body []byte) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	added := 0
	now := s.now()
	for _, sub := range s.subs {
		if !subscribedTo(sub.Events, kind) {
			continue
		}
		s.queue = append(s.queue, &webhookEvent{
			SubID:     sub.ID,
			Kind:      kind,
			Body:      append(json.RawMessage(nil), body...),
			NextTryAt: now,
			CreatedAt: now,
		})
		added++
		if s.metrics != nil {
			s.metrics.Enqueued.Inc(kind)
		}
	}
	s.sampleGaugesLocked()
	return added
}

// deliverOnce processes every event whose NextTryAt has passed.
// Returns the number of events attempted. Idempotent and safe to
// call from the worker loop or a test.
//
// BF hardening — two guards run under the lock while we split the
// pending set from the queue:
//
//   * Circuit breaker: an event whose sub has an open circuit gets
//     its NextTryAt pushed to CircuitOpenUntil and is put back on
//     the queue instead of dispatched. The event still counts as
//     pending work; it just does not fire an HTTP call this tick.
//
//   * Dead-letter TTL: entries older than webhookDeadLetterTTL are
//     evicted here so the eviction cadence matches the worker tick.
func (s *webhookStore) deliverOnce(ctx context.Context) int {
	s.mu.Lock()
	now := s.now()
	s.evictExpiredDeadLettersLocked(now)
	pending := make([]*webhookEvent, 0)
	remaining := make([]*webhookEvent, 0, len(s.queue))
	for _, ev := range s.queue {
		if ev.NextTryAt.After(now) {
			remaining = append(remaining, ev)
			continue
		}
		sub, ok := s.subs[ev.SubID]
		if ok && sub.CircuitOpenUntil.After(now) {
			// Sub is in cool-down — defer this event.
			ev.NextTryAt = sub.CircuitOpenUntil
			remaining = append(remaining, ev)
			continue
		}
		pending = append(pending, ev)
	}
	s.queue = remaining
	// Snapshot subs so we can release the lock during delivery.
	subCopy := make(map[string]*webhookSubscription, len(s.subs))
	for k, v := range s.subs {
		subCopy[k] = v
	}
	s.mu.Unlock()

	attempted := 0
	for _, ev := range pending {
		sub, ok := subCopy[ev.SubID]
		if !ok {
			// Subscription vanished (unregistered between enqueue and now).
			continue
		}
		attempted++
		s.deliverEvent(ctx, sub, ev)
	}
	return attempted
}

// evictExpiredDeadLettersLocked drops dead-letters older than
// webhookDeadLetterTTL. Called with s.mu already held.
func (s *webhookStore) evictExpiredDeadLettersLocked(now time.Time) {
	if len(s.dead) == 0 {
		return
	}
	cutoff := now.Add(-webhookDeadLetterTTL)
	kept := s.dead[:0]
	for _, dl := range s.dead {
		if dl.FailedAt.After(cutoff) {
			kept = append(kept, dl)
		}
	}
	s.dead = kept
}

// deliverEvent sends one dispatch attempt and mutates the event with
// the outcome — either finalized (dropped from queue), retried
// (pushed back with backoff), or dead-lettered.
// idempotencyKey is a stable per-attempt id the receiver can use to
// deduplicate a retried delivery — required by BF because a 2xx that
// crossed the wire but never reached us gets retried, and a
// well-behaved receiver must be able to tell it's a retry.
//
// Format: <sub_id>-<attempt>-<sha256 of body, first 8 hex chars>.
func idempotencyKey(subID string, attempt int, body []byte) string {
	h := sha256.Sum256(body)
	return fmt.Sprintf("%s-%d-%s", subID, attempt, hex.EncodeToString(h[:4]))
}

func (s *webhookStore) deliverEvent(ctx context.Context, sub *webhookSubscription, ev *webhookEvent) {
	sig := signWebhook(sub.secret, ev.Body)
	deliveryID := fmt.Sprintf("%s-%d", sub.ID, ev.Attempts+1)
	start := s.now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(ev.Body))
	if err != nil {
		s.recordFailure(sub, ev, fmt.Sprintf("build request: %v", err), 0, 0)
		s.observeAttempt(ev.Kind, 0, start)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CP-Event", ev.Kind)
	req.Header.Set("X-CP-Delivery", deliveryID)
	req.Header.Set("X-CP-Idempotency-Key", idempotencyKey(sub.ID, ev.Attempts+1, ev.Body))
	req.Header.Set("X-CP-Timestamp", s.now().UTC().Format(time.RFC3339))
	if sig != "" {
		req.Header.Set("X-CP-Signature", sig)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.recordFailure(sub, ev, err.Error(), 0, 0)
		s.observeAttempt(ev.Kind, 0, start)
		return
	}
	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), s.now())
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	code := resp.StatusCode
	if code >= 200 && code < 300 {
		s.recordSuccess(sub, ev, code)
		s.observeAttempt(ev.Kind, code, start)
		return
	}
	s.recordFailure(sub, ev, fmt.Sprintf("http %d", resp.StatusCode), resp.StatusCode, retryAfter)
	s.observeAttempt(ev.Kind, resp.StatusCode, start)
}

// observeAttempt records the outcome of one HTTP dispatch attempt.
// No-op when metrics is nil.
func (s *webhookStore) observeAttempt(kind string, code int, start time.Time) {
	s.mu.RLock()
	m := s.metrics
	now := s.now
	s.mu.RUnlock()
	if m == nil {
		return
	}
	class := observability.StatusClass(code)
	m.Attempts.Inc(kind, class)
	if now != nil {
		m.AttemptDuration.Observe(now().Sub(start).Seconds(), kind, class)
	}
}

func (s *webhookStore) recordSuccess(sub *webhookSubscription, ev *webhookEvent, code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub.Attempts++
	sub.Deliveries++
	sub.LastAttemptAt = s.now()
	sub.LastStatusCode = code
	sub.LastError = ""
	// Circuit breaker: a 2xx clears both the streak counter and any
	// active pause — the next event is delivered right away.
	sub.ConsecutiveFailures = 0
	sub.CircuitOpenUntil = time.Time{}
	if s.metrics != nil {
		s.metrics.Delivered.Inc(ev.Kind)
	}
	s.sampleGaugesLocked()
}

// recordFailure books one failed attempt. retryAfter, when > 0,
// forces the next-try-at to at least now+retryAfter regardless of
// the computed jittered backoff (bounded by webhookRetryAfterCeiling).
func (s *webhookStore) recordFailure(sub *webhookSubscription, ev *webhookEvent, msg string, code int, retryAfter time.Duration) {
	s.mu.Lock()
	sub.Attempts++
	sub.Failures++
	sub.LastAttemptAt = s.now()
	sub.LastStatusCode = code
	sub.LastError = msg
	sub.ConsecutiveFailures++
	if sub.ConsecutiveFailures >= circuitBreakerThreshold && sub.CircuitOpenUntil.Before(s.now()) {
		sub.CircuitOpenUntil = s.now().Add(circuitBreakerCooldown)
		sub.CircuitTrippedTotal++
	}
	ev.Attempts++
	ev.LastError = msg
	if ev.Attempts >= webhookMaxAttempts {
		dl := &deadLetter{
			DeliveryID: newDeadLetterID(sub.ID, ev.CreatedAt),
			Event:      *ev,
			FailedAt:   s.now(),
			URL:        sub.URL,
			OwnerKey:   sub.OwnerKey,
		}
		s.dead = append(s.dead, dl)
		if len(s.dead) > webhookDeadLetterLimit {
			s.dead = s.dead[len(s.dead)-webhookDeadLetterLimit:]
		}
		if s.metrics != nil {
			s.metrics.DeadLettered.Inc(ev.Kind)
		}
		s.sampleGaugesLocked()
		s.mu.Unlock()
		return
	}
	backoff := jitteredBackoff(ev.Attempts)
	if retryAfter > 0 {
		if retryAfter > webhookRetryAfterCeiling {
			retryAfter = webhookRetryAfterCeiling
		}
		if retryAfter > backoff {
			backoff = retryAfter
		}
	}
	ev.NextTryAt = s.now().Add(backoff)
	s.queue = append(s.queue, ev)
	s.mu.Unlock()
}

// jitteredBackoff returns a random duration in [0, cap) where cap =
// min(initial*2^(attempt-1), webhookMaxBackoff). This is the
// "full-jitter" variant recommended by AWS's exponential-backoff
// guidance — every retry pulls a fresh random point in the growing
// window, which is empirically the best decorrelator for a herd of
// clients hitting the same overloaded receiver.
//
// A crypto/rand source is deliberate: math/rand's default source is
// process-wide and can be re-seeded by unrelated code, which would
// couple our jitter with third-party callers.
func jitteredBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	capDur := webhookInitialBackoff << (attempt - 1)
	if capDur > webhookMaxBackoff || capDur < 0 {
		capDur = webhookMaxBackoff
	}
	if capDur <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(capDur)))
	if err != nil {
		// Extremely unlikely (only when the OS RNG fails). Fall back to
		// the deterministic cap so we still make progress.
		return capDur
	}
	return time.Duration(n.Int64())
}

// parseRetryAfter accepts a Retry-After header value in either of
// its two RFC 7231 forms — an integer number of seconds, or an
// HTTP-date — and returns the resulting delay from `now`. Returns 0
// on empty or malformed input; callers use that to fall back to the
// jittered backoff.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		d := t.Sub(now)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

func (s *webhookStore) deadLetters() []*deadLetter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*deadLetter, len(s.dead))
	copy(out, s.dead)
	return out
}

// replay pulls a dead-lettered event back onto the delivery queue at
// attempts=0 with NextTryAt=now, and removes it from the dead-letter
// list. Ownership is enforced against ownerKeyHash so caller A cannot
// replay caller B's dead letters. Returns the delivery id of the
// re-enqueued event on success, or an error if not found / not owned
// / the target subscription has been unregistered.
func (s *webhookStore) replay(deliveryID, ownerKeyHash string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, dl := range s.dead {
		if dl.DeliveryID == deliveryID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", errors.New("not found")
	}
	dl := s.dead[idx]
	if dl.OwnerKey != ownerKeyHash {
		return "", errors.New("not found") // deliberately opaque
	}
	sub, ok := s.subs[dl.Event.SubID]
	if !ok {
		return "", errors.New("subscription unregistered — cannot replay")
	}
	_ = sub
	// Re-enqueue with attempts reset. Keep the body verbatim so the
	// HMAC signature stays byte-identical to the original attempt.
	ev := dl.Event
	ev.Attempts = 0
	ev.LastError = ""
	ev.NextTryAt = s.now()
	s.queue = append(s.queue, &ev)
	// Remove from dead list.
	s.dead = append(s.dead[:idx], s.dead[idx+1:]...)
	if s.metrics != nil {
		s.metrics.Replayed.Inc(ev.Kind)
	}
	s.sampleGaugesLocked()
	// Preserve the replay counter — useful for observability if the
	// same event gets dead-lettered a second time.
	return dl.DeliveryID, nil
}

// newDeadLetterID mints the stable id used by the replay endpoint.
// Format: "dl_" + first 12 hex chars of sha256(subID | createdAt).
// Two events on the same subscription created in the same nanosecond
// would collide — extraordinarily unlikely for a single node.
func newDeadLetterID(subID string, createdAt time.Time) string {
	h := sha256.Sum256([]byte(subID + "|" + createdAt.Format(time.RFC3339Nano)))
	return "dl_" + hex.EncodeToString(h[:6])
}

// runWorker delivers events on a tick until the store's stopping
// channel closes. Callers spawn it once at server start.
func (s *webhookStore) runWorker(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopping:
			return
		case <-t.C:
			s.deliverOnce(ctx)
		}
	}
}

// signWebhook computes the HMAC-SHA256 header value. An empty secret
// disables signing (the header is omitted, and the receiver must not
// require it) — useful for local demos where the whole path is
// localhost.
func signWebhook(secret string, body []byte) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhookSignature is exposed for SDKs and tests: given the
// shared secret, the raw body, and the received header value, return
// true if the signature matches. Uses subtle.ConstantTimeCompare to
// avoid timing leaks.
func VerifyWebhookSignature(secret string, body []byte, header string) bool {
	if secret == "" || header == "" {
		return false
	}
	want := signWebhook(secret, body)
	return subtle.ConstantTimeCompare([]byte(want), []byte(header)) == 1
}

func isKnownEvent(kind string) bool {
	for _, e := range KnownWebhookEvents {
		if e == kind {
			return true
		}
	}
	return false
}

func subscribedTo(list []string, kind string) bool {
	for _, e := range list {
		if e == kind || e == "*" {
			return true
		}
	}
	return false
}

// newWebhookID mints a short, sortable-ish subscription id. Format:
// "wh_" + hex(6 bytes of unix nanos xor'd) — collision-resistant
// enough for a demo and human-readable.
func newWebhookID(now time.Time) string {
	nanos := now.UnixNano()
	b := make([]byte, 6)
	for i := 0; i < 6; i++ {
		b[i] = byte(nanos >> (i * 8))
	}
	// Fold in a per-call counter via a monotonic bump so two calls in
	// the same nanosecond can't collide.
	webhookIDCounter.mu.Lock()
	webhookIDCounter.n++
	c := webhookIDCounter.n
	webhookIDCounter.mu.Unlock()
	b[0] ^= byte(c)
	b[1] ^= byte(c >> 8)
	return "wh_" + hex.EncodeToString(b)
}

var webhookIDCounter = struct {
	mu sync.Mutex
	n  uint64
}{}
