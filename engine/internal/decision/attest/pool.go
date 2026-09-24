package attest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// A2A (agent-to-agent) provider pool + router.
//
// The baseline Judge (see provider.go) evaluates one Provider per run.
// Real agent-to-agent workflows involve several providers with different
// trust levels and capabilities: for instance a `system` LLM safety
// evaluator, an `observational` retrieval-augmented fact-checker, and a
// `delegated` compliance oracle.  The Router selects a subset of providers
// for each decision based on the requested facets and the providers'
// declared capabilities, calls them in parallel, and hands the concatenated
// per-facet verdicts to the Byzantine-robust aggregation path.
//
// This file intentionally does not touch any I/O or LLM: every Provider
// registered here MUST already satisfy the deterministic contract
// documented on decision.Provider.

// TrustLevel is a coarse policy tier for a provider. Higher tiers are
// consulted first and their verdicts carry more weight in
// diagnostic/audit output. It is NOT used to override Byzantine-robust
// aggregation: the on-chain rule is verdict-count-only.
type TrustLevel int

const (
	// TrustObservational — external / third-party, cannot vote on
	// critical facets. Included for auditability but its verdicts on
	// FacetSafety/FacetEquivocation are downgraded to ABSTAIN.
	TrustObservational TrustLevel = iota
	// TrustDelegated — vetted third-party evaluator, can vote on any
	// facet.
	TrustDelegated
	// TrustSystem — first-party evaluator maintained by the operator,
	// authoritative on critical facets.
	TrustSystem
)

// String renders a TrustLevel for logs and receipts.
func (t TrustLevel) String() string {
	switch t {
	case TrustSystem:
		return "system"
	case TrustDelegated:
		return "delegated"
	case TrustObservational:
		return "observational"
	default:
		return "unknown"
	}
}

// Capability identifies what a provider is competent to evaluate. A
// provider MAY be competent for a subset of AllFacetKinds; the Router
// only routes decisions requesting a matching facet to that provider.
type Capability string

const (
	CapSafety          Capability = "safety"
	CapCorrectness     Capability = "correctness"
	CapSpecCompliance  Capability = "spec_compliance"
	CapEquivocation    Capability = "equivocation"
)

// facetToCap maps a FacetKind to the Capability required to vote on it.
// Kept intentionally 1:1 for now; a provider covering multiple facets
// declares multiple capabilities.
func facetToCap(k FacetKind) Capability {
	switch k {
	case FacetSafety:
		return CapSafety
	case FacetCorrectness:
		return CapCorrectness
	case FacetSpecCompliance:
		return CapSpecCompliance
	case FacetEquivocation:
		return CapEquivocation
	default:
		return Capability("")
	}
}

// PooledProvider bundles a Provider with its declared trust level and
// capabilities. It is registered once, and the Router consults it on
// every subsequent Route call.
type PooledProvider struct {
	// Provider is the deterministic evaluator. MUST satisfy the
	// determinism contract documented on decision.Provider.
	Provider Provider
	// Trust is the coarse trust tier (see TrustLevel).
	Trust TrustLevel
	// Capabilities lists which facets this provider is competent to
	// evaluate. Duplicates are ignored.
	Capabilities []Capability
}

// ProviderPool is a thread-safe registry of PooledProviders. It is the
// backend for the Router.
type ProviderPool struct {
	mu        sync.RWMutex
	providers []PooledProvider
}

// NewProviderPool returns an empty pool.
func NewProviderPool() *ProviderPool { return &ProviderPool{} }

// Register adds a provider to the pool. Providers with identical Name()
// are rejected — the pool refuses to store two evaluators with the same
// identity, since receipt provenance would be ambiguous.
func (pl *ProviderPool) Register(p PooledProvider) error {
	if p.Provider == nil {
		return errors.New("decision: nil provider")
	}
	if p.Provider.Name() == "" {
		return errors.New("decision: provider without name")
	}
	if len(p.Capabilities) == 0 {
		return fmt.Errorf("decision: provider %q has no capabilities", p.Provider.Name())
	}
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for _, existing := range pl.providers {
		if existing.Provider.Name() == p.Provider.Name() {
			return fmt.Errorf("decision: provider %q already registered", p.Provider.Name())
		}
	}
	pl.providers = append(pl.providers, p)
	return nil
}

// Len returns the number of providers currently registered.
func (pl *ProviderPool) Len() int {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return len(pl.providers)
}

// Names returns the registered provider names, sorted for deterministic
// receipt output.
func (pl *ProviderPool) Names() []string {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	out := make([]string, 0, len(pl.providers))
	for _, p := range pl.providers {
		out = append(out, p.Provider.Name())
	}
	sort.Strings(out)
	return out
}

// PoolOutcome is what a Router.Route call returns. It carries every
// individual verdict, tagged by provider name, so downstream code (the
// Byzantine aggregator, the on-chain receipt) can attribute votes.
type PoolOutcome struct {
	// Verdicts is the concatenation of every provider's FacetVerdicts,
	// in a deterministic order (sorted by provider name, then facet
	// kind).
	Verdicts []FacetVerdict
	// Providers lists the names of providers that were actually called
	// (in sorted order).
	Providers []string
	// TrustByProvider maps provider name → the TrustLevel that was
	// applied for this decision. Down-weighted providers (e.g. an
	// observational provider that tried to vote on a critical facet)
	// appear with their original trust level; the downgrade shows up in
	// Verdicts as an ABSTAIN with a machine-readable reason.
	TrustByProvider map[string]TrustLevel
	// ErrorsByProvider maps provider name → error, if any. A provider
	// that errored contributes zero verdicts but is otherwise ignored
	// (fail-open at the pool level; the aggregator's Byzantine rule
	// still decides the final outcome).
	ErrorsByProvider map[string]error
}

// Router routes a decision to the subset of pool providers whose
// capabilities cover at least one FacetKind the caller cares about,
// calls them in parallel, and returns the merged PoolOutcome.
type Router struct {
	pool *ProviderPool
}

// NewRouter constructs a Router over the given pool.
func NewRouter(pool *ProviderPool) *Router {
	return &Router{pool: pool}
}

// ErrNoProviders is returned when Route is invoked against an empty pool.
var ErrNoProviders = errors.New("decision: provider pool is empty")

// ErrNoRoutedProviders is returned when Route runs against a non-empty
// pool but no provider in the pool has any of the requested capabilities.
var ErrNoRoutedProviders = errors.New("decision: no provider covers requested facets")

// Route fans out to every provider in the pool whose capability set
// intersects `wantedFacets`, aggregates their per-facet verdicts, and
// returns a PoolOutcome. Non-critical facets from observational providers
// pass through untouched; critical-facet verdicts from an observational
// provider are downgraded to ABSTAIN with a "trust-downgrade" reason so
// they cannot silently pass a safety check.
//
// `wantedFacets` MUST be a subset of AllFacetKinds. Passing nil or an
// empty slice is interpreted as "all facets".
func (r *Router) Route(ctx context.Context, d Decision, wantedFacets []FacetKind) (PoolOutcome, error) {
	if r == nil || r.pool == nil {
		return PoolOutcome{}, ErrNoProviders
	}
	r.pool.mu.RLock()
	providers := make([]PooledProvider, len(r.pool.providers))
	copy(providers, r.pool.providers)
	r.pool.mu.RUnlock()
	if len(providers) == 0 {
		return PoolOutcome{}, ErrNoProviders
	}
	if len(wantedFacets) == 0 {
		wantedFacets = append(wantedFacets, AllFacetKinds...)
	}
	wanted := make(map[Capability]struct{}, len(wantedFacets))
	for _, f := range wantedFacets {
		wanted[facetToCap(f)] = struct{}{}
	}

	// Select providers that cover at least one wanted facet.
	selected := make([]PooledProvider, 0, len(providers))
	for _, p := range providers {
		for _, c := range p.Capabilities {
			if _, ok := wanted[c]; ok {
				selected = append(selected, p)
				break
			}
		}
	}
	if len(selected) == 0 {
		return PoolOutcome{}, ErrNoRoutedProviders
	}

	type result struct {
		name     string
		trust    TrustLevel
		verdicts []FacetVerdict
		err      error
	}
	resultsCh := make(chan result, len(selected))

	var wg sync.WaitGroup
	for _, p := range selected {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			verdicts, err := p.Provider.Evaluate(ctx, d)
			resultsCh <- result{
				name:     p.Provider.Name(),
				trust:    p.Trust,
				verdicts: verdicts,
				err:      err,
			}
		}()
	}
	wg.Wait()
	close(resultsCh)

	var (
		merged       []FacetVerdict
		usedNames    []string
		errs         = make(map[string]error)
		trustByName  = make(map[string]TrustLevel)
		results      = make([]result, 0, len(selected))
	)
	for r := range resultsCh {
		results = append(results, r)
	}
	// Sort by provider name so both usedNames and merged verdicts are
	// deterministic across runs.
	sort.Slice(results, func(i, j int) bool { return results[i].name < results[j].name })

	for _, res := range results {
		if res.err != nil {
			errs[res.name] = res.err
			continue
		}
		trustByName[res.name] = res.trust
		usedNames = append(usedNames, res.name)
		for _, v := range sortedByKind(res.verdicts) {
			// Trust downgrade: an observational provider MUST NOT
			// carry weight on a critical facet.
			if res.trust == TrustObservational && v.Kind.isCritical() && v.Verdict != VerdictAbstain {
				v = FacetVerdict{
					Kind:       v.Kind,
					Verdict:    VerdictAbstain,
					Confidence: 0,
					Reason: fmt.Sprintf("trust-downgrade: observational provider %q cannot vote on critical facet %s",
						res.name, v.Kind),
				}
			}
			merged = append(merged, v)
		}
	}

	return PoolOutcome{
		Verdicts:         merged,
		Providers:        usedNames,
		TrustByProvider:  trustByName,
		ErrorsByProvider: errs,
	}, nil
}

func sortedByKind(vs []FacetVerdict) []FacetVerdict {
	out := make([]FacetVerdict, len(vs))
	copy(out, vs)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}
