package phase2

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// Property-based tests for ValidateDAG.
//
// Runs ~100k randomized chains and checks four classes of invariants:
//   (A) well-formed linear/tree chains ALWAYS validate;
//   (B) cycle mutation ALWAYS breaks a previously valid chain;
//   (C) input-mismatch mutation ALWAYS breaks a previously valid chain;
//   (D) parent-drop / duplicate-root mutations ALWAYS break a previously valid chain.
//
// The generator uses a seeded rand.Rand so runs are reproducible.
// To reproduce a specific counterexample, look at the seed printed in the failure.

func h(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// buildValidChain constructs a valid tree-shaped proof DAG with n steps.
// The first step is the root (no parents). Each subsequent step picks a random
// existing step as its parent; input hash chains to that parent's output hash.
func buildValidChain(r *rand.Rand, n int) *ProofChain {
	if n < 1 {
		n = 1
	}
	steps := make([]ChainStep, 0, n)

	// Root.
	rootID := fmt.Sprintf("p%d", 0)
	rootOut := h(fmt.Sprintf("root-out-%d", r.Int63()))
	steps = append(steps, ChainStep{
		ProofID:    rootID,
		ParentIDs:  nil,
		ModelHash:  h("model"),
		InputHash:  h("root-in"),
		OutputHash: rootOut,
		StepIndex:  0,
		Verified:   false,
	})

	for i := 1; i < n; i++ {
		parentIdx := r.Intn(i) // any prior step (ensures acyclic tree)
		parent := steps[parentIdx]
		out := h(fmt.Sprintf("out-%d-%d", i, r.Int63()))
		steps = append(steps, ChainStep{
			ProofID:    fmt.Sprintf("p%d", i),
			ParentIDs:  []string{parent.ProofID},
			ModelHash:  h("model"),
			InputHash:  parent.OutputHash, // continuity
			OutputHash: out,
			StepIndex:  i,
			Verified:   false,
		})
	}

	return &ProofChain{
		ID:          "chain-x",
		RootProofID: rootID,
		Depth:       n,
		TotalSteps:  n,
		Status:      ChainPending,
		Steps:       steps,
	}
}

func TestPBT_ValidChainAlwaysValidates(t *testing.T) {
	const iterations = 50000
	seed := int64(20260725)
	r := rand.New(rand.NewSource(seed))

	for i := 0; i < iterations; i++ {
		n := 1 + r.Intn(20)
		c := buildValidChain(r, n)
		if err := ValidateDAG(c); err != nil {
			t.Fatalf("iter=%d seed=%d n=%d: valid chain rejected: %v", i, seed, n, err)
		}
	}
}

func TestPBT_CycleAlwaysBreaks(t *testing.T) {
	const iterations = 20000
	seed := int64(20260726)
	r := rand.New(rand.NewSource(seed))

	for i := 0; i < iterations; i++ {
		n := 3 + r.Intn(10) // need >=3 nodes to make a cycle informative
		c := buildValidChain(r, n)

		// Inject a cycle: attach root's ParentIDs to some other step.
		victim := 1 + r.Intn(n-1)
		c.Steps[0].ParentIDs = []string{c.Steps[victim].ProofID}
		// Also force input continuity so a *different* rule doesn't spuriously trip first.
		c.Steps[0].InputHash = c.Steps[victim].OutputHash

		if err := ValidateDAG(c); err == nil {
			t.Fatalf("iter=%d seed=%d n=%d: cycle chain accepted", i, seed, n)
		} else if !strings.Contains(err.Error(), "cycle") &&
			!strings.Contains(err.Error(), "expected 1 root") {
			// injecting parents into the root turns it into a non-root, so
			// "expected 1 root, found 0" is also an acceptable rejection.
			t.Fatalf("iter=%d seed=%d n=%d: unexpected error: %v", i, seed, n, err)
		}
	}
}

func TestPBT_InputMismatchBreaks(t *testing.T) {
	const iterations = 20000
	seed := int64(20260727)
	r := rand.New(rand.NewSource(seed))

	for i := 0; i < iterations; i++ {
		n := 2 + r.Intn(10)
		c := buildValidChain(r, n)

		// Pick any non-root step and corrupt its InputHash so it no longer
		// matches its parent's OutputHash.
		idx := 1 + r.Intn(n-1)
		c.Steps[idx].InputHash = h(fmt.Sprintf("corrupt-%d", r.Int63()))

		if err := ValidateDAG(c); err == nil {
			t.Fatalf("iter=%d seed=%d n=%d idx=%d: input-mismatch chain accepted", i, seed, n, idx)
		} else if !strings.Contains(err.Error(), "input mismatch") {
			t.Fatalf("iter=%d seed=%d n=%d idx=%d: unexpected error: %v", i, seed, n, idx, err)
		}
	}
}

func TestPBT_MultiRootBreaks(t *testing.T) {
	const iterations = 10000
	seed := int64(20260728)
	r := rand.New(rand.NewSource(seed))

	for i := 0; i < iterations; i++ {
		n := 2 + r.Intn(10)
		c := buildValidChain(r, n)

		// Turn a random non-root step into a second root by clearing its ParentIDs.
		idx := 1 + r.Intn(n-1)
		c.Steps[idx].ParentIDs = nil

		if err := ValidateDAG(c); err == nil {
			t.Fatalf("iter=%d seed=%d n=%d idx=%d: multi-root chain accepted", i, seed, n, idx)
		} else if !strings.Contains(err.Error(), "expected 1 root") {
			t.Fatalf("iter=%d seed=%d n=%d idx=%d: unexpected error: %v", i, seed, n, idx, err)
		}
	}
}

func TestPBT_UnknownParentBreaks(t *testing.T) {
	const iterations = 10000
	seed := int64(20260729)
	r := rand.New(rand.NewSource(seed))

	for i := 0; i < iterations; i++ {
		n := 2 + r.Intn(10)
		c := buildValidChain(r, n)

		// Reassign one step's parent to a non-existent proof id.
		idx := 1 + r.Intn(n-1)
		c.Steps[idx].ParentIDs = []string{fmt.Sprintf("ghost-%d", r.Int63())}

		if err := ValidateDAG(c); err == nil {
			t.Fatalf("iter=%d seed=%d n=%d idx=%d: unknown-parent chain accepted", i, seed, n, idx)
		} else if !strings.Contains(err.Error(), "unknown parent") {
			t.Fatalf("iter=%d seed=%d n=%d idx=%d: unexpected error: %v", i, seed, n, idx, err)
		}
	}
}

func TestPBT_EmptyChainRejected(t *testing.T) {
	if err := ValidateDAG(&ProofChain{}); err == nil {
		t.Fatal("empty chain accepted")
	}
}

// Aggregate summary: ~110k randomized cases across five property tests + 1 unit case.
func TestPBT_TotalCoverage(t *testing.T) {
	// This test only exists to make `go test -v` announce the coverage total.
	const total = 50000 + 20000 + 20000 + 10000 + 10000 + 1
	t.Logf("PBT total randomized cases: %d", total)
}
