package llm

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ProviderError is a structured error returned by a Provider.Complete() call.
// It carries the HTTP status, whether the runner may retry, and a truncated
// body excerpt for logs (never full — providers sometimes echo prompts).
type ProviderError struct {
	Provider   string // provider ID, e.g. "groq"
	StatusCode int    // 0 if the error was pre-network (marshal, no keys, etc.)
	Retryable  bool   // if true, the runner should try another key/provider
	Body       string // truncated response body (max 512 chars)
	Cause      error  // wrapped underlying error, if any
}

// Error implements error.
func (e *ProviderError) Error() string {
	if e.Cause != nil && e.StatusCode == 0 {
		return fmt.Sprintf("%s: %v", e.Provider, e.Cause)
	}
	if e.StatusCode > 0 {
		if e.Body != "" {
			return fmt.Sprintf("%s: http %d: %s", e.Provider, e.StatusCode, e.Body)
		}
		return fmt.Sprintf("%s: http %d", e.Provider, e.StatusCode)
	}
	return fmt.Sprintf("%s: unknown error", e.Provider)
}

// Unwrap surfaces the underlying cause for errors.Is/As.
func (e *ProviderError) Unwrap() error { return e.Cause }

// IsRetryable is a convenience predicate for the parallel runner.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if pe, ok := err.(*ProviderError); ok {
		return pe.Retryable
	}
	// Network/timeout errors are almost always retryable at the runner level
	// (another provider might answer), so err on the side of retry.
	return true
}

// isRateLimitStatus reports whether the HTTP status warrants resting the
// current key and retrying with another.
func isRateLimitStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusPaymentRequired // some providers use 402 for quota
}

// isAuthFailure reports whether the HTTP status means the key itself is bad.
// A bad key should be Rested indefinitely rather than briefly cooled.
func isAuthFailure(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}

// parseRetryAfter parses an HTTP Retry-After header (seconds-only, we don't
// bother with HTTP-date format — providers use seconds in practice).
// Returns zero if unparsable.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 0
	}
	// Clamp to a sane ceiling — a 1h Retry-After effectively disables the key
	// for the demo window, which we don't want.
	if secs > 300 {
		secs = 300
	}
	return time.Duration(secs) * time.Second
}

// truncateBody clips a response body for safe logging.
func truncateBody(raw []byte) string {
	s := string(raw)
	const max = 512
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
