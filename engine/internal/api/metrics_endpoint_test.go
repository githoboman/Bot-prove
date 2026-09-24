package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/observability"
)

// TestMetricsEndpoint checks the observability handler serves the
// Prometheus text exposition and that the instrument wrapper is
// counting requests.
func TestMetricsEndpoint(t *testing.T) {
	s := newTestServer("")
	if s.metrics == nil || s.httpMetric == nil {
		t.Fatalf("expected observability registry/metrics initialized on newTestServer")
	}

	// Wrap /health with the same instrumentation the server applies,
	// hit it, then check the metrics handler.
	instr := s.httpMetric.Instrument("/health", http.HandlerFunc(s.health))
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		instr.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("health: got %d", rec.Code)
		}
		// Response must carry a fresh traceparent.
		if tp := rec.Header().Get("traceparent"); !strings.HasPrefix(tp, "00-") {
			t.Errorf("missing traceparent on /health response")
		}
	}

	// Now scrape /metrics via the dedicated handler.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	observability.MetricsHandler(s.metrics).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics: got %d", rec.Code)
	}
	body := rec.Body.String()
	want := []string{
		"# TYPE cp_http_requests_total counter",
		"# TYPE cp_http_request_duration_seconds histogram",
		`cp_http_requests_total{method="GET",route="/health",status="2xx"} 3`,
	}
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in /metrics:\n%s", w, body)
		}
	}
}

// TestMetricsEndpoint_UpstreamTraceparentPropagates confirms an
// incoming W3C traceparent header on a /health request survives
// into the outgoing response with a fresh span-id.
func TestMetricsEndpoint_UpstreamTraceparentPropagates(t *testing.T) {
	s := newTestServer("")
	upstream := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	instr := s.httpMetric.Instrument("/health", http.HandlerFunc(s.health))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("traceparent", upstream)
	instr.ServeHTTP(rec, req)

	out := rec.Header().Get("traceparent")
	if !strings.HasPrefix(out, "00-0af7651916cd43dd8448eb211c80319c-") {
		t.Errorf("upstream trace_id not propagated: %q", out)
	}
	if strings.Contains(out, "b7ad6b7169203331") {
		t.Errorf("response reused upstream span_id: %q", out)
	}
}
