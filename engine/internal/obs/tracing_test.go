package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func newTestTracer(w *bytes.Buffer) *Tracer {
	t := NewTracer("test-service", w)
	// deterministic clock + ids
	base := time.Unix(1700000000, 0).UTC()
	step := int64(0)
	t.clock = func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Millisecond)
	}
	// deterministic ids: fill trace and span with counter values
	var seq byte = 1
	t.idGen = func() (traceID [16]byte, spanID [8]byte) {
		for i := range traceID {
			traceID[i] = seq
		}
		for i := range spanID {
			spanID[i] = seq + 1
		}
		seq += 2
		return
	}
	return t
}

type spanRecord struct {
	Service    string         `json:"service"`
	Name       string         `json:"name"`
	TraceID    string         `json:"trace_id"`
	SpanID     string         `json:"span_id"`
	ParentID   string         `json:"parent_span_id"`
	StartUnix  int64          `json:"start_unix_nano"`
	EndUnix    int64          `json:"end_unix_nano"`
	DurationMs float64        `json:"duration_ms"`
	Status     string         `json:"status"`
	Attrs      map[string]any `json:"attrs"`
	Events     []struct {
		Time  string         `json:"time"`
		Name  string         `json:"name"`
		Attrs map[string]any `json:"attrs"`
	} `json:"events"`
}

func TestSpanBasic(t *testing.T) {
	var buf bytes.Buffer
	tr := newTestTracer(&buf)
	_, span := tr.Start(context.Background(), "test.op")
	span.SetAttr("http.route", "/foo")
	span.SetAttr("http.method", "GET")
	span.AddEvent("cache.hit", map[string]any{"key": "abc"})
	span.End()

	var rec spanRecord
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("bad json: %v — %s", err, buf.String())
	}
	if rec.Name != "test.op" || rec.Status != "OK" {
		t.Fatalf("unexpected span: %+v", rec)
	}
	if rec.Attrs["http.route"] != "/foo" || rec.Attrs["http.method"] != "GET" {
		t.Fatalf("attrs missing: %+v", rec.Attrs)
	}
	if len(rec.Events) != 1 || rec.Events[0].Name != "cache.hit" {
		t.Fatalf("event missing: %+v", rec.Events)
	}
	if len(rec.TraceID) != 32 || len(rec.SpanID) != 16 {
		t.Fatalf("bad id shape: %+v", rec)
	}
}

func TestSpanParentChild(t *testing.T) {
	var buf bytes.Buffer
	tr := newTestTracer(&buf)
	ctx, parent := tr.Start(context.Background(), "parent")
	_, child := tr.Start(ctx, "child")
	child.End()
	parent.End()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), buf.String())
	}
	var c, p spanRecord
	if err := json.Unmarshal([]byte(lines[0]), &c); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &p); err != nil {
		t.Fatal(err)
	}
	if c.TraceID != p.TraceID {
		t.Fatalf("child must share trace id with parent: %s vs %s", c.TraceID, p.TraceID)
	}
	if c.ParentID != p.SpanID {
		t.Fatalf("child parent_span_id (%s) must equal parent span id (%s)", c.ParentID, p.SpanID)
	}
	if p.ParentID != "" {
		t.Fatalf("root span must have no parent, got: %s", p.ParentID)
	}
}

func TestSpanRecordError(t *testing.T) {
	var buf bytes.Buffer
	tr := newTestTracer(&buf)
	_, span := tr.Start(context.Background(), "boom")
	span.RecordError(errors.New("bang"))
	span.End()

	var rec spanRecord
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if rec.Status != "ERROR" {
		t.Fatalf("expected ERROR status, got %q", rec.Status)
	}
	if len(rec.Events) != 1 || rec.Events[0].Attrs["exception.message"] != "bang" {
		t.Fatalf("error event missing/wrong: %+v", rec.Events)
	}
}

func TestSpanEndIdempotent(t *testing.T) {
	var buf bytes.Buffer
	tr := newTestTracer(&buf)
	_, span := tr.Start(context.Background(), "once")
	span.End()
	span.End() // must not double-emit
	if strings.Count(buf.String(), "\n") != 1 {
		t.Fatalf("End must be idempotent, got:\n%s", buf.String())
	}
}

func TestFromContextNilSafe(t *testing.T) {
	if s := FromContext(context.Background()); s != nil {
		t.Fatalf("expected nil from empty ctx")
	}
	// nil-span method calls must not panic
	var s *Span
	s.SetAttr("x", 1)
	s.AddEvent("e", nil)
	s.RecordError(errors.New("e"))
	s.End()
}
