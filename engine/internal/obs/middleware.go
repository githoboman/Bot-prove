package obs

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// HTTPMetrics bundles the standard HTTP RED metrics: request count, latency,
// in-flight, and error count.
type HTTPMetrics struct {
	Requests   *Counter
	Errors     *Counter
	InFlight   *Gauge
	DurationSec *Histogram
}

// NewHTTPMetrics registers the standard HTTP metrics against r.
func NewHTTPMetrics(r *Registry) *HTTPMetrics {
	return &HTTPMetrics{
		Requests:   r.NewCounter("cp_http_requests_total", "Total HTTP requests handled."),
		Errors:     r.NewCounter("cp_http_errors_total", "HTTP responses with status >= 500."),
		InFlight:   r.NewGauge("cp_http_inflight", "In-flight HTTP requests."),
		DurationSec: r.NewHistogram("cp_http_request_duration_seconds", "HTTP request duration seconds.", DefaultLatencyBuckets()),
	}
}

// statusRecorder is a minimal ResponseWriter wrapper that captures the status
// code without buffering.
type statusRecorder struct {
	http.ResponseWriter
	code    int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.code = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.code = http.StatusOK
		s.written = true
	}
	return s.ResponseWriter.Write(b)
}

// Middleware returns an http.Handler wrapper that emits RED metrics and, when
// tracer != nil, an outer span per request.
//
// Route label uses whatever the underlying mux resolves via routeFor(r), or
// r.Pattern once dispatched. Cardinality is bounded to declared patterns —
// never the raw path — guarding against unbounded label churn.
func (m *HTTPMetrics) Middleware(tracer *Tracer, next http.Handler) http.Handler {
	return m.MiddlewareRoute(tracer, next, nil)
}

// RouteResolver returns the route label for a request, e.g. a Go 1.22 mux's
// pattern. Returning "" means the caller couldn't classify the request.
type RouteResolver func(*http.Request) string

// MiddlewareRoute is Middleware with an explicit route resolver — useful when
// the mux is passed in so we can query it (mux.Handler(r) exposes the
// matched pattern before dispatch).
func (m *HTTPMetrics) MiddlewareRoute(tracer *Tracer, next http.Handler, resolve RouteResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := ""
		if resolve != nil {
			route = resolve(r)
		}
		if route == "" {
			route = r.Pattern
		}
		if route == "" {
			route = "unknown"
		}
		method := r.Method
		start := time.Now()
		m.InFlight.Inc("route", route)
		defer m.InFlight.Dec("route", route)

		var span *Span
		ctx := r.Context()
		if tracer != nil {
			ctx, span = tracer.Start(ctx, "http.request")
			span.SetAttr("http.method", method)
			span.SetAttr("http.route", route)
			span.SetAttr("http.target", r.URL.Path)
			r = r.WithContext(ctx)
		}

		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)

		// If the route was unknown at intake but the downstream mux populated
		// r.Pattern during dispatch, promote it now.
		if route == "unknown" && r.Pattern != "" {
			route = r.Pattern
		}

		dur := time.Since(start)
		status := strconv.Itoa(rec.code)
		m.Requests.Inc("route", route, "method", method, "status", status)
		if rec.code >= 500 {
			m.Errors.Inc("route", route, "method", method, "status", status)
		}
		m.DurationSec.ObserveDuration(dur, "route", route, "method", method)

		if span != nil {
			span.SetAttr("http.status_code", rec.code)
			span.SetAttr("http.route", route)
			if rec.code >= 500 {
				span.SetStatus("ERROR")
			}
			span.End()
		}
	})
}

// MuxRouteResolver returns a RouteResolver backed by a *http.ServeMux. It
// queries mux.Handler(r) which returns the matched pattern before dispatch,
// giving the middleware a stable route label even at ingress.
func MuxRouteResolver(mux *http.ServeMux) RouteResolver {
	return func(r *http.Request) string {
		if mux == nil {
			return ""
		}
		_, pattern := mux.Handler(r)
		return pattern
	}
}

// Handler returns an http.Handler that serves the Prometheus exposition
// format for r.
func Handler(r *Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := r.WritePrometheus(w); err != nil {
			// nothing to do — headers already sent
			_ = err
		}
	})
}

// StartSpan is a small helper for non-HTTP code paths that want to emit a
// span (e.g. inside a background worker). Returns a no-op function when
// tracer is nil.
func StartSpan(ctx context.Context, tracer *Tracer, name string) (context.Context, func()) {
	if tracer == nil {
		return ctx, func() {}
	}
	ctx, s := tracer.Start(ctx, name)
	return ctx, s.End
}
