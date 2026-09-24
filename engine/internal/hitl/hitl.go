// Package hitl implements the human-in-the-loop policy service that
// gates a decision commit before it reaches the on-chain proof
// registry.
//
// The service answers exactly one question: given an aggregation
// result from the decision layer, must a human review this decision
// before the gate releases? The answer is one of three actions:
//
//   * PASS       — no human needed; downstream may proceed.
//   * ESCALATE   — a durable ticket is opened; the gate blocks until
//                  a human resolves it.
//   * VETO       — the policy hard-rejects the decision without human
//                  review (used for equivocation and other invariants
//                  that a human cannot legitimately override).
//
// The policy is declarative:
//   1. Any critical facet that ABSTAINed ⇒ ESCALATE.
//   2. Any critical facet that REJECTed ⇒ VETO (already REJECTed at
//      the aggregate layer; we keep the mirror action here so a caller
//      that sees only the HITL response can still know the decision
//      was terminally rejected).
//   3. Non-critical facets combined confidence < ConfidenceThreshold
//      ⇒ ESCALATE.
//   4. Everything else ⇒ PASS.
//
// The ticket store is deliberately in-process (map + mutex) for the
// 30-day slice: production deployments swap in a Postgres-backed store
// via the TicketStore interface. That contract is documented in
// docs/DECISION_A2A_HITL.md.
package hitl

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
)

// Action is what Decide returns.
type Action string

const (
	// ActionPass — downstream may proceed without human review.
	ActionPass Action = "pass"
	// ActionEscalate — a durable ticket has been opened; the gate must
	// block until it is resolved.
	ActionEscalate Action = "escalate"
	// ActionVeto — the policy hard-rejects the attest.
	ActionVeto Action = "veto"
)

// Policy configures the Decide function.
type Policy struct {
	// ConfidenceThreshold is the minimum mean confidence across the
	// non-critical APPROVE facets required to skip HITL. Below this,
	// Decide returns ESCALATE with reason "low-confidence".
	ConfidenceThreshold float64
	// EscalateOnCriticalAbstain, if true (default), forces ESCALATE
	// when any critical facet ABSTAINed. Set false only in test
	// deployments that intentionally exercise the PASS path.
	EscalateOnCriticalAbstain bool
	// VetoOnCriticalReject, if true (default), forces VETO when any
	// critical facet REJECTed.
	VetoOnCriticalReject bool
}

// DefaultPolicy is what NewService uses when the caller passes a zero
// Policy. It is deliberately conservative.
var DefaultPolicy = Policy{
	ConfidenceThreshold:       0.6,
	EscalateOnCriticalAbstain: true,
	VetoOnCriticalReject:      true,
}

// Response is what Decide returns to the caller.
type Response struct {
	// Action is the policy outcome.
	Action Action `json:"action"`
	// Reason is a machine-readable explanation.
	Reason string `json:"reason"`
	// TicketID is populated when Action == ActionEscalate.
	TicketID string `json:"ticket_id,omitempty"`
}

// Ticket represents an open escalation.
type Ticket struct {
	ID            string    `json:"id"`
	DecisionID    string    `json:"decision_id"`
	Submitter     string    `json:"submitter"`
	SpecID        string    `json:"spec_id"`
	OpenedAt      time.Time `json:"opened_at"`
	ResolvedAt    time.Time `json:"resolved_at,omitempty"`
	Reason        string    `json:"reason"`
	Aggregate     string    `json:"aggregate"`     // "APPROVE"/"ABSTAIN"/"REJECT"
	VetoedBy      string    `json:"vetoed_by,omitempty"`
	// State is one of "pending", "approved", "rejected",
	// "stale_escalation". A resolved ticket has State != "pending".
	State string `json:"state"`
	// Resolver is the ID of the human who resolved the ticket
	// (opaque to this package; the caller passes it into Resolve).
	Resolver string `json:"resolver,omitempty"`
	// ResolutionNote is a free-text comment attached by the resolver.
	ResolutionNote string `json:"resolution_note,omitempty"`
}

// TicketStore persists open and resolved tickets. The interface is
// intentionally minimal so a Postgres-backed store can drop in.
type TicketStore interface {
	Create(t Ticket) error
	Get(id string) (Ticket, error)
	Resolve(id, resolver, state, note string) (Ticket, error)
	List(filter ListFilter) ([]Ticket, error)
}

// ListFilter narrows a List call. Zero value = list all.
type ListFilter struct {
	// State, if non-empty, restricts to tickets whose State matches.
	State string
	// Limit, if > 0, caps the returned slice length.
	Limit int
	// OpenedAfter, if non-zero, restricts to tickets opened at or after.
	OpenedAfter time.Time
}

// Errors returned by the store and Decide.
var (
	ErrTicketNotFound = errors.New("hitl: ticket not found")
	ErrAlreadyResolved = errors.New("hitl: ticket already resolved")
	ErrInvalidState   = errors.New("hitl: invalid resolution state")
)

// InMemoryTicketStore is a thread-safe in-process implementation.
type InMemoryTicketStore struct {
	mu      sync.RWMutex
	tickets map[string]Ticket
}

// NewInMemoryTicketStore returns an empty store.
func NewInMemoryTicketStore() *InMemoryTicketStore {
	return &InMemoryTicketStore{tickets: make(map[string]Ticket)}
}

// Create inserts a new ticket. Duplicate IDs are rejected.
func (s *InMemoryTicketStore) Create(t Ticket) error {
	if t.ID == "" {
		return errors.New("hitl: ticket with empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tickets[t.ID]; ok {
		return fmt.Errorf("hitl: ticket %s already exists", t.ID)
	}
	s.tickets[t.ID] = t
	return nil
}

// Get returns a copy of the ticket by ID or ErrTicketNotFound.
func (s *InMemoryTicketStore) Get(id string) (Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tickets[id]
	if !ok {
		return Ticket{}, ErrTicketNotFound
	}
	return t, nil
}

// Resolve marks a ticket resolved. `state` must be "approved",
// "rejected", or "stale_escalation".
func (s *InMemoryTicketStore) Resolve(id, resolver, state, note string) (Ticket, error) {
	if !validResolutionState(state) {
		return Ticket{}, ErrInvalidState
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	if !ok {
		return Ticket{}, ErrTicketNotFound
	}
	if t.State != "pending" {
		return Ticket{}, ErrAlreadyResolved
	}
	t.State = state
	t.Resolver = resolver
	t.ResolutionNote = note
	t.ResolvedAt = time.Now().UTC()
	s.tickets[id] = t
	return t, nil
}

// List returns tickets matching filter, sorted by OpenedAt descending
// then ID for tiebreaker determinism.
func (s *InMemoryTicketStore) List(filter ListFilter) ([]Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Ticket, 0, len(s.tickets))
	for _, t := range s.tickets {
		if filter.State != "" && t.State != filter.State {
			continue
		}
		if !filter.OpenedAfter.IsZero() && t.OpenedAt.Before(filter.OpenedAfter) {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].OpenedAt.Equal(out[j].OpenedAt) {
			return out[i].OpenedAt.After(out[j].OpenedAt)
		}
		return out[i].ID < out[j].ID
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func validResolutionState(s string) bool {
	switch s {
	case "approved", "rejected", "stale_escalation":
		return true
	}
	return false
}

// Service orchestrates a Decide call.
type Service struct {
	policy Policy
	store  TicketStore
	// clock allows tests to freeze time. Defaults to time.Now.
	clock func() time.Time
	// idgen allows tests to make IDs deterministic. Defaults to
	// crypto/rand-backed 16-byte hex.
	idgen func() string
}

// NewService returns a Service with the given policy and store. A zero
// policy is replaced with DefaultPolicy. A nil store is replaced with a
// fresh InMemoryTicketStore.
func NewService(policy Policy, store TicketStore) *Service {
	if policy == (Policy{}) {
		policy = DefaultPolicy
	}
	if store == nil {
		store = NewInMemoryTicketStore()
	}
	return &Service{policy: policy, store: store, clock: func() time.Time { return time.Now().UTC() }, idgen: randomID}
}

// WithClock overrides the clock (test hook). Returns the service for
// chaining.
func (s *Service) WithClock(fn func() time.Time) *Service { s.clock = fn; return s }

// WithIDGen overrides the ID generator (test hook).
func (s *Service) WithIDGen(fn func() string) *Service { s.idgen = fn; return s }

// Store returns the underlying store (for tests and API handlers).
func (s *Service) Store() TicketStore { return s.store }

// Policy returns the applied policy.
func (s *Service) Policy() Policy { return s.policy }

// Decide inspects the commit and returns the HITL Response. If the
// action is ActionEscalate, a Ticket is created in the store before
// return.
func (s *Service) Decide(commit attest.DecisionCommit) (Response, error) {
	if s == nil {
		return Response{}, errors.New("hitl: nil service")
	}
	// 1. Critical REJECT ⇒ VETO (mirrors the aggregate).
	if s.policy.VetoOnCriticalReject && commit.Aggregate == attest.VerdictReject && commit.VetoedBy != "" {
		return Response{
			Action: ActionVeto,
			Reason: fmt.Sprintf("critical-veto by %s: %s", commit.VetoedBy, commit.AbstainReason),
		}, nil
	}

	// 2. Any critical facet that ABSTAINed ⇒ ESCALATE.
	if s.policy.EscalateOnCriticalAbstain {
		for _, v := range commit.FacetVerdicts {
			if !isCriticalKind(v.Kind) {
				continue
			}
			if v.Verdict == attest.VerdictAbstain {
				ticket, err := s.openTicket(commit,
					fmt.Sprintf("critical facet %s ABSTAINed: %s", v.Kind, v.Reason))
				if err != nil {
					return Response{}, err
				}
				return Response{Action: ActionEscalate, Reason: ticket.Reason, TicketID: ticket.ID}, nil
			}
		}
	}

	// 3. Low confidence across non-critical facets ⇒ ESCALATE.
	meanConf, n := nonCriticalApproveConfidence(commit.FacetVerdicts)
	if n > 0 && meanConf < s.policy.ConfidenceThreshold {
		ticket, err := s.openTicket(commit,
			fmt.Sprintf("low-confidence: mean approve confidence %.3f < %.3f threshold",
				meanConf, s.policy.ConfidenceThreshold))
		if err != nil {
			return Response{}, err
		}
		return Response{Action: ActionEscalate, Reason: ticket.Reason, TicketID: ticket.ID}, nil
	}

	// 4. Nothing tripped ⇒ PASS.
	return Response{Action: ActionPass, Reason: "policy: no escalation trigger"}, nil
}

func (s *Service) openTicket(commit attest.DecisionCommit, reason string) (Ticket, error) {
	t := Ticket{
		ID:         s.idgen(),
		DecisionID: commit.DecisionID,
		Submitter:  commit.Decision.Submitter,
		SpecID:     commit.Decision.SpecID,
		OpenedAt:   s.clock(),
		Reason:     reason,
		Aggregate:  commit.Aggregate.String(),
		VetoedBy:   string(commit.VetoedBy),
		State:      "pending",
	}
	if err := s.store.Create(t); err != nil {
		return Ticket{}, err
	}
	return t, nil
}

// isCriticalKind mirrors attest.FacetKind.isCritical without leaking
// unexported code across the package boundary.
func isCriticalKind(k attest.FacetKind) bool {
	return k == attest.FacetSafety || k == attest.FacetEquivocation
}

// nonCriticalApproveConfidence returns the mean confidence of the
// non-critical facets that voted APPROVE and the count of such votes.
func nonCriticalApproveConfidence(verdicts []attest.FacetVerdict) (float64, int) {
	var sum float64
	var n int
	for _, v := range verdicts {
		if isCriticalKind(v.Kind) {
			continue
		}
		if v.Verdict != attest.VerdictApprove {
			continue
		}
		sum += v.Confidence
		n++
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

// randomID is the default 16-byte hex ID generator. It is safe under
// crypto/rand starvation because the caller-facing action is not
// authorization-critical: two colliding IDs would surface as a Create
// error rather than a security issue.
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback that still produces a well-formed hex string. The
		// upper 8 bytes are the current nanosecond, the lower are zero.
		ns := time.Now().UnixNano()
		for i := 0; i < 8; i++ {
			b[i] = byte(ns >> (i * 8))
		}
	}
	return hex.EncodeToString(b[:])
}
