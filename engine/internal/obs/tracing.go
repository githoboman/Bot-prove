package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Tracer is a lightweight, dependency-free span emitter that serialises spans
// as JSON records in an OpenTelemetry-compatible shape. Each Span, when its
// End() is called, is flushed to the underlying io.Writer (default: stderr).
//
// The JSON shape is intentionally OTel-flavoured — Trace ID and Span ID are
// hex-encoded per the W3C Trace Context spec, timestamps are ISO 8601 UTC,
// and attribute keys follow OTel semantic conventions (http.*, service.*).
// This makes it trivial to shell into a sidecar that reads NDJSON and
// converts to OTLP, but the transport is NOT included here.
type Tracer struct {
	name string
	mu   sync.Mutex
	w    io.Writer
	// clock is injectable for tests.
	clock func() time.Time
	// idGen is injectable for tests.
	idGen func() (traceID [16]byte, spanID [8]byte)
}

// NewTracer creates a tracer that emits NDJSON spans to w. Pass nil for
// stderr.
func NewTracer(serviceName string, w io.Writer) *Tracer {
	if w == nil {
		w = os.Stderr
	}
	return &Tracer{
		name:  serviceName,
		w:     w,
		clock: time.Now,
		idGen: randomIDs,
	}
}

// Span is a single OTel-flavoured span in flight.
type Span struct {
	tracer   *Tracer
	name     string
	traceID  [16]byte
	spanID   [8]byte
	parentID [8]byte
	start    time.Time
	attrs    map[string]any
	events   []spanEvent
	status   string
	ended    bool
	mu       sync.Mutex
}

type spanEvent struct {
	Time  time.Time      `json:"time"`
	Name  string         `json:"name"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// ctxKey is used for storing the current span in context.
type ctxKey struct{}

// Start creates a new span. If ctx already contains a parent span, the new
// span inherits its trace ID and links back via parent span ID.
func (t *Tracer) Start(ctx context.Context, name string) (context.Context, *Span) {
	tid, sid := t.idGen()
	var parent [8]byte
	if p, ok := ctx.Value(ctxKey{}).(*Span); ok && p != nil {
		tid = p.traceID
		parent = p.spanID
	}
	s := &Span{
		tracer:   t,
		name:     name,
		traceID:  tid,
		spanID:   sid,
		parentID: parent,
		start:    t.clock(),
		attrs:    make(map[string]any),
		status:   "OK",
	}
	return context.WithValue(ctx, ctxKey{}, s), s
}

// FromContext returns the current span, or nil.
func FromContext(ctx context.Context) *Span {
	s, _ := ctx.Value(ctxKey{}).(*Span)
	return s
}

// SetAttr sets a single attribute. Nil values are stored as JSON null.
func (s *Span) SetAttr(key string, val any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.attrs[key] = val
	s.mu.Unlock()
}

// AddEvent records a named event with optional attrs.
func (s *Span) AddEvent(name string, attrs map[string]any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.events = append(s.events, spanEvent{Time: s.tracer.clock(), Name: name, Attrs: attrs})
	s.mu.Unlock()
}

// SetStatus sets OK, ERROR, or UNSET.
func (s *Span) SetStatus(status string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

// RecordError sets status ERROR and adds an event with the error message.
func (s *Span) RecordError(err error) {
	if s == nil || err == nil {
		return
	}
	s.AddEvent("exception", map[string]any{"exception.message": err.Error()})
	s.SetStatus("ERROR")
}

// End finalises the span and writes it to the tracer's writer. Safe to call
// multiple times (subsequent calls no-op).
func (s *Span) End() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	end := s.tracer.clock()
	rec := struct {
		Service    string         `json:"service"`
		Name       string         `json:"name"`
		TraceID    string         `json:"trace_id"`
		SpanID     string         `json:"span_id"`
		ParentID   string         `json:"parent_span_id,omitempty"`
		StartUnix  int64          `json:"start_unix_nano"`
		EndUnix    int64          `json:"end_unix_nano"`
		DurationMs float64        `json:"duration_ms"`
		Status     string         `json:"status"`
		Attrs      map[string]any `json:"attrs,omitempty"`
		Events     []spanEvent    `json:"events,omitempty"`
	}{
		Service:    s.tracer.name,
		Name:       s.name,
		TraceID:    hex.EncodeToString(s.traceID[:]),
		SpanID:     hex.EncodeToString(s.spanID[:]),
		ParentID:   parentIDString(s.parentID),
		StartUnix:  s.start.UnixNano(),
		EndUnix:    end.UnixNano(),
		DurationMs: float64(end.Sub(s.start).Microseconds()) / 1000.0,
		Status:     s.status,
		Attrs:      s.attrs,
		Events:     s.events,
	}
	s.mu.Unlock()

	s.tracer.mu.Lock()
	_ = json.NewEncoder(s.tracer.w).Encode(rec)
	s.tracer.mu.Unlock()
}

// TraceID returns the 16-byte hex trace id for correlation with logs.
func (s *Span) TraceID() string {
	if s == nil {
		return ""
	}
	return hex.EncodeToString(s.traceID[:])
}

// SpanID returns the 8-byte hex span id.
func (s *Span) SpanID() string {
	if s == nil {
		return ""
	}
	return hex.EncodeToString(s.spanID[:])
}

func parentIDString(id [8]byte) string {
	var zero [8]byte
	if id == zero {
		return ""
	}
	return hex.EncodeToString(id[:])
}

func randomIDs() (traceID [16]byte, spanID [8]byte) {
	_, _ = rand.Read(traceID[:])
	_, _ = rand.Read(spanID[:])
	return
}
