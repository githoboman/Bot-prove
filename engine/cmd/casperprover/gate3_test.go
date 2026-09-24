package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

// TestGate3_ReproducibleReceipt runs `casperprover gate3` twice and asserts the
// two receipts have identical scenario digests. Reproducibility is what makes
// the demo auditable — a judge can diff evidence hashes across machines.
//
// Skipped under `go test -short` because it shells out to `go run`.
func TestGate3_ReproducibleReceipt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shell-out test in -short mode")
	}
	run := func() map[string]any {
		var out bytes.Buffer
		cmd := exec.Command("go", "run", ".", "gate3")
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("gate3 run failed: %v", err)
		}
		var v map[string]any
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatalf("gate3 output was not valid JSON: %v", err)
		}
		return v
	}
	r1, r2 := run(), run()
	if r1["receipt_digest"] != r2["receipt_digest"] {
		t.Fatalf("receipt digest not stable: r1=%v r2=%v", r1["receipt_digest"], r2["receipt_digest"])
	}
	if r1["receipt_digest"] == "" {
		t.Fatal("receipt digest empty")
	}
	if all, _ := r1["all_passed"].(bool); !all {
		t.Fatalf("gate3 did not all-pass: %+v", r1)
	}
}
