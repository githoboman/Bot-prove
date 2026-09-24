package observability

import (
	"context"
	"strings"
	"testing"
)

func TestParseTraceparent_Valid(t *testing.T) {
	hdr := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	sc, err := ParseTraceparent(hdr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sc.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("bad trace_id: %s", sc.TraceID)
	}
	if sc.ParentID != "b7ad6b7169203331" {
		t.Errorf("bad parent_id: %s", sc.ParentID)
	}
	if !sc.Sampled() {
		t.Errorf("expected sampled=true")
	}
}

func TestParseTraceparent_Invalid(t *testing.T) {
	cases := []string{
		"",
		"01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", // wrong version
		"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331",    // missing flags
		"00-0af7651916cd43dd8448eb211c80319-b7ad6b7169203331-01",  // trace_id too short
		"00-00000000000000000000000000000000-b7ad6b7169203331-01", // all-zero trace
		"00-0af7651916cd43dd8448eb211c80319c-0000000000000000-01", // all-zero parent
		"00-XAf7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", // uppercase hex forbidden
	}
	for _, c := range cases {
		if _, err := ParseTraceparent(c); err == nil {
			t.Errorf("expected error on %q", c)
		}
	}
}

func TestContinueSpan_InheritsTraceID(t *testing.T) {
	hdr := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	child := ContinueSpan(hdr)
	if child.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("child did not inherit trace_id: %s", child.TraceID)
	}
	if child.SpanID == "b7ad6b7169203331" {
		t.Errorf("child re-used parent span_id")
	}
	if child.ParentID != "b7ad6b7169203331" {
		t.Errorf("child did not record parent: %s", child.ParentID)
	}
}

func TestContinueSpan_EmptyHeaderRoots(t *testing.T) {
	sc := ContinueSpan("")
	if len(sc.TraceID) != 32 || len(sc.SpanID) != 16 {
		t.Errorf("bad root ids: %+v", sc)
	}
}

func TestContinueSpan_BadHeaderRoots(t *testing.T) {
	sc := ContinueSpan("this is not a traceparent")
	if len(sc.TraceID) != 32 {
		t.Errorf("bad root trace_id: %s", sc.TraceID)
	}
	if sc.ParentID != "" {
		t.Errorf("root should have empty parent, got %s", sc.ParentID)
	}
}

func TestTraceparent_Roundtrip(t *testing.T) {
	sc := NewRootSpan()
	hdr := sc.Traceparent()
	if !strings.HasPrefix(hdr, "00-") {
		t.Errorf("missing version prefix: %s", hdr)
	}
	parsed, err := ParseTraceparent(hdr)
	if err != nil {
		t.Fatalf("roundtrip failed: %v", err)
	}
	if parsed.TraceID != sc.TraceID {
		t.Errorf("trace_id changed: %s vs %s", sc.TraceID, parsed.TraceID)
	}
}

func TestSpanContext_ContextRoundtrip(t *testing.T) {
	ctx := context.Background()
	sc := NewRootSpan()
	ctx = WithSpanContext(ctx, sc)
	got, ok := SpanContextFromContext(ctx)
	if !ok {
		t.Fatalf("no span in context")
	}
	if got.TraceID != sc.TraceID {
		t.Errorf("trace_id mismatch")
	}
}
