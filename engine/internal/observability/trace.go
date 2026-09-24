// W3C Trace Context propagation.
//
// Implements a minimal, spec-conformant parser+generator for the
// `traceparent` header defined in the W3C Trace Context Level 1 REC
// (https://www.w3.org/TR/trace-context-1/). This is enough to let a
// remote caller propagate their trace-id through our engine and appear
// as a linked span in any OTel-compatible collector downstream — even
// without pulling in the full OTel SDK.
//
// Format: `version-trace_id-parent_id-flags`.
//   - version: 2 lowercase-hex chars (current: "00")
//   - trace_id: 32 lowercase-hex chars (128 bits, nonzero)
//   - parent_id (aka span-id): 16 lowercase-hex chars (64 bits, nonzero)
//   - flags: 2 lowercase-hex chars (bit 0 = sampled)
//
// We do NOT parse `tracestate` — that is a vendor-specific extension
// point, and forwarding it verbatim is safe (we just copy the header).
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

// SpanContext is the minimal set of fields we track per request.
type SpanContext struct {
	TraceID    string // 32 hex chars
	SpanID     string // 16 hex chars — this request's own span
	ParentID   string // 16 hex chars — inherited from upstream traceparent, empty if root
	Flags      byte   // bit 0 = sampled
	TraceState string // opaque, forwarded verbatim
}

// Sampled reports whether the sampled bit is set.
func (s SpanContext) Sampled() bool {
	return s.Flags&0x01 != 0
}

// Traceparent renders the header value.
func (s SpanContext) Traceparent() string {
	return "00-" + s.TraceID + "-" + s.SpanID + "-" + hexByte(s.Flags)
}

// ErrInvalidTraceparent is returned when the header cannot be parsed.
var ErrInvalidTraceparent = errors.New("observability: invalid traceparent header")

// ParseTraceparent parses a W3C traceparent header. The spec says we
// MUST accept future versions with more fields but treat unknown
// content conservatively; here we accept version "00" and reject
// anything else (a receiver behind an OTel gateway will normalise).
func ParseTraceparent(hdr string) (SpanContext, error) {
	parts := strings.Split(hdr, "-")
	if len(parts) < 4 {
		return SpanContext{}, ErrInvalidTraceparent
	}
	if parts[0] != "00" {
		return SpanContext{}, ErrInvalidTraceparent
	}
	if len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return SpanContext{}, ErrInvalidTraceparent
	}
	if !isLowerHex(parts[1]) || !isLowerHex(parts[2]) || !isLowerHex(parts[3]) {
		return SpanContext{}, ErrInvalidTraceparent
	}
	// The all-zero trace-id / parent-id are invalid per spec.
	if allZero(parts[1]) || allZero(parts[2]) {
		return SpanContext{}, ErrInvalidTraceparent
	}
	flagsB, err := hex.DecodeString(parts[3])
	if err != nil {
		return SpanContext{}, ErrInvalidTraceparent
	}
	return SpanContext{
		TraceID:  parts[1],
		ParentID: parts[2],
		Flags:    flagsB[0],
	}, nil
}

// NewRootSpan starts a fresh trace with a random trace-id and span-id.
// Sampled=true by default; adjust by caller before emit.
func NewRootSpan() SpanContext {
	return SpanContext{
		TraceID: randHex(16),
		SpanID:  randHex(8),
		Flags:   0x01,
	}
}

// ContinueSpan derives a child span from an upstream traceparent. The
// child inherits the trace-id and generates a fresh span-id. On parse
// error, returns a fresh root — that matches the spec's forgiveness
// posture (bad upstream headers must not break the request).
func ContinueSpan(traceparent string) SpanContext {
	if traceparent == "" {
		return NewRootSpan()
	}
	parsed, err := ParseTraceparent(traceparent)
	if err != nil {
		return NewRootSpan()
	}
	parsed.SpanID = randHex(8)
	return parsed
}

// Context key for storing a SpanContext.
type ctxKey struct{}

// WithSpanContext attaches sc to ctx.
func WithSpanContext(ctx context.Context, sc SpanContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, sc)
}

// SpanContextFromContext extracts an attached span, or a zero value if
// none is present.
func SpanContextFromContext(ctx context.Context) (SpanContext, bool) {
	if ctx == nil {
		return SpanContext{}, false
	}
	sc, ok := ctx.Value(ctxKey{}).(SpanContext)
	return sc, ok
}

// --- helpers ---

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func allZero(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is unrecoverable at process level; fall
		// back to zeros with the low bits stamped so the value is not
		// literally all-zero (which the spec forbids).
		b[len(b)-1] = 0x01
	}
	return hex.EncodeToString(b)
}

func hexByte(b byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[(b>>4)&0x0f], digits[b&0x0f]})
}
