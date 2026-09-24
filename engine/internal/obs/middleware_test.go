package obs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareRED(t *testing.T) {
	r := NewRegistry()
	m := NewHTTPMetrics(r)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	handler := m.MiddlewareRoute(nil, mux, MuxRouteResolver(mux))

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))
		if w.Code != 200 {
			t.Fatalf("bad code: %d", w.Code)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if w.Code != 500 {
		t.Fatalf("bad code: %d", w.Code)
	}

	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("expose: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `cp_http_requests_total{method="GET",route="GET /ok",status="200"} 3`) {
		t.Fatalf("ok counter wrong:\n%s", out)
	}
	if !strings.Contains(out, `cp_http_requests_total{method="GET",route="GET /boom",status="500"} 1`) {
		t.Fatalf("boom counter wrong:\n%s", out)
	}
	if !strings.Contains(out, `cp_http_errors_total{method="GET",route="GET /boom",status="500"} 1`) {
		t.Fatalf("errors counter wrong:\n%s", out)
	}
	if !strings.Contains(out, `cp_http_request_duration_seconds_count{method="GET",route="GET /ok"} 3`) {
		t.Fatalf("hist count wrong:\n%s", out)
	}
}

func TestMiddlewareTracing(t *testing.T) {
	var traceBuf bytes.Buffer
	tr := newTestTracer(&traceBuf)
	r := NewRegistry()
	m := NewHTTPMetrics(r)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /submit", func(w http.ResponseWriter, r *http.Request) {
		span := FromContext(r.Context())
		if span == nil {
			t.Fatalf("expected active span in handler ctx")
		}
		span.SetAttr("cp.route.custom", "yes")
		w.WriteHeader(http.StatusCreated)
	})

	handler := m.MiddlewareRoute(tr, mux, MuxRouteResolver(mux))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/submit", nil))
	if w.Code != 201 {
		t.Fatalf("bad code: %d", w.Code)
	}

	var rec struct {
		Name     string         `json:"name"`
		Status   string         `json:"status"`
		Attrs    map[string]any `json:"attrs"`
		ParentID string         `json:"parent_span_id"`
	}
	if err := json.Unmarshal(traceBuf.Bytes(), &rec); err != nil {
		t.Fatalf("bad trace json: %v — %s", err, traceBuf.String())
	}
	if rec.Name != "http.request" || rec.Status != "OK" {
		t.Fatalf("unexpected span: %+v", rec)
	}
	if rec.Attrs["http.method"] != "POST" {
		t.Fatalf("missing method attr: %+v", rec.Attrs)
	}
	if rec.Attrs["http.route"] != "POST /submit" {
		t.Fatalf("missing/wrong route attr: %+v", rec.Attrs)
	}
	if rec.Attrs["cp.route.custom"] != "yes" {
		t.Fatalf("custom handler attr missing: %+v", rec.Attrs)
	}
}

func TestMetricsHandler(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("cp_handler_test", "h")
	c.Inc()

	h := Handler(r)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("wrong content-type: %q", ct)
	}
	if !strings.Contains(w.Body.String(), "cp_handler_test 1") {
		t.Fatalf("missing counter in output: %s", w.Body.String())
	}
}
