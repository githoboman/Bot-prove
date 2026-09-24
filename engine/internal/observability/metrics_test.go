package observability

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestCounter_IncAndText(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("cp_requests_total", "total requests", "method", "path")
	c.Inc("GET", "/health")
	c.Inc("GET", "/health")
	c.Add(3, "POST", "/proofs")

	var buf bytes.Buffer
	if err := r.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	got := buf.String()
	want := []string{
		"# HELP cp_requests_total total requests",
		"# TYPE cp_requests_total counter",
		`cp_requests_total{method="GET",path="/health"} 2`,
		`cp_requests_total{method="POST",path="/proofs"} 3`,
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
}

func TestGauge_SetAndAdd(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("cp_workers_in_flight", "current worker count")
	g.Set(5)
	g.Add(-2)
	g.Add(1)
	var buf bytes.Buffer
	_ = r.WriteText(&buf)
	if !strings.Contains(buf.String(), "cp_workers_in_flight 4") {
		t.Errorf("expected gauge=4:\n%s", buf.String())
	}
}

func TestHistogram_ObserveAndText(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogram("cp_latency_seconds", "request latency",
		[]float64{0.1, 0.5, 1.0}, "route")
	// 0.05 -> falls into 0.1, 0.5, 1.0, +Inf
	h.Observe(0.05, "/health")
	// 0.4 -> falls into 0.5, 1.0, +Inf (not 0.1)
	h.Observe(0.4, "/health")
	// 2.0 -> falls into +Inf only
	h.Observe(2.0, "/health")

	var buf bytes.Buffer
	if err := r.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	got := buf.String()
	must := []string{
		`cp_latency_seconds_bucket{route="/health",le="0.1"} 1`,
		`cp_latency_seconds_bucket{route="/health",le="0.5"} 2`,
		`cp_latency_seconds_bucket{route="/health",le="1"} 2`,
		`cp_latency_seconds_bucket{route="/health",le="+Inf"} 3`,
		`cp_latency_seconds_count{route="/health"} 3`,
	}
	for _, w := range must {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	// Sum is 0.05+0.4+2.0 = 2.45; be lenient on format.
	if !strings.Contains(got, "cp_latency_seconds_sum") {
		t.Errorf("missing sum line:\n%s", got)
	}
}

func TestCounter_Concurrent(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("hits", "hits")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	var buf bytes.Buffer
	_ = r.WriteText(&buf)
	if !strings.Contains(buf.String(), "hits 100000") {
		t.Errorf("expected hits=100000:\n%s", buf.String())
	}
}

func TestCounter_NegativeAddPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on negative counter Add")
		}
	}()
	r := NewRegistry()
	c := r.NewCounter("x", "x")
	c.Add(-1)
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on duplicate metric name")
		}
	}()
	r := NewRegistry()
	r.NewCounter("dup", "x")
	r.NewCounter("dup", "y")
}

func TestHistogram_BadBucketsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on non-ascending buckets")
		}
	}()
	r := NewRegistry()
	r.NewHistogram("bad", "", []float64{1, 0.5})
}

func TestLabelValueEscaping(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("x", "y", "label")
	c.Inc(`a"b\c` + "\n")
	var buf bytes.Buffer
	_ = r.WriteText(&buf)
	want := `x{label="a\"b\\c\n"} 1`
	if !strings.Contains(buf.String(), want) {
		t.Errorf("bad escape:\n%s", buf.String())
	}
}
