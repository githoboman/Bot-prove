package submitter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/make-software/casper-go-sdk/v2/rpc"
)

// circuitState models the classic three-state circuit breaker.
type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// ErrCircuitOpen is returned when a call is rejected because the breaker
// is open and the cooldown window has not elapsed yet.
var ErrCircuitOpen = errors.New("circuit breaker open: contract query calls are suspended")

// RetryConfig controls the retry/backoff behavior for contract queries.
type RetryConfig struct {
	MaxAttempts int           // total attempts, including the first (>=1)
	BaseDelay   time.Duration // delay before the 2nd attempt
	MaxDelay    time.Duration // cap on backoff delay
	Multiplier  float64       // exponential backoff multiplier
}

// DefaultRetryConfig is a sane default for querying a Casper node over
// JSON-RPC: a handful of quick retries with capped exponential backoff.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 4,
		BaseDelay:   200 * time.Millisecond,
		MaxDelay:    3 * time.Second,
		Multiplier:  2.0,
	}
}

// CircuitBreakerConfig controls when the breaker opens and how long it
// stays open before probing again.
type CircuitBreakerConfig struct {
	FailureThreshold int           // consecutive failures required to open the circuit
	OpenTimeout      time.Duration // how long the circuit stays open before a half-open probe
}

// DefaultCircuitBreakerConfig is a sane default: 5 consecutive failures
// trips the breaker for 30s before a single probe request is allowed.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		OpenTimeout:      30 * time.Second,
	}
}

// GlobalStateQuerier is the minimal subset of rpc.Client's read-only
// global-state query surface that ResilientQuerier needs. rpc.Client
// satisfies it directly; tests can supply a lightweight fake.
type GlobalStateQuerier interface {
	QueryLatestGlobalState(ctx context.Context, key string, path []string) (rpc.QueryGlobalStateResult, error)
}

// ResilientQuerier wraps a casper-go-sdk rpc.Client's read-only global
// state queries with retry-with-backoff and a circuit breaker, so that a
// flaky or temporarily-unreachable Casper node degrades gracefully
// instead of hammering the node or blocking callers indefinitely.
type ResilientQuerier struct {
	client GlobalStateQuerier
	retry  RetryConfig
	cb     CircuitBreakerConfig

	mu              sync.Mutex
	state           circuitState
	consecutiveFail int
	openedAt        time.Time
	probeInFlight   bool // true while a half-open probe call has not yet completed
}

// NewResilientQuerier builds a ResilientQuerier around an existing
// casper-go-sdk rpc.Client using the supplied retry/breaker config.
func NewResilientQuerier(client GlobalStateQuerier, retry RetryConfig, cb CircuitBreakerConfig) *ResilientQuerier {
	return &ResilientQuerier{
		client: client,
		retry:  retry,
		cb:     cb,
		state:  circuitClosed,
	}
}

// State reports the breaker's current state, useful for tests and health
// endpoints (never blocks/mutates).
func (q *ResilientQuerier) State() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.stateLocked().String()
}

func (s circuitState) String() string {
	switch s {
	case circuitClosed:
		return "closed"
	case circuitOpen:
		return "open"
	case circuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// stateLocked reports the current logical state, resolving an expired
// open circuit into half-open once OpenTimeout has elapsed. It does not
// mutate q.state; callers that need to transition into half-open must do
// so explicitly (see allow). Caller must hold q.mu.
func (q *ResilientQuerier) stateLocked() circuitState {
	if q.state == circuitOpen && time.Since(q.openedAt) >= q.cb.OpenTimeout {
		return circuitHalfOpen
	}
	return q.state
}

// allow decides whether a new call may proceed, and returns whether this
// call is a half-open probe (which, on failure, must immediately reopen
// the circuit rather than counting toward the threshold).
//
// Concurrency: only ONE probe may be in-flight at a time. Additional
// callers arriving during the half-open window while a probe is already
// executing are rejected with ErrCircuitOpen (the caller sees
// proceed=false, probe=false), rather than being let through as extra
// concurrent probes. This preserves the classic circuit-breaker
// contract: recovery is validated by a single lightweight probe, not by
// a thundering herd against a still-flaky node.
func (q *ResilientQuerier) allow() (proceed bool, probe bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	switch q.stateLocked() {
	case circuitOpen:
		return false, false
	case circuitHalfOpen:
		// If another probe is already in flight, reject this caller: we
		// don't want N goroutines all treating themselves as "the" probe
		// and hammering the recovering node.
		if q.probeInFlight {
			return false, false
		}
		// Promote bookkeeping state to half-open and claim the probe
		// slot; subsequent concurrent callers will hit the guard above.
		q.state = circuitHalfOpen
		q.probeInFlight = true
		return true, true
	default:
		return true, false
	}
}

func (q *ResilientQuerier) onSuccess(probe bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.consecutiveFail = 0
	q.state = circuitClosed
	if probe {
		q.probeInFlight = false
	}
}

func (q *ResilientQuerier) onFailure(probe bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if probe {
		// Half-open probe failed: reopen immediately, reset the timer,
		// release the probe slot so the next window can probe again.
		q.state = circuitOpen
		q.openedAt = time.Now()
		q.probeInFlight = false
		return
	}

	q.consecutiveFail++
	if q.consecutiveFail >= q.cb.FailureThreshold {
		q.state = circuitOpen
		q.openedAt = time.Now()
	}
}

// backoffDelay returns the delay before attempt N (1-indexed attempt
// number of the *next* try), capped at MaxDelay.
func (r RetryConfig) backoffDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	d := float64(r.BaseDelay) * math.Pow(r.Multiplier, float64(attempt-1))
	if d > float64(r.MaxDelay) {
		d = float64(r.MaxDelay)
	}
	return time.Duration(d)
}

// QueryGlobalState performs a read-only global state query (via the
// underlying rpc.Client's QueryLatestGlobalState) protected by retry with
// exponential backoff and a circuit breaker. It returns ErrCircuitOpen
// without touching the network if the breaker is currently open.
func (q *ResilientQuerier) QueryGlobalState(ctx context.Context, key string, path []string) (rpc.QueryGlobalStateResult, error) {
	proceed, probe := q.allow()
	if !proceed {
		return rpc.QueryGlobalStateResult{}, ErrCircuitOpen
	}

	var lastErr error
	for attempt := 1; attempt <= q.retry.MaxAttempts; attempt++ {
		if attempt > 1 {
			delay := q.retry.backoffDelay(attempt - 1)
			select {
			case <-ctx.Done():
				return rpc.QueryGlobalStateResult{}, ctx.Err()
			case <-time.After(delay):
			}
		}

		res, err := q.client.QueryLatestGlobalState(ctx, key, path)
		if err == nil {
			q.onSuccess(probe)
			return res, nil
		}

		lastErr = err
		slog.Warn("contract query attempt failed",
			"attempt", attempt, "max_attempts", q.retry.MaxAttempts, "key", key, "err", err)

		// A probe call fails fast: don't burn retries on a half-open
		// circuit, just report the failure and reopen.
		if probe {
			break
		}
	}

	q.onFailure(probe)
	return rpc.QueryGlobalStateResult{}, fmt.Errorf("contract query failed after retries: %w", lastErr)
}
