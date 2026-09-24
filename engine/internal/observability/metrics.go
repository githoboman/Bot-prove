// Package observability provides an in-house Prometheus text-format
// exporter and W3C traceparent context propagation for the engine.
//
// Design goals:
//   - Zero new module dependencies. The Prometheus wire protocol is a
//     simple text format we emit ourselves; W3C Trace Context is just
//     header parse+generate. Both are stable, versioned specs.
//   - Concurrent-safe on the hot path. Counters/gauges/histograms use
//     atomic ops or a single RWMutex depending on the shape.
//   - Cardinality-safe. Label sets are canonicalised into a fixed
//     string key; the caller is expected to keep labels bounded (we
//     do not enforce a cap here — that is a code-review discipline).
//
// This is real Prometheus (a running prometheus-server can scrape
// /metrics and store the results). It is NOT the reference
// `prometheus/client_golang` library — features like summaries,
// exemplars and native histograms are out of scope.
package observability

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Registry is the central metric store. Cheap to construct; safe for
// concurrent use.
type Registry struct {
	mu     sync.RWMutex
	metric map[string]metric // keyed by metric name
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{metric: make(map[string]metric)}
}

// Counter, Gauge, Histogram are the three metric kinds we support.
// Each metric is name-scoped and label-partitioned.

// Counter is a monotonically-increasing float. Zero-safe on init.
type Counter struct {
	reg      *Registry
	name     string
	help     string
	labelset []string // ordered label names
	mu       sync.RWMutex
	series   map[string]*counterSeries // keyed by canonical label string
}

type counterSeries struct {
	labels []labelKV
	val    uint64 // stored as bit-cast float64 for atomic increment on non-integer deltas
}

// Gauge is a bidirectional float.
type Gauge struct {
	reg      *Registry
	name     string
	help     string
	labelset []string
	mu       sync.RWMutex
	series   map[string]*gaugeSeries
}

type gaugeSeries struct {
	labels []labelKV
	val    uint64 // bit-cast float64
}

// Histogram uses cumulative buckets.
type Histogram struct {
	reg      *Registry
	name     string
	help     string
	labelset []string
	buckets  []float64
	mu       sync.RWMutex
	series   map[string]*histogramSeries
}

type histogramSeries struct {
	labels  []labelKV
	counts  []uint64  // per-bucket cumulative counts; len == len(buckets)+1 for +Inf
	sumBits uint64    // bit-cast float64 total
	total   uint64    // observation count
}

type labelKV struct {
	name, val string
}

type metric interface {
	writeTo(io.Writer) error
}

// NewCounter registers a counter under name.
func (r *Registry) NewCounter(name, help string, labels ...string) *Counter {
	c := &Counter{
		reg:      r,
		name:     name,
		help:     help,
		labelset: append([]string(nil), labels...),
		series:   make(map[string]*counterSeries),
	}
	r.register(name, c)
	return c
}

// NewGauge registers a gauge.
func (r *Registry) NewGauge(name, help string, labels ...string) *Gauge {
	g := &Gauge{
		reg:      r,
		name:     name,
		help:     help,
		labelset: append([]string(nil), labels...),
		series:   make(map[string]*gaugeSeries),
	}
	r.register(name, g)
	return g
}

// DefaultBuckets are HTTP-latency-oriented, in seconds.
var DefaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// NewHistogram registers a histogram. If buckets is nil, DefaultBuckets
// is used. Buckets must be sorted ascending; violations panic (a bug in
// the caller, caught at init time).
func (r *Registry) NewHistogram(name, help string, buckets []float64, labels ...string) *Histogram {
	if buckets == nil {
		buckets = DefaultBuckets
	}
	// Validate ordering.
	for i := 1; i < len(buckets); i++ {
		if buckets[i] <= buckets[i-1] {
			panic(fmt.Sprintf("observability: histogram %s buckets not strictly ascending at index %d", name, i))
		}
	}
	h := &Histogram{
		reg:      r,
		name:     name,
		help:     help,
		labelset: append([]string(nil), labels...),
		buckets:  append([]float64(nil), buckets...),
		series:   make(map[string]*histogramSeries),
	}
	r.register(name, h)
	return h
}

func (r *Registry) register(name string, m metric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.metric[name]; dup {
		panic(fmt.Sprintf("observability: metric %q already registered", name))
	}
	r.metric[name] = m
}

// --- Counter ---

// Inc adds 1 to the counter under the given label values (in the order
// declared to NewCounter). Panics if the number of values does not
// match the label set — a caller bug.
func (c *Counter) Inc(labelValues ...string) {
	c.Add(1, labelValues...)
}

// Add adds delta to the counter. Negative delta panics (a counter must
// be monotonic).
func (c *Counter) Add(delta float64, labelValues ...string) {
	if delta < 0 {
		panic("observability: counter Add negative delta")
	}
	s := c.seriesFor(labelValues)
	addFloatAtomic(&s.val, delta)
}

func (c *Counter) seriesFor(vals []string) *counterSeries {
	if len(vals) != len(c.labelset) {
		panic(fmt.Sprintf("observability: counter %s expected %d labels, got %d",
			c.name, len(c.labelset), len(vals)))
	}
	key := labelKey(c.labelset, vals)
	c.mu.RLock()
	s, ok := c.series[key]
	c.mu.RUnlock()
	if ok {
		return s
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok = c.series[key]; ok {
		return s
	}
	s = &counterSeries{labels: makeLabels(c.labelset, vals)}
	c.series[key] = s
	return s
}

// --- Gauge ---

// Set stores a value.
func (g *Gauge) Set(v float64, labelValues ...string) {
	s := g.seriesFor(labelValues)
	atomic.StoreUint64(&s.val, math.Float64bits(v))
}

// Add adds delta (can be negative).
func (g *Gauge) Add(delta float64, labelValues ...string) {
	s := g.seriesFor(labelValues)
	addFloatAtomic(&s.val, delta)
}

func (g *Gauge) seriesFor(vals []string) *gaugeSeries {
	if len(vals) != len(g.labelset) {
		panic(fmt.Sprintf("observability: gauge %s expected %d labels, got %d",
			g.name, len(g.labelset), len(vals)))
	}
	key := labelKey(g.labelset, vals)
	g.mu.RLock()
	s, ok := g.series[key]
	g.mu.RUnlock()
	if ok {
		return s
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if s, ok = g.series[key]; ok {
		return s
	}
	s = &gaugeSeries{labels: makeLabels(g.labelset, vals)}
	g.series[key] = s
	return s
}

// --- Histogram ---

// Observe records one value.
func (h *Histogram) Observe(v float64, labelValues ...string) {
	s := h.seriesFor(labelValues)
	// Increment matching + all higher buckets (cumulative Prom spec).
	idx := sort.SearchFloat64s(h.buckets, v)
	// idx is the first bucket where h.buckets[idx] >= v.
	// v falls into that bucket AND all higher ones.
	for i := idx; i < len(h.buckets); i++ {
		atomic.AddUint64(&s.counts[i], 1)
	}
	// +Inf bucket (last index).
	atomic.AddUint64(&s.counts[len(h.buckets)], 1)
	atomic.AddUint64(&s.total, 1)
	addFloatAtomic(&s.sumBits, v)
}

func (h *Histogram) seriesFor(vals []string) *histogramSeries {
	if len(vals) != len(h.labelset) {
		panic(fmt.Sprintf("observability: histogram %s expected %d labels, got %d",
			h.name, len(h.labelset), len(vals)))
	}
	key := labelKey(h.labelset, vals)
	h.mu.RLock()
	s, ok := h.series[key]
	h.mu.RUnlock()
	if ok {
		return s
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok = h.series[key]; ok {
		return s
	}
	s = &histogramSeries{
		labels: makeLabels(h.labelset, vals),
		counts: make([]uint64, len(h.buckets)+1),
	}
	h.series[key] = s
	return s
}

// --- Text exposition ---

// WriteText serialises the registry in the Prometheus text exposition
// format (v0.0.4). A prometheus-server can scrape this directly.
func (r *Registry) WriteText(w io.Writer) error {
	r.mu.RLock()
	// Sort metric names for deterministic output (nice for tests).
	names := make([]string, 0, len(r.metric))
	for n := range r.metric {
		names = append(names, n)
	}
	r.mu.RUnlock()
	sort.Strings(names)
	for _, n := range names {
		r.mu.RLock()
		m := r.metric[n]
		r.mu.RUnlock()
		if err := m.writeTo(w); err != nil {
			return err
		}
	}
	return nil
}

func (c *Counter) writeTo(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, escapeHelp(c.help), c.name); err != nil {
		return err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Emit in deterministic label order.
	keys := make([]string, 0, len(c.series))
	for k := range c.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := c.series[k]
		v := math.Float64frombits(atomic.LoadUint64(&s.val))
		if _, err := fmt.Fprintf(w, "%s%s %s\n", c.name, formatLabels(s.labels), formatFloat(v)); err != nil {
			return err
		}
	}
	return nil
}

func (g *Gauge) writeTo(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", g.name, escapeHelp(g.help), g.name); err != nil {
		return err
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	keys := make([]string, 0, len(g.series))
	for k := range g.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := g.series[k]
		v := math.Float64frombits(atomic.LoadUint64(&s.val))
		if _, err := fmt.Fprintf(w, "%s%s %s\n", g.name, formatLabels(s.labels), formatFloat(v)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Histogram) writeTo(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, escapeHelp(h.help), h.name); err != nil {
		return err
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	keys := make([]string, 0, len(h.series))
	for k := range h.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := h.series[k]
		// Per-bucket cumulative counts.
		for i, ub := range h.buckets {
			lbls := append([]labelKV(nil), s.labels...)
			lbls = append(lbls, labelKV{name: "le", val: formatFloat(ub)})
			if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, formatLabels(lbls), atomic.LoadUint64(&s.counts[i])); err != nil {
				return err
			}
		}
		lbls := append([]labelKV(nil), s.labels...)
		lbls = append(lbls, labelKV{name: "le", val: "+Inf"})
		if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, formatLabels(lbls), atomic.LoadUint64(&s.counts[len(h.buckets)])); err != nil {
			return err
		}
		sum := math.Float64frombits(atomic.LoadUint64(&s.sumBits))
		if _, err := fmt.Fprintf(w, "%s_sum%s %s\n", h.name, formatLabels(s.labels), formatFloat(sum)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s_count%s %d\n", h.name, formatLabels(s.labels), atomic.LoadUint64(&s.total)); err != nil {
			return err
		}
	}
	return nil
}

// --- helpers ---

// labelKey produces a stable canonical string for a label-value tuple,
// used as the series map key.
func labelKey(names, values []string) string {
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	for i, n := range names {
		if i > 0 {
			b.WriteByte(0x1e) // ASCII record separator; illegal in label vals
		}
		b.WriteString(n)
		b.WriteByte(0x1f) // unit separator
		b.WriteString(values[i])
	}
	return b.String()
}

func makeLabels(names, values []string) []labelKV {
	out := make([]labelKV, len(names))
	for i, n := range names {
		out[i] = labelKV{name: n, val: values[i]}
	}
	return out
}

func formatLabels(lbls []labelKV) string {
	if len(lbls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, kv := range lbls {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(kv.name)
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(escapeLabelVal(kv.val))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

// escapeLabelVal escapes backslashes, double quotes and newlines per the
// Prometheus text exposition spec.
func escapeLabelVal(s string) string {
	if !strings.ContainsAny(s, "\\\"\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeHelp escapes only backslashes and newlines (help lines don't
// need to escape quotes).
func escapeHelp(s string) string {
	if !strings.ContainsAny(s, "\\\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatFloat(v float64) string {
	// Prometheus wants "1", "1.5", "+Inf", "-Inf", "NaN".
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	}
	// Compact: integers as integers, others with %g.
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

// addFloatAtomic performs an atomic float add over a uint64 word.
func addFloatAtomic(word *uint64, delta float64) {
	for {
		old := atomic.LoadUint64(word)
		newF := math.Float64frombits(old) + delta
		if atomic.CompareAndSwapUint64(word, old, math.Float64bits(newF)) {
			return
		}
	}
}
