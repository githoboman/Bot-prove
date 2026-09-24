package prover

import (
	"sync"
	"testing"
)

func TestGenerate(t *testing.T) {
	eng := New()
	p := eng.Generate("agent-1", []byte("in"), []byte("out"), []byte("m"), "test")
	if p.ID != "P-1" {
		t.Fatalf("want P-1 got %s", p.ID)
	}
	if !p.Valid || p.Revoked {
		t.Fatal("new proof should be valid")
	}
	if p.PH == "" || p.IH == "" || p.OH == "" || p.MH == "" {
		t.Fatal("missing hashes")
	}
}

func TestGet(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")
	p, ok := eng.Get("P-1")
	if !ok || p == nil {
		t.Fatal("proof not found")
	}
}

func TestGetMissing(t *testing.T) {
	eng := New()
	_, ok := eng.Get("P-999")
	if ok {
		t.Fatal("should not find missing")
	}
}

func TestRevoke(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")
	err := eng.Revoke("P-1", "test")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	p, _ := eng.Get("P-1")
	if p.Valid || !p.Revoked {
		t.Fatal("should be revoked")
	}
}

func TestDoubleRevoke(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")
	_ = eng.Revoke("P-1", "r1")
	err := eng.Revoke("P-1", "r2")
	if err == nil {
		t.Fatal("double revoke should fail")
	}
}

func TestVerify(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")
	ok, err := eng.Verify("P-1")
	if err != nil || !ok {
		t.Fatal("should verify")
	}
}

func TestVerifyRevoked(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")
	_ = eng.Revoke("P-1", "r")
	ok, _ := eng.Verify("P-1")
	if ok {
		t.Fatal("revoked should not verify")
	}
}

func TestList(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")
	eng.Generate("b", []byte("4"), []byte("5"), []byte("6"), "uc")
	all := eng.List()
	if len(all) != 2 {
		t.Fatalf("want 2, got %d", len(all))
	}
}

func TestSequentialIDs(t *testing.T) {
	eng := New()
	p1 := eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")
	p2 := eng.Generate("a", []byte("4"), []byte("5"), []byte("6"), "uc")
	if p1.ID == p2.ID {
		t.Fatal("ids should be unique")
	}
}

func TestVerifyMissing(t *testing.T) {
	eng := New()
	_, err := eng.Verify("P-404")
	if err == nil {
		t.Fatal("verify missing should error")
	}
}

func TestRevokeMissing(t *testing.T) {
	eng := New()
	err := eng.Revoke("P-404", "reason")
	if err == nil {
		t.Fatal("revoke missing should error")
	}
}

func TestProofHashes(t *testing.T) {
	eng := New()
	p := eng.Generate("agent", []byte("input"), []byte("output"), []byte("model"), "uc")
	if len(p.PH) != 64 {
		t.Fatalf("PH len %d", len(p.PH))
	}
	if len(p.IH) != 64 {
		t.Fatalf("IH len %d", len(p.IH))
	}
	if len(p.OH) != 64 {
		t.Fatalf("OH len %d", len(p.OH))
	}
	if len(p.MH) != 64 {
		t.Fatalf("MH len %d", len(p.MH))
	}
}

func TestProofRoot(t *testing.T) {
	eng := New()
	p := eng.Generate("a", []byte("x"), []byte("y"), []byte("z"), "test")
	if len(p.Root) != 64 {
		t.Fatalf("root len %d", len(p.Root))
	}
}

func TestProofTimestamp(t *testing.T) {
	eng := New()
	p := eng.Generate("a", []byte("i"), []byte("o"), []byte("m"), "uc")
	if p.TS <= 0 {
		t.Fatal("timestamp should be positive")
	}
}

func TestProofUseCase(t *testing.T) {
	eng := New()
	p := eng.Generate("a", []byte("i"), []byte("o"), []byte("m"), "inference")
	if p.UseCase != "inference" {
		t.Fatalf("want inference, got %s", p.UseCase)
	}
}

func TestConcurrentGenerate(t *testing.T) {
	eng := New()
	var wg sync.WaitGroup
	n := 20
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			eng.Generate("agent", []byte("i"), []byte("o"), []byte("m"), "uc")
		}(i)
	}
	wg.Wait()
	all := eng.List()
	if len(all) != n {
		t.Fatalf("want %d proofs, got %d", n, len(all))
	}
}

func TestConcurrentVerify(t *testing.T) {
	eng := New()
	eng.Generate("a", []byte("1"), []byte("2"), []byte("3"), "uc")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := eng.Verify("P-1")
			if err != nil || !ok {
				t.Errorf("concurrent verify failed")
			}
		}()
	}
	wg.Wait()
}
