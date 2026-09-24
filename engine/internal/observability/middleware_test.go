package observability

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPMetrics_CountsAndTraceparent(t *testing.T) {
	r := NewRegistry()
	m := NewHTTPMetrics(r, "cp_http")

	h := m.Instrument("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))

	// 3 successful GETs, 1 failing GET.
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/health", nil)
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		// Response must carry a traceparent (root span since no header supplied).
		if tp := w.Header().Get("traceparent"); !strings.HasPrefix(tp, "00-") {
			t.Errorf("missing traceparent on response: %q", tp)
		}
	}

	// 4xx path.
	h4 := m.Instrument("/oops", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusBadRequest)
	}))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oops", nil)
	h4.ServeHTTP(w, req)

	var buf bytes.Buffer
	if err := r.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	out := buf.String()

	must := []string{
		`cp_http_requests_total{method="GET",route="/health",status="2xx"} 3`,
		`cp_http_requests_total{method="GET",route="/oops",status="4xx"} 1`,
		`cp_http_request_duration_seconds_count{method="GET",route="/health"} 3`,
	}
	for _, s := range must {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in:\n%s", s, out)
		}
	}
}

func TestHTTPMetrics_UpstreamTraceparentInherited(t *testing.T) {
	r := NewRegistry()
	m := NewHTTPMetrics(r, "cp_http")

	upstream := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"

	var seenInHandler string
	h := m.Instrument("/x", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc, ok := SpanContextFromContext(r.Context())
		if ok {
			seenInHandler = sc.TraceID
		}
		w.WriteHeader(200)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("traceparent", upstream)
	h.ServeHTTP(w, req)

	if seenInHandler != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("handler did not see inherited trace_id: %q", seenInHandler)
	}
	resp := w.Header().Get("traceparent")
	if !strings.HasPrefix(resp, "00-0af7651916cd43dd8448eb211c80319c-") {
		t.Errorf("response traceparent did not preserve trace_id: %q", resp)
	}
	// The span_id must differ from the upstream one.
	if strings.Contains(resp, "b7ad6b7169203331") {
		t.Errorf("response reused upstream span_id: %q", resp)
	}
}

func TestMetricsHandler_ServesText(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("test_total", "test")
	c.Inc()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	MetricsHandler(r).ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("bad content-type: %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "test_total 1") {
		t.Errorf("missing counter in body:\n%s", body)
	}
}
