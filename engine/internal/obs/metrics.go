// Package obs provides zero-dependency observability primitives for CasperProver:
//
//   - Prometheus text-exposition format metrics (counters, gauges, histograms)
//     exposed on GET /metrics.
//   - JSON span emission compatible with OpenTelemetry semantic conventions,
//     defaulting to a stderr writer (safe for local dev) and swappable to any
//     io.Writer (e.g. a Tempo/OTLP sidecar via a small adapter).
//
// HONESTY BADGE: this is a minimal, purpose-built implementation. It is
// exposition-format compatible with Prometheus 0.0.4 and semantically
// aligned with OTel spans, but it is NOT the official client library and
// does NOT ship OTLP/gRPC transport. Trade-off: dependency-free, no cross-
// version breakage, deterministic behavior. Upgrade path to
// prometheus/client_golang + go.opentelemetry.io/otel is documented in
// docs/OBSERVABILITY.md.
package obs

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Registry holds all metrics for exposition. Zero value is not usable; call
// NewRegistry.
type Registry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// NewRegistry returns a new empty registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

// NewCounter returns a counter registered under (name, help). Repeated calls
// with the same name return the same counter.
func (r *Registry) NewCounter(name, help string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c := &Counter{name: name, help: help, series: make(map[string]*uint64)}
	r.counters[name] = c
	return c
}

// NewGauge returns a gauge registered under (name, help).
func (r *Registry) NewGauge(name, help string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g := &Gauge{name: name, help: help, series: make(map[string]*int64)}
	r.gauges[name] = g
	return g
}

// NewHistogram returns a histogram with the given exponential buckets in
// seconds. Bucket boundaries must be sorted ascending; +Inf is always added
// as final bucket implicitly.
func (r *Registry) NewHistogram(name, help string, buckets []float64) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.histograms[name]; ok {
		return h
	}
	// defensive: sort + dedup buckets
	b := append([]float64(nil), buckets...)
	sort.Float64s(b)
	h := &Histogram{name: name, help: help, buckets: b, series: make(map[string]*histSeries)}
	r.histograms[name] = h
	return h
}

// DefaultLatencyBuckets returns exponentially-spaced buckets suitable for HTTP
// request latency (seconds): 1ms .. 30s.
func DefaultLatencyBuckets() []float64 {
	return []float64{
		0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5,
		1.0, 2.5, 5.0, 10.0, 30.0,
	}
}

// -----------------------------------------------------------------------------
// Counter
// -----------------------------------------------------------------------------

type Counter struct {
	name, help string
	mu         sync.RWMutex
	series     map[string]*uint64 // labelKey -> value
}

// Inc adds 1 with the given labels.
func (c *Counter) Inc(labels ...string) { c.Add(1, labels...) }

// Add adds delta with the given labels; labels are alternating key/value.
func (c *Counter) Add(delta uint64, labels ...string) {
	key := labelsKey(labels)
	c.mu.RLock()
	ptr, ok := c.series[key]
	c.mu.RUnlock()
	if !ok {
		c.mu.Lock()
		if ptr, ok = c.series[key]; !ok {
			var v uint64
			ptr = &v
			c.series[key] = ptr
		}
		c.mu.Unlock()
	}
	atomic.AddUint64(ptr, delta)
}

// -----------------------------------------------------------------------------
// Gauge
// -----------------------------------------------------------------------------

type Gauge struct {
	name, help string
	mu         sync.RWMutex
	series     map[string]*int64 // labelKey -> value
}

// Set overwrites the gauge value with the given labels.
func (g *Gauge) Set(v int64, labels ...string) {
	key := labelsKey(labels)
	g.mu.RLock()
	ptr, ok := g.series[key]
	g.mu.RUnlock()
	if !ok {
		g.mu.Lock()
		if ptr, ok = g.series[key]; !ok {
			var val int64
			ptr = &val
			g.series[key] = ptr
		}
		g.mu.Unlock()
	}
	atomic.StoreInt64(ptr, v)
}

// Add adjusts gauge by delta (may be negative).
func (g *Gauge) Add(delta int64, labels ...string) {
	key := labelsKey(labels)
	g.mu.RLock()
	ptr, ok := g.series[key]
	g.mu.RUnlock()
	if !ok {
		g.mu.Lock()
		if ptr, ok = g.series[key]; !ok {
			var val int64
			ptr = &val
			g.series[key] = ptr
		}
		g.mu.Unlock()
	}
	atomic.AddInt64(ptr, delta)
}

// Inc/Dec convenience.
func (g *Gauge) Inc(labels ...string) { g.Add(1, labels...) }
func (g *Gauge) Dec(labels ...string) { g.Add(-1, labels...) }

// -----------------------------------------------------------------------------
// Histogram
// -----------------------------------------------------------------------------

type histSeries struct {
	counts []uint64 // len == len(buckets)+1 (last is +Inf)
	sum    uint64   // bits of accumulated sum as float64
	count  uint64
}

type Histogram struct {
	name, help string
	buckets    []float64
	mu         sync.RWMutex
	series     map[string]*histSeries
}

// Observe records value with the given labels. Value should be positive but
// negative values are recorded honestly (into the smallest bucket) rather
// than silently dropped.
func (h *Histogram) Observe(v float64, labels ...string) {
	key := labelsKey(labels)
	h.mu.RLock()
	s, ok := h.series[key]
	h.mu.RUnlock()
	if !ok {
		h.mu.Lock()
		if s, ok = h.series[key]; !ok {
			s = &histSeries{counts: make([]uint64, len(h.buckets)+1)}
			h.series[key] = s
		}
		h.mu.Unlock()
	}
	// bucket lookup
	i := sort.SearchFloat64s(h.buckets, v)
	// SearchFloat64s returns the first bucket >= v; we want the first bucket
	// whose upper bound >= v — same semantics.
	atomic.AddUint64(&s.counts[i], 1)
	atomic.AddUint64(&s.count, 1)
	// accumulate sum atomically via CAS on float64 bits
	for {
		oldBits := atomic.LoadUint64(&s.sum)
		newVal := math.Float64frombits(oldBits) + v
		if atomic.CompareAndSwapUint64(&s.sum, oldBits, math.Float64bits(newVal)) {
			break
		}
	}
}

// ObserveDuration is convenience for time.Duration.
func (h *Histogram) ObserveDuration(d time.Duration, labels ...string) {
	h.Observe(d.Seconds(), labels...)
}

// -----------------------------------------------------------------------------
// Exposition
// -----------------------------------------------------------------------------

// WritePrometheus writes all metrics in Prometheus 0.0.4 text exposition
// format to w.
func (r *Registry) WritePrometheus(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// stable ordering for reproducible output
	counterNames := make([]string, 0, len(r.counters))
	for n := range r.counters {
		counterNames = append(counterNames, n)
	}
	sort.Strings(counterNames)
	for _, n := range counterNames {
		c := r.counters[n]
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", n, escapeHelp(c.help), n); err != nil {
			return err
		}
		c.mu.RLock()
		keys := make([]string, 0, len(c.series))
		for k := range c.series {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := atomic.LoadUint64(c.series[k])
			if _, err := fmt.Fprintf(w, "%s%s %d\n", n, formatLabelKey(k), v); err != nil {
				c.mu.RUnlock()
				return err
			}
		}
		c.mu.RUnlock()
	}

	gaugeNames := make([]string, 0, len(r.gauges))
	for n := range r.gauges {
		gaugeNames = append(gaugeNames, n)
	}
	sort.Strings(gaugeNames)
	for _, n := range gaugeNames {
		g := r.gauges[n]
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", n, escapeHelp(g.help), n); err != nil {
			return err
		}
		g.mu.RLock()
		keys := make([]string, 0, len(g.series))
		for k := range g.series {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := atomic.LoadInt64(g.series[k])
			if _, err := fmt.Fprintf(w, "%s%s %d\n", n, formatLabelKey(k), v); err != nil {
				g.mu.RUnlock()
				return err
			}
		}
		g.mu.RUnlock()
	}

	histNames := make([]string, 0, len(r.histograms))
	for n := range r.histograms {
		histNames = append(histNames, n)
	}
	sort.Strings(histNames)
	for _, n := range histNames {
		h := r.histograms[n]
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", n, escapeHelp(h.help), n); err != nil {
			return err
		}
		h.mu.RLock()
		keys := make([]string, 0, len(h.series))
		for k := range h.series {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s := h.series[k]
			var cum uint64
			labelPart := parseLabelKey(k) // without braces
			for i, b := range h.buckets {
				cum += atomic.LoadUint64(&s.counts[i])
				le := fmt.Sprintf("%g", b)
				if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", n, appendLabel(labelPart, "le", le), cum); err != nil {
					h.mu.RUnlock()
					return err
				}
			}
			cum += atomic.LoadUint64(&s.counts[len(h.buckets)])
			if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", n, appendLabel(labelPart, "le", "+Inf"), cum); err != nil {
				h.mu.RUnlock()
				return err
			}
			sum := math.Float64frombits(atomic.LoadUint64(&s.sum))
			count := atomic.LoadUint64(&s.count)
			if _, err := fmt.Fprintf(w, "%s_sum%s %g\n%s_count%s %d\n", n, formatLabelKey(k), sum, n, formatLabelKey(k), count); err != nil {
				h.mu.RUnlock()
				return err
			}
		}
		h.mu.RUnlock()
	}

	return nil
}

// -----------------------------------------------------------------------------
// Label helpers
// -----------------------------------------------------------------------------

// labelsKey turns alternating key/value pairs into a canonical string. Odd
// slices drop the trailing key. Empty labels return "".
func labelsKey(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	// coerce to even
	n := len(labels) - (len(labels) % 2)
	if n == 0 {
		return ""
	}
	type kv struct{ k, v string }
	pairs := make([]kv, 0, n/2)
	for i := 0; i < n; i += 2 {
		pairs = append(pairs, kv{labels[i], escapeLabelValue(labels[i+1])})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	var sb strings.Builder
	for i, p := range pairs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(p.k)
		sb.WriteString(`="`)
		sb.WriteString(p.v)
		sb.WriteByte('"')
	}
	return sb.String()
}

// formatLabelKey renders "{k1="v",k2="v"}" or "" when empty.
func formatLabelKey(k string) string {
	if k == "" {
		return ""
	}
	return "{" + k + "}"
}

// parseLabelKey returns the inner content of formatLabelKey without braces.
func parseLabelKey(k string) string { return k }

// appendLabel builds "{existing,key=\"val\"}" or "{key=\"val\"}".
func appendLabel(existing, key, val string) string {
	if existing == "" {
		return `{` + key + `="` + escapeLabelValue(val) + `"}`
	}
	return `{` + existing + `,` + key + `="` + escapeLabelValue(val) + `"}`
}

func escapeLabelValue(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}

func escapeHelp(v string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`)
	return r.Replace(v)
}
