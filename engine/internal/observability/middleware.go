// HTTP middleware that wires metrics + trace propagation into any
// http.Handler chain.
//
// The middleware:
//   - increments a per-method+status counter,
//   - observes a per-method+route latency histogram,
//   - parses the incoming `traceparent` header (or generates a root),
//     stashes the SpanContext in request.Context, and echoes the
//     resulting traceparent back on the response so downstream
//     services (or the caller's own tracing) see a linked span.
//
// `routeLabel` is a function the caller supplies to turn a request
// into a low-cardinality label — typically the matched route pattern,
// NOT the raw path (which would spike cardinality).
package observability

import (
	"net/http"
	"time"
)

// HTTPMetrics groups the three request-scoped metrics.
type HTTPMetrics struct {
	Requests *Counter
	Latency  *Histogram
	InFlight *Gauge
}

// NewHTTPMetrics registers the standard trio on r under the given
// name prefix (e.g. "cp_http").
func NewHTTPMetrics(r *Registry, prefix string) *HTTPMetrics {
	return &HTTPMetrics{
		Requests: r.NewCounter(prefix+"_requests_total",
			"total HTTP requests processed",
			"method", "route", "status"),
		Latency: r.NewHistogram(prefix+"_request_duration_seconds",
			"HTTP request latency in seconds",
			DefaultBuckets,
			"method", "route"),
		InFlight: r.NewGauge(prefix+"_requests_in_flight",
			"HTTP requests currently being served"),
	}
}

// statusRecorder captures the response status without buffering the body.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.wrote {
		sr.status = code
		sr.wrote = true
	}
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.wrote {
		// Implicit 200 on first Write.
		sr.status = http.StatusOK
		sr.wrote = true
	}
	return sr.ResponseWriter.Write(b)
}

// Instrument wraps a handler with metric recording + W3C trace context
// propagation. `route` is the low-cardinality label to attribute the
// call to (e.g. `"/v1/proofs"` or `"POST /proofs/{id}"`).
func (m *HTTPMetrics) Instrument(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Trace context: continue upstream or start fresh.
		sc := ContinueSpan(r.Header.Get("traceparent"))
		if ts := r.Header.Get("tracestate"); ts != "" {
			sc.TraceState = ts
		}
		w.Header().Set("traceparent", sc.Traceparent())
		if sc.TraceState != "" {
			w.Header().Set("tracestate", sc.TraceState)
		}
		ctx := WithSpanContext(r.Context(), sc)
		r = r.WithContext(ctx)

		if m.InFlight != nil {
			m.InFlight.Add(1)
			defer m.InFlight.Add(-1)
		}

		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sr, r)
		dur := time.Since(start).Seconds()

		if m.Latency != nil {
			m.Latency.Observe(dur, r.Method, route)
		}
		if m.Requests != nil {
			m.Requests.Inc(r.Method, route, httpStatusLabel(sr.status))
		}
	})
}

// InstrumentAll is a lighter variant that labels every request as
// `route="all"`. Useful for a global wrapper when the router is a
// simple http.ServeMux and per-route wrapping is not practical.
func (m *HTTPMetrics) InstrumentAll(next http.Handler) http.Handler {
	return m.Instrument("all", next)
}

// MetricsHandler is a ready-made http.Handler for /metrics that
// serves the registry's text exposition.
func MetricsHandler(r *Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = r.WriteText(w)
	})
}

// httpStatusLabel groups status codes into their standard class
// (2xx, 4xx, ...) to keep label cardinality bounded. If the exact
// code is needed the caller can extend with a dedicated counter.
func httpStatusLabel(code int) string {
	switch {
	case code >= 100 && code < 200:
		return "1xx"
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	}
	return "unknown"
}
