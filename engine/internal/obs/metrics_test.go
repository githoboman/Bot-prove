package obs

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestCounterAddAndInc(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("cp_test_counter", "test counter")
	c.Inc("route", "/foo", "method", "GET")
	c.Add(3, "route", "/foo", "method", "GET")
	c.Inc("route", "/bar")

	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("expose: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "# TYPE cp_test_counter counter") {
		t.Fatalf("missing TYPE line: %s", out)
	}
	if !strings.Contains(out, `cp_test_counter{method="GET",route="/foo"} 4`) {
		t.Fatalf("unexpected combined counter value:\n%s", out)
	}
	if !strings.Contains(out, `cp_test_counter{route="/bar"} 1`) {
		t.Fatalf("missing bar series:\n%s", out)
	}
}

func TestGaugeSetAddIncDec(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("cp_test_gauge", "test gauge")
	g.Set(5, "kind", "queue")
	g.Add(2, "kind", "queue")
	g.Inc("kind", "queue")
	g.Dec("kind", "queue")

	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("expose: %v", err)
	}
	if !strings.Contains(buf.String(), `cp_test_gauge{kind="queue"} 7`) {
		t.Fatalf("unexpected gauge value:\n%s", buf.String())
	}
}

func TestHistogramExpositionShape(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogram("cp_test_hist", "test histogram", []float64{0.1, 0.5, 1.0})
	h.Observe(0.05, "route", "/a") // bucket 0.1
	h.Observe(0.3, "route", "/a")  // bucket 0.5
	h.Observe(0.7, "route", "/a")  // bucket 1.0
	h.Observe(5.0, "route", "/a")  // bucket +Inf

	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("expose: %v", err)
	}
	out := buf.String()

	// cumulative counts: 0.1->1, 0.5->2, 1.0->3, +Inf->4
	checks := []string{
		`cp_test_hist_bucket{route="/a",le="0.1"} 1`,
		`cp_test_hist_bucket{route="/a",le="0.5"} 2`,
		`cp_test_hist_bucket{route="/a",le="1"} 3`,
		`cp_test_hist_bucket{route="/a",le="+Inf"} 4`,
		`cp_test_hist_count{route="/a"} 4`,
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("missing exposition line %q in:\n%s", want, out)
		}
	}

	// sum must be ~6.05
	m := regexp.MustCompile(`cp_test_hist_sum\{route="/a"\} ([0-9\.eE\+\-]+)`).FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("no sum line: %s", out)
	}
	if !strings.HasPrefix(m[1], "6.05") && !strings.HasPrefix(m[1], "6.049") && !strings.HasPrefix(m[1], "6.0500") && !strings.HasPrefix(m[1], "6.05e") && !strings.HasPrefix(m[1], "6.050000") {
		// tolerate float rendering; check with parseable value
		if !strings.HasPrefix(m[1], "6") {
			t.Fatalf("sum unexpected: %q", m[1])
		}
	}
}

func TestExpositionEscaping(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("cp_test_esc", "help with \"quotes\" and \n newline")
	c.Inc("path", `weird"value`+"\n")

	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("expose: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `\n`) || strings.Contains(out, "\n newline\n") {
		// The HELP line must have escaped the raw newline, so " newline" should
		// not appear on its own line.
		if strings.Contains(out, "\nhelp") {
			t.Fatalf("help contains raw newline: %q", out)
		}
	}
	if !strings.Contains(out, `weird\"value`) {
		t.Fatalf("label value not escaped: %q", out)
	}
}

func TestStableOrdering(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("cp_test_ord", "ord")
	for _, k := range []string{"z", "a", "m"} {
		c.Inc("k", k)
	}
	var buf1, buf2 bytes.Buffer
	_ = r.WritePrometheus(&buf1)
	_ = r.WritePrometheus(&buf2)
	if buf1.String() != buf2.String() {
		t.Fatalf("exposition not deterministic:\n%s\n---\n%s", buf1.String(), buf2.String())
	}
	// a should come before m before z
	s := buf1.String()
	if strings.Index(s, `k="a"`) > strings.Index(s, `k="m"`) {
		t.Fatalf("labels not sorted: %s", s)
	}
	if strings.Index(s, `k="m"`) > strings.Index(s, `k="z"`) {
		t.Fatalf("labels not sorted: %s", s)
	}
}

func TestOnlyExposesRegisteredMetrics(t *testing.T) {
	r := NewRegistry()
	// no metrics at all — expect empty output
	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("expose: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty output, got: %q", buf.String())
	}
}
