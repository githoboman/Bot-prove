package llm

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Runner orchestrates a multi-provider LLM call with:
//
//  1. Parallel fan-out across all TierFast providers, first-non-error wins,
//     losers get cancelled the moment we have a winner.
//  2. If every TierFast provider failed, sequential fallback across
//     TierReliability providers in the order registered.
//  3. If everything real failed and fixture fallback is enabled, the
//     configured FixtureProvider returns a deterministic canned answer so
//     the demo pipeline never hard-fails.
//
// The whole operation is bounded by cfg.TotalBudget; each individual provider
// call is bounded by cfg.PerProviderTimeout.
//
// The runner is safe for concurrent use — providers/fixture are stored by
// value once at construction, and each Complete() call owns its own channels.
type Runner struct {
	fast        []Provider
	reliability []Provider
	fixture     *FixtureProvider
	cfg         Config
	// clock (test hook) — defaults to time.Now.
	now func() time.Time
}

// NewRunner builds a Runner from a heterogenous provider list plus a
// FixtureProvider. Providers with KeyCount()==0 are silently dropped (they
// were configured with no keys — Anna's Actions minutes are precious). If
// the fixture is nil, fixture fallback is disabled.
func NewRunner(providers []Provider, fixture *FixtureProvider, cfg Config) *Runner {
	r := &Runner{
		fixture: fixture,
		cfg:     cfg,
		now:     time.Now,
	}
	if cfg.PerProviderTimeout <= 0 || cfg.TotalBudget <= 0 {
		defaults := DefaultConfig()
		if cfg.PerProviderTimeout <= 0 {
			r.cfg.PerProviderTimeout = defaults.PerProviderTimeout
		}
		if cfg.TotalBudget <= 0 {
			r.cfg.TotalBudget = defaults.TotalBudget
		}
	}
	// PerProviderTimeout must never exceed TotalBudget — otherwise a single
	// provider could sleep past the total-budget deadline. If misconfigured,
	// clamp the per-provider timeout down to the total budget.
	if r.cfg.PerProviderTimeout > r.cfg.TotalBudget {
		r.cfg.PerProviderTimeout = r.cfg.TotalBudget
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		if p.KeyCount() == 0 {
			continue
		}
		switch p.Tier() {
		case TierFast:
			r.fast = append(r.fast, p)
		case TierReliability:
			r.reliability = append(r.reliability, p)
		}
	}
	// Stable ordering by ID makes fixture keys and audit logs deterministic.
	sort.SliceStable(r.fast, func(i, j int) bool { return r.fast[i].ID() < r.fast[j].ID() })
	sort.SliceStable(r.reliability, func(i, j int) bool { return r.reliability[i].ID() < r.reliability[j].ID() })
	return r
}

// ErrAllProvidersFailed is returned by Complete when every real provider
// failed and fixture fallback was disabled.
var ErrAllProvidersFailed = errors.New("llm: all providers failed and fixture is disabled")

// providerAttempt is one row of the per-call attempt log.
type providerAttempt struct {
	Provider  string        `json:"provider"`
	LatencyMs int64         `json:"latency_ms"`
	Success   bool          `json:"success"`
	Err       string        `json:"err,omitempty"`
	Retryable bool          `json:"retryable,omitempty"`
	Status    int           `json:"status,omitempty"`
	KeyIndex  int           `json:"key_index"`
	Elapsed   time.Duration `json:"-"`
}

// RunReport is the diagnostic trace of a single Runner.Complete call.
// Callers who want to hash the *audit trail* (which providers were tried,
// what they said, who won) should build the hash from RunReport plus the
// winning Response.Canonical() — not from any single field alone.
type RunReport struct {
	// Winner is the ID of the provider whose response was returned. Empty
	// on total failure.
	Winner string `json:"winner"`

	// Fixture indicates the fixture fallback served the answer.
	Fixture bool `json:"fixture"`

	// Attempts is the ordered log of every provider call made in this run.
	Attempts []providerAttempt `json:"attempts"`

	// TotalLatencyMs is the wall-clock duration of the whole run.
	TotalLatencyMs int64 `json:"total_latency_ms"`

	// FastFanOutSize is how many TierFast providers were called in parallel.
	FastFanOutSize int `json:"fast_fan_out_size"`
}

// Complete runs one full end-to-end LLM call with fan-out + fallback.
// Returns the winning Response, a diagnostic RunReport, and any error.
// On success err is nil even if some providers failed along the way — check
// RunReport.Attempts for the trace.
func (r *Runner) Complete(ctx context.Context, req Request) (*Response, *RunReport, error) {
	start := r.now()
	report := &RunReport{FastFanOutSize: len(r.fast)}

	// Enforce total budget.
	ctx, cancel := context.WithTimeout(ctx, r.cfg.TotalBudget)
	defer cancel()

	// If the operator forced fixture mode, skip real providers entirely.
	if r.cfg.ForceFixture() && r.fixture != nil {
		resp, err := r.fixture.Complete(ctx, req)
		report.TotalLatencyMs = time.Since(start).Milliseconds()
		if err != nil {
			return nil, report, err
		}
		report.Winner = resp.Provider
		report.Fixture = true
		report.Attempts = append(report.Attempts, providerAttempt{
			Provider: resp.Provider, Success: true, LatencyMs: 0,
		})
		return resp, report, nil
	}

	// Step 1: parallel fan-out across TierFast.
	if len(r.fast) > 0 {
		if resp, attempts := r.fanOutFast(ctx, req); resp != nil {
			report.Attempts = append(report.Attempts, attempts...)
			report.Winner = resp.Provider
			report.TotalLatencyMs = time.Since(start).Milliseconds()
			return resp, report, nil
		} else {
			report.Attempts = append(report.Attempts, attempts...)
		}
	}

	// Step 2: sequential fallback across TierReliability.
	for _, p := range r.reliability {
		resp, att := r.callOne(ctx, p, req)
		report.Attempts = append(report.Attempts, att)
		if resp != nil {
			report.Winner = resp.Provider
			report.TotalLatencyMs = time.Since(start).Milliseconds()
			return resp, report, nil
		}
		// Bail out if the total budget is exhausted so we don't waste time
		// on providers that can't fit anyway.
		if ctx.Err() != nil {
			break
		}
	}

	// Step 3: fixture fallback (last resort).
	if r.cfg.EnableFixtureFallback && r.fixture != nil {
		// Use a fresh context — the parent may have timed out, and we still
		// want the fixture to answer for demo continuity. Cap it at 500ms
		// so no code path hangs indefinitely.
		fxCtx, fxCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer fxCancel()
		resp, err := r.fixture.Complete(fxCtx, req)
		if err == nil {
			report.Winner = resp.Provider
			report.Fixture = true
			report.Attempts = append(report.Attempts, providerAttempt{
				Provider: resp.Provider, Success: true, LatencyMs: 0,
			})
			report.TotalLatencyMs = time.Since(start).Milliseconds()
			return resp, report, nil
		}
	}

	report.TotalLatencyMs = time.Since(start).Milliseconds()
	return nil, report, ErrAllProvidersFailed
}

// fanOutFast calls every TierFast provider concurrently. First non-error wins;
// the losers are cancelled as soon as we have a winner. Returns the winning
// response (may be nil if all failed) plus the per-attempt log.
func (r *Runner) fanOutFast(ctx context.Context, req Request) (*Response, []providerAttempt) {
	type result struct {
		resp *Response
		att  providerAttempt
	}

	results := make(chan result, len(r.fast))
	fanCtx, cancelFan := context.WithCancel(ctx)
	defer cancelFan()

	var wg sync.WaitGroup
	for _, p := range r.fast {
		wg.Add(1)
		go func(prov Provider) {
			defer wg.Done()
			resp, att := r.callOne(fanCtx, prov, req)
			// Non-blocking send — if the winner already cancelled us, the
			// buffered channel absorbs stragglers without blocking.
			select {
			case results <- result{resp: resp, att: att}:
			default:
			}
		}(p)
	}

	// Collector: wait for the first success or until every goroutine finishes.
	var attempts []providerAttempt
	winnerCh := make(chan *Response, 1)
	go func() {
		wg.Wait()
		close(results)
	}()

	for res := range results {
		attempts = append(attempts, res.att)
		if res.resp != nil {
			// First success — cancel remaining fan-out goroutines.
			cancelFan()
			select {
			case winnerCh <- res.resp:
			default:
			}
			// Drain the rest to collect their attempts for the audit trail.
			for extra := range results {
				attempts = append(attempts, extra.att)
			}
			break
		}
	}

	select {
	case w := <-winnerCh:
		return w, sortAttemptsByStart(attempts)
	default:
		return nil, sortAttemptsByStart(attempts)
	}
}

// callOne invokes a single provider under PerProviderTimeout, and captures
// the attempt in a providerAttempt row.
func (r *Runner) callOne(ctx context.Context, prov Provider, req Request) (*Response, providerAttempt) {
	att := providerAttempt{Provider: prov.ID()}
	callStart := r.now()

	// Per-provider timeout (bounded by parent ctx).
	callCtx, cancel := context.WithTimeout(ctx, r.cfg.PerProviderTimeout)
	defer cancel()

	resp, err := prov.Complete(callCtx, req)
	att.LatencyMs = time.Since(callStart).Milliseconds()

	if err != nil {
		att.Success = false
		att.Err = err.Error()
		if pe, ok := err.(*ProviderError); ok {
			att.Retryable = pe.Retryable
			att.Status = pe.StatusCode
		}
		return nil, att
	}
	att.Success = true
	att.KeyIndex = resp.KeyIndex
	return resp, att
}

// sortAttemptsByStart returns attempts as-is; kept as a hook if future
// versions want to reorder attempts by wall-clock start time for the
// audit trail.
func sortAttemptsByStart(a []providerAttempt) []providerAttempt {
	return a
}

// PollResult is one provider's response for Runner.Poll.
type PollResult struct {
	// Provider is the provider ID ("groq", "gemini", ...).
	Provider string

	// Resp is the successful response, or nil on failure.
	Resp *Response

	// Attempt is the full attempt trace (latency, error, key index).
	Attempt providerAttempt
}

// Poll calls EVERY registered provider (fast + reliability) in parallel and
// returns one PollResult per provider. Unlike Complete (winner-take-all),
// Poll waits for all providers to answer or the TotalBudget to expire.
//
// This is the entry point used by the facet-based judge: every provider
// gets an independent shot at the same prompt, and the judge aggregates
// their votes into AGREE / DISAGREE / ABSTAIN.
//
// If EnableFixtureFallback is set AND zero providers answered successfully,
// the fixture is invoked once and appended as an extra result. Otherwise
// the fixture is not polled — a judge with real answers should NOT be
// influenced by fixture data.
func (r *Runner) Poll(ctx context.Context, req Request) []PollResult {
	all := make([]Provider, 0, len(r.fast)+len(r.reliability))
	all = append(all, r.fast...)
	all = append(all, r.reliability...)

	ctx, cancel := context.WithTimeout(ctx, r.cfg.TotalBudget)
	defer cancel()

	if r.cfg.ForceFixture() && r.fixture != nil {
		resp, err := r.fixture.Complete(ctx, req)
		att := providerAttempt{Provider: r.fixture.ID()}
		if err != nil {
			att.Err = err.Error()
		} else {
			att.Success = true
		}
		return []PollResult{{Provider: r.fixture.ID(), Resp: resp, Attempt: att}}
	}

	results := make([]PollResult, len(all))
	var wg sync.WaitGroup
	for i, p := range all {
		wg.Add(1)
		go func(idx int, prov Provider) {
			defer wg.Done()
			resp, att := r.callOne(ctx, prov, req)
			results[idx] = PollResult{Provider: prov.ID(), Resp: resp, Attempt: att}
		}(i, p)
	}
	wg.Wait()

	// Count real successes.
	liveCount := 0
	for _, res := range results {
		if res.Resp != nil {
			liveCount++
		}
	}

	// Fixture fallback only when zero real providers answered.
	if liveCount == 0 && r.cfg.EnableFixtureFallback && r.fixture != nil {
		fxCtx, fxCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer fxCancel()
		resp, err := r.fixture.Complete(fxCtx, req)
		att := providerAttempt{Provider: r.fixture.ID()}
		if err != nil {
			att.Err = err.Error()
		} else {
			att.Success = true
		}
		results = append(results, PollResult{Provider: r.fixture.ID(), Resp: resp, Attempt: att})
	}

	return results
}

// String returns a compact multi-line dump of the report — handy for logs.
func (r *RunReport) String() string {
	var b []byte
	b = append(b, fmt.Sprintf("winner=%s fixture=%v total_ms=%d fan_out=%d attempts=%d\n",
		r.Winner, r.Fixture, r.TotalLatencyMs, r.FastFanOutSize, len(r.Attempts))...)
	for _, a := range r.Attempts {
		if a.Success {
			b = append(b, fmt.Sprintf("  ok  %-12s %4dms key=%d\n", a.Provider, a.LatencyMs, a.KeyIndex)...)
		} else {
			b = append(b, fmt.Sprintf("  err %-12s %4dms status=%d retryable=%v %s\n",
				a.Provider, a.LatencyMs, a.Status, a.Retryable, a.Err)...)
		}
	}
	return string(b)
}
