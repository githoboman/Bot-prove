package hitl

import (
	"errors"
	"testing"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
)

func newCommit(agg attest.Verdict, vetoedBy attest.FacetKind, facets []attest.FacetVerdict) attest.DecisionCommit {
	dec := attest.Decision{Submitter: "sub", SpecID: "spec", Nonce: 1}
	return attest.DecisionCommit{
		Decision:      dec,
		DecisionID:    dec.ID(),
		FacetVerdicts: facets,
		Aggregate:     agg,
		VetoedBy:      vetoedBy,
	}
}

func fixedIDGen(id string) func() string { return func() string { return id } }

func TestDecideVetoOnCriticalReject(t *testing.T) {
	svc := NewService(DefaultPolicy, nil)
	commit := newCommit(attest.VerdictReject, attest.FacetSafety, []attest.FacetVerdict{
		{Kind: attest.FacetSafety, Verdict: attest.VerdictReject, Confidence: 0.9, Reason: "unsafe"},
	})
	resp, err := svc.Decide(commit)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if resp.Action != ActionVeto {
		t.Fatalf("expected veto, got %v", resp.Action)
	}
	if resp.TicketID != "" {
		t.Fatalf("veto path must not open a ticket, got id %q", resp.TicketID)
	}
}

func TestDecideEscalatesOnCriticalAbstain(t *testing.T) {
	svc := NewService(DefaultPolicy, nil).WithIDGen(fixedIDGen("t-1"))
	commit := newCommit(attest.VerdictAbstain, "", []attest.FacetVerdict{
		{Kind: attest.FacetSafety, Verdict: attest.VerdictAbstain, Reason: "insufficient evidence"},
		{Kind: attest.FacetCorrectness, Verdict: attest.VerdictApprove, Confidence: 0.9, Reason: "ok"},
	})
	resp, err := svc.Decide(commit)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if resp.Action != ActionEscalate {
		t.Fatalf("expected escalate, got %v (%s)", resp.Action, resp.Reason)
	}
	if resp.TicketID != "t-1" {
		t.Fatalf("expected TicketID t-1, got %q", resp.TicketID)
	}
	ticket, err := svc.Store().Get("t-1")
	if err != nil {
		t.Fatalf("Store().Get: %v", err)
	}
	if ticket.State != "pending" {
		t.Fatalf("expected pending, got %q", ticket.State)
	}
}

func TestDecideEscalatesOnLowConfidence(t *testing.T) {
	svc := NewService(Policy{
		ConfidenceThreshold:       0.9,
		EscalateOnCriticalAbstain: true,
		VetoOnCriticalReject:      true,
	}, nil).WithIDGen(fixedIDGen("t-lowconf"))
	commit := newCommit(attest.VerdictApprove, "", []attest.FacetVerdict{
		{Kind: attest.FacetSafety, Verdict: attest.VerdictApprove, Confidence: 0.95, Reason: "safe"},
		{Kind: attest.FacetEquivocation, Verdict: attest.VerdictApprove, Confidence: 0.95, Reason: "no conflict"},
		{Kind: attest.FacetCorrectness, Verdict: attest.VerdictApprove, Confidence: 0.5, Reason: "meh"},
		{Kind: attest.FacetSpecCompliance, Verdict: attest.VerdictApprove, Confidence: 0.5, Reason: "meh"},
	})
	resp, err := svc.Decide(commit)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if resp.Action != ActionEscalate {
		t.Fatalf("expected escalate on low confidence, got %v (%s)", resp.Action, resp.Reason)
	}
	if resp.TicketID != "t-lowconf" {
		t.Fatalf("expected t-lowconf, got %q", resp.TicketID)
	}
}

func TestDecidePassPath(t *testing.T) {
	svc := NewService(DefaultPolicy, nil)
	commit := newCommit(attest.VerdictApprove, "", []attest.FacetVerdict{
		{Kind: attest.FacetSafety, Verdict: attest.VerdictApprove, Confidence: 0.9, Reason: "safe"},
		{Kind: attest.FacetEquivocation, Verdict: attest.VerdictApprove, Confidence: 0.9, Reason: "no conflict"},
		{Kind: attest.FacetCorrectness, Verdict: attest.VerdictApprove, Confidence: 0.8, Reason: "ok"},
		{Kind: attest.FacetSpecCompliance, Verdict: attest.VerdictApprove, Confidence: 0.8, Reason: "ok"},
	})
	resp, err := svc.Decide(commit)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if resp.Action != ActionPass {
		t.Fatalf("expected pass, got %v (%s)", resp.Action, resp.Reason)
	}
}

func TestResolveTicketLifecycle(t *testing.T) {
	store := NewInMemoryTicketStore()
	svc := NewService(DefaultPolicy, store).WithIDGen(fixedIDGen("t-resolve")).WithClock(func() time.Time {
		return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	})
	commit := newCommit(attest.VerdictAbstain, "", []attest.FacetVerdict{
		{Kind: attest.FacetSafety, Verdict: attest.VerdictAbstain, Reason: "unclear"},
	})
	if _, err := svc.Decide(commit); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	// Invalid state → error.
	if _, err := store.Resolve("t-resolve", "reviewer-1", "bogus", ""); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
	// Not-found → error.
	if _, err := store.Resolve("nonexistent", "reviewer-1", "approved", ""); !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("expected ErrTicketNotFound, got %v", err)
	}
	// Happy path.
	tk, err := store.Resolve("t-resolve", "reviewer-1", "approved", "checked evidence")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if tk.State != "approved" || tk.Resolver != "reviewer-1" || tk.ResolutionNote != "checked evidence" {
		t.Fatalf("unexpected resolved ticket: %+v", tk)
	}
	// Double-resolve → error.
	if _, err := store.Resolve("t-resolve", "reviewer-2", "rejected", "later"); !errors.Is(err, ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved, got %v", err)
	}
}

func TestListFilterAndOrdering(t *testing.T) {
	store := NewInMemoryTicketStore()
	base := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	must := func(err error) {
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	must(store.Create(Ticket{ID: "a", OpenedAt: base, State: "pending"}))
	must(store.Create(Ticket{ID: "b", OpenedAt: base.Add(2 * time.Second), State: "pending"}))
	must(store.Create(Ticket{ID: "c", OpenedAt: base.Add(1 * time.Second), State: "approved"}))
	// pending only, sorted by OpenedAt desc → b, a
	got, err := store.List(ListFilter{State: "pending"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("unexpected pending order: %+v", got)
	}
	// limit=1 → only newest pending
	got, _ = store.List(ListFilter{State: "pending", Limit: 1})
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("expected [b], got %+v", got)
	}
	// OpenedAfter drops the earliest one
	got, _ = store.List(ListFilter{OpenedAfter: base.Add(500 * time.Millisecond)})
	// c (base+1s) and b (base+2s), sorted desc
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "c" {
		t.Fatalf("unexpected OpenedAfter set: %+v", got)
	}
}
