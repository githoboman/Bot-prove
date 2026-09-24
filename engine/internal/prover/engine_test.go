package prover

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ============================================================================
// GenerateWithKey
// ============================================================================

func TestGenerateWithKey(t *testing.T) {
	eng := New()
	p := eng.GenerateWithKey("agent-1", "pubkey123", []byte("in"), []byte("out"), []byte("m"), "kyc", "anchored")
	if p.PubKey != "pubkey123" {
		t.Fatalf("want pubkey123, got %s", p.PubKey)
	}
	if p.Mode != "anchored" {
		t.Fatalf("want anchored, got %s", p.Mode)
	}
}

func TestGenerateDefaultMode(t *testing.T) {
	eng := New()
	p := eng.Generate("a", []byte("i"), []byte("o"), []byte("m"), "test")
	if p.Mode != "local" {
		t.Fatalf("want local, got %s", p.Mode)
	}
}

func TestGenerateWithEmptyMode(t *testing.T) {
	eng := New()
	p := eng.GenerateWithKey("a", "", []byte("i"), []byte("o"), []byte("m"), "test", "")
	if p.Mode != "local" {
		t.Fatalf("empty mode should default to local, got %s", p.Mode)
	}
}

// ============================================================================
// ListFiltered (pagination & filters)
// ============================================================================

func TestListFilteredByAgent(t *testing.T) {
	eng := New()
	eng.Generate("alice", []byte("1"), []byte("2"), []byte("3"), "uc")
	eng.Generate("bob", []byte("4"), []byte("5"), []byte("6"), "uc")
	eng.Generate("alice", []byte("7"), []byte("8"), []byte("9"), "uc")

	proofs, total := eng.ListFiltered(ListFilter{Agent: "alice"})
	if total != 2 {
		t.Fatalf("want 2 proofs for alice, got %d", total)
	}
	if len(proofs) != 2 {
		t.Fatalf("want 2 returned, got %d", len(proofs))
	}
}

func TestListFilteredByPubKey(t *testing.T) {
	eng := New()
	eng.GenerateWithKey("a", "key1", []byte("1"), []byte("2"), []byte("3"), "uc", "local")
	eng.GenerateWithKey("b", "key2", []byte("4"), []byte("5"), []byte("6"), "uc", "local")
	eng.GenerateWithKey("c", "key1", []byte("7"), []byte("8"), []byte("9"), "uc", "local")

	proofs, total := eng.ListFiltered(ListFilter{PubKey: "key1"})
	if total != 2 {
		t.Fatalf("want 2 proofs for key1, got %d", total)
	}
	if len(proofs) != 2 {
		t.Fatalf("want 2 returned, got %d", len(proofs))
	}
}

func TestListFilteredByMode(t *testing.T) {
	eng := New()
	eng.GenerateWithKey("a", "", []byte("1"), []byte("2"), []byte("3"), "uc", "local")
	eng.GenerateWithKey("b", "", []byte("4"), []byte("5"), []byte("6"), "uc", "anchored")
	eng.GenerateWithKey("c", "", []byte("7"), []byte("8"), []byte("9"), "uc", "anchored")

	proofs, total := eng.ListFiltered(ListFilter{Mode: "anchored"})
	if total != 2 {
		t.Fatalf("want 2 anchored proofs, got %d", total)
	}
	_ = proofs
}

func TestListFilteredPagination(t *testing.T) {
	eng := New()
	for i := 0; i < 10; i++ {
		eng.Generate("agent", []byte("i"), []byte("o"), []byte("m"), "uc")
	}

	// Page 1, limit 3
	page1, total := eng.ListFiltered(ListFilter{Page: 1, Limit: 3})
	if total != 10 {
		t.Fatalf("want total 10, got %d", total)
	}
	if len(page1) != 3 {
		t.Fatalf("want 3 on page 1, got %d", len(page1))
	}

	// Page 4 (items 10-12 → only item 10)
	page4, _ := eng.ListFiltered(ListFilter{Page: 4, Limit: 3})
	if len(page4) != 1 {
		t.Fatalf("want 1 on page 4, got %d", len(page4))
	}

	// Beyond last page
	page99, _ := eng.ListFiltered(ListFilter{Page: 99, Limit: 3})
	if len(page99) != 0 {
		t.Fatalf("want 0 beyond last page, got %d", len(page99))
	}
}

func TestListFilteredDefaultLimit(t *testing.T) {
	eng := New()
	for i := 0; i < 60; i++ {
		eng.Generate("a", []byte("i"), []byte("o"), []byte("m"), "uc")
	}

	proofs, total := eng.ListFiltered(ListFilter{}) // limit=0 → defaults to 50
	if total != 60 {
		t.Fatalf("want total 60, got %d", total)
	}
	if len(proofs) != 50 {
		t.Fatalf("default limit should be 50, got %d", len(proofs))
	}
}

func TestListFilteredCombinedFilters(t *testing.T) {
	eng := New()
	eng.GenerateWithKey("alice", "k1", []byte("1"), []byte("2"), []byte("3"), "uc", "anchored")
	eng.GenerateWithKey("alice", "k1", []byte("4"), []byte("5"), []byte("6"), "uc", "local")
	eng.GenerateWithKey("bob", "k1", []byte("7"), []byte("8"), []byte("9"), "uc", "anchored")

	proofs, total := eng.ListFiltered(ListFilter{Agent: "alice", Mode: "anchored"})
	if total != 1 {
		t.Fatalf("want 1 proof (alice+anchored), got %d", total)
	}
	_ = proofs
}

// ============================================================================
// GetStats
// ============================================================================

func TestGetStatsEmpty(t *testing.T) {
	eng := New()
	s := eng.GetStats()
	if s.Total != 0 || s.Valid != 0 || s.Agents != 0 {
		t.Fatal("empty engine should have zero stats")
	}
}

func TestGetStatsPopulated(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "kyc")
	eng.Generate("b", []byte("4"), []byte("5"), []byte("6"), "loan")
	eng.Generate("a", []byte("7"), []byte("8"), []byte("9"), "kyc")
	_ = eng.Revoke("P-2", "test")

	s := eng.GetStats()
	if s.Total != 3 {
		t.Fatalf("want 3 total, got %d", s.Total)
	}
	if s.Valid != 2 {
		t.Fatalf("want 2 valid, got %d", s.Valid)
	}
	if s.Revoked != 1 {
		t.Fatalf("want 1 revoked, got %d", s.Revoked)
	}
	if s.Agents != 2 {
		t.Fatalf("want 2 agents, got %d", s.Agents)
	}
	if s.UseCases["kyc"] != 2 {
		t.Fatalf("want 2 kyc, got %d", s.UseCases["kyc"])
	}
	if s.UseCases["loan"] != 1 {
		t.Fatalf("want 1 loan, got %d", s.UseCases["loan"])
	}
	if s.AvgGenMs <= 0 {
		t.Fatal("avg gen ms should be > 0")
	}
}

// ============================================================================
// Restore
// ============================================================================

func TestRestore(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")

	// Restore a proof with higher sequence
	restored := &Proof{
		ID:      "P-100",
		Agent:   "restored-agent",
		PH:      "aabb",
		Valid:   true,
		UseCase: "restored",
		Mode:    "anchored",
	}
	eng.Restore(restored)

	p, ok := eng.Get("P-100")
	if !ok || p.Agent != "restored-agent" {
		t.Fatal("restored proof not found")
	}

	// Sequence should have been bumped to 100
	newP := eng.Generate("b", []byte("x"), []byte("y"), []byte("z"), "uc")
	if newP.ID != "P-101" {
		t.Fatalf("want P-101 after restore, got %s", newP.ID)
	}
}

func TestRestoreDoesNotAffectOriginal(t *testing.T) {
	eng := New()
	orig := &Proof{ID: "P-50", Agent: "orig", Valid: true}
	eng.Restore(orig)

	// Modify the original pointer
	orig.Agent = "modified"

	// Should not affect the engine's copy
	p, _ := eng.Get("P-50")
	if p.Agent != "orig" {
		t.Fatal("restore should copy, not share pointer")
	}
}

// ============================================================================
// SeedDemoData
// ============================================================================

func TestSeedDemoData(t *testing.T) {
	eng := New()
	eng.SeedDemoData()

	all := eng.List()
	if len(all) < 5 {
		t.Fatalf("want at least 5 demo proofs, got %d", len(all))
	}

	// All should be valid and have deploy hashes
	for _, p := range all {
		if !p.Valid {
			t.Fatalf("demo proof %s should be valid", p.ID)
		}
		if p.Deploy == "" {
			t.Fatalf("demo proof %s should have deploy hash", p.ID)
		}
	}
}

func TestSeedDemoDataUseCases(t *testing.T) {
	eng := New()
	eng.SeedDemoData()

	s := eng.GetStats()
	if len(s.UseCases) < 3 {
		t.Fatalf("want at least 3 use cases, got %d", len(s.UseCases))
	}
}

// ============================================================================
// ProofBundle Save/Load
// ============================================================================

func TestSaveAndLoadBundle(t *testing.T) {
	dir := t.TempDir()

	bundle := &ProofBundle{
		Root:  "aabbccdd",
		Leafs: []string{"leaf1", "leaf2"},
		Count: 2,
	}
	if err := SaveBundle(dir, bundle); err != nil {
		t.Fatalf("save: %v", err)
	}

	path := filepath.Join(dir, "aabbccdd.json")
	loaded, err := LoadBundle(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Root != "aabbccdd" {
		t.Fatalf("want root aabbccdd, got %s", loaded.Root)
	}
	if loaded.Count != 2 || len(loaded.Leafs) != 2 {
		t.Fatal("loaded bundle doesn't match")
	}
}

func TestLoadBundleMissing(t *testing.T) {
	_, err := LoadBundle("/nonexistent/path.json")
	if err == nil {
		t.Fatal("should error on missing file")
	}
}

func TestSaveBundleCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	bundle := &ProofBundle{Root: "test123", Leafs: nil, Count: 0}
	if err := SaveBundle(dir, bundle); err != nil {
		t.Fatalf("save should create dirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test123.json")); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}

// ============================================================================
// Concurrency — stress test
// ============================================================================

func TestConcurrentGenerateAndVerify(t *testing.T) {
	eng := New()
	var wg sync.WaitGroup
	n := 50

	// Generate concurrently
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			eng.Generate("agent", []byte("i"), []byte("o"), []byte("m"), "uc")
		}(i)
	}
	wg.Wait()

	// Verify all concurrently
	wg.Add(n)
	for i := 1; i <= n; i++ {
		go func(idx int) {
			defer wg.Done()
			pid := "P-" + string(rune('0'+idx/10)) + string(rune('0'+idx%10))
			// Just exercise the code path; not all IDs will match our format
			_, _ = eng.Verify(pid)
		}(i)
	}
	wg.Wait()

	all := eng.List()
	if len(all) != n {
		t.Fatalf("want %d, got %d", n, len(all))
	}
}

func TestConcurrentRevokeAndList(t *testing.T) {
	eng := New()
	for i := 0; i < 20; i++ {
		eng.Generate("a", []byte("i"), []byte("o"), []byte("m"), "uc")
	}

	var wg sync.WaitGroup
	// Revoke odd IDs while listing
	wg.Add(30)
	for i := 1; i <= 20; i += 2 {
		go func(idx int) {
			defer wg.Done()
			_ = eng.Revoke("P-"+itoa(idx), "concurrent")
		}(i)
	}
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			_ = eng.List()
		}()
	}
	wg.Wait()
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// ============================================================================
// Edge cases
// ============================================================================

func TestGenerateEmptyInputs(t *testing.T) {
	eng := New()
	p := eng.Generate("a", []byte{}, []byte{}, []byte{}, "empty")
	if p.ID == "" {
		t.Fatal("should generate even with empty inputs")
	}
	if len(p.PH) != 64 {
		t.Fatalf("proof hash should be 64 hex chars, got %d", len(p.PH))
	}
}

func TestVerifyAfterRevokeReturnsFalse(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("i"), []byte("o"), []byte("m"), "uc")
	_ = eng.Revoke("P-1", "reason")
	ok, err := eng.Verify("P-1")
	if err != nil {
		t.Fatalf("verify after revoke should not error: %v", err)
	}
	if ok {
		t.Fatal("revoked proof should not verify")
	}
}

func TestListSortedByTimestamp(t *testing.T) {
	eng := New()
	for i := 0; i < 5; i++ {
		eng.Generate("a", []byte("i"), []byte("o"), []byte("m"), "uc")
	}
	all := eng.List()
	for i := 1; i < len(all); i++ {
		if all[i].TS > all[i-1].TS {
			t.Fatal("list should be sorted by timestamp descending")
		}
	}
}
