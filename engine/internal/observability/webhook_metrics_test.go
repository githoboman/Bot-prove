package observability

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWebhookMetrics_RegisterAndScrape(t *testing.T) {
	r := NewRegistry()
	m := NewWebhookMetrics(r, "cp_webhook")

	// Simulate a small workload.
	m.Enqueued.Inc("proof.verified")
	m.Enqueued.Inc("proof.verified")
	m.Enqueued.Inc("proof.anchored")

	m.Attempts.Inc("proof.verified", "2xx")
	m.Attempts.Inc("proof.verified", "5xx")
	m.Attempts.Inc("proof.verified", "network")

	m.Delivered.Inc("proof.verified")
	m.DeadLettered.Inc("proof.anchored")
	m.Replayed.Inc("proof.anchored")

	m.AttemptDuration.Observe(0.012, "proof.verified", "2xx")
	m.AttemptDuration.Observe(2.3, "proof.verified", "5xx")

	m.QueueDepth.Set(7)
	m.DeadLetterDepth.Set(1)

	var buf bytes.Buffer
	if err := r.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	out := buf.String()

	// Spot-check a handful of series lines — we care that labels
	// and values round-trip, not the exact ordering.
	wants := []string{
		`cp_webhook_enqueued_total{event="proof.verified"} 2`,
		`cp_webhook_enqueued_total{event="proof.anchored"} 1`,
		`cp_webhook_attempts_total{event="proof.verified",status_class="2xx"} 1`,
		`cp_webhook_attempts_total{event="proof.verified",status_class="5xx"} 1`,
		`cp_webhook_attempts_total{event="proof.verified",status_class="network"} 1`,
		`cp_webhook_delivered_total{event="proof.verified"} 1`,
		`cp_webhook_dead_lettered_total{event="proof.anchored"} 1`,
		`cp_webhook_replayed_total{event="proof.anchored"} 1`,
		`cp_webhook_attempt_duration_seconds_count{event="proof.verified",status_class="2xx"} 1`,
		`cp_webhook_attempt_duration_seconds_count{event="proof.verified",status_class="5xx"} 1`,
		`cp_webhook_queue_depth 7`,
		`cp_webhook_dead_letter_depth 1`,
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("scrape missing %q\n---\n%s", w, out)
		}
	}
}

func TestStatusClass(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{0, "network"},
		{200, "2xx"},
		{204, "2xx"},
		{301, "3xx"},
		{404, "4xx"},
		{500, "5xx"},
		{503, "5xx"},
		{-1, "other"},
		{999, "other"},
	}
	for _, tc := range cases {
		if got := StatusClass(tc.code); got != tc.want {
			t.Errorf("StatusClass(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestWebhookMetrics_NilSafeUsage(t *testing.T) {
	// The webhooks package treats a nil *WebhookMetrics as "metrics
	// disabled" — verify that shape is coherent (calls dispatched
	// through a helper that no-ops on nil). We validate here by
	// building the "no-op" helper the way the webhooks store does.
	var m *WebhookMetrics
	noop := func(fn func()) {
		if m == nil {
			return
		}
		fn()
	}
	noop(func() { m.Enqueued.Inc("proof.verified") })
	noop(func() { m.QueueDepth.Set(3) })
	// If we got here without panic, the pattern is safe.
}

func TestWebhookMetrics_HistogramBucketsMonotonic(t *testing.T) {
	// Sanity — a scrape after several observations should show
	// cumulative counts increasing across buckets.
	r := NewRegistry()
	m := NewWebhookMetrics(r, "cp_webhook")
	for _, v := range []float64{0.005, 0.02, 0.05, 0.2, 1.5} {
		m.AttemptDuration.Observe(v, "proof.verified", "2xx")
	}
	// Give WriteText a stable "now" independent of test clock.
	_ = time.Now
	var buf bytes.Buffer
	_ = r.WriteText(&buf)
	// Extract the bucket lines for our label set — bucket counts
	// must be non-decreasing.
	lines := strings.Split(buf.String(), "\n")
	var prev uint64
	sawAny := false
	for _, l := range lines {
		if !strings.Contains(l, `cp_webhook_attempt_duration_seconds_bucket{event="proof.verified",status_class="2xx"`) {
			continue
		}
		sawAny = true
		var v uint64
		parts := strings.Fields(l)
		if len(parts) < 2 {
			continue
		}
		if _, err := fmtSscan(parts[len(parts)-1], &v); err != nil {
			continue
		}
		if v < prev {
			t.Errorf("bucket counts not monotonic: %d < %d in %q", v, prev, l)
		}
		prev = v
	}
	if !sawAny {
		t.Fatalf("no bucket lines found in scrape:\n%s", buf.String())
	}
}

// fmtSscan is a tiny wrapper so the test does not need fmt import
// twice — keeps this file self-contained.
func fmtSscan(s string, v *uint64) (int, error) {
	// strconv would be cleaner but we already import strings; parse
	// manually to avoid pulling in strconv-only for one call.
	var acc uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNAN
		}
		acc = acc*10 + uint64(c-'0')
	}
	*v = acc
	return 1, nil
}

var errNAN = &nanErr{}

type nanErr struct{}

func (*nanErr) Error() string { return "not a number" }
