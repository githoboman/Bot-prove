package prover

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/hasher"
)

type ProofEngine struct {
	mu     sync.RWMutex
	proofs map[string]*Proof
	seq    int
}

func New() *ProofEngine {
	return &ProofEngine{proofs: make(map[string]*Proof)}
}

func (e *ProofEngine) Generate(agent string, input, output, model []byte, uc string) *Proof {
	return e.GenerateWithKey(agent, "", input, output, model, uc, "local")
}

func (e *ProofEngine) GenerateWithKey(agent, pubKey string, input, output, model []byte, uc, mode string) *Proof {
	start := time.Now()

	e.mu.Lock()
	defer e.mu.Unlock()

	e.evictIfNeeded()
	e.seq++
	pid := fmt.Sprintf("P-%d", e.seq)
	ph := hasher.CommitHash(input, output, model)
	ih := hasher.HexHash(input)
	oh := hasher.HexHash(output)
	mh := hasher.HexHash(model)

	leaves := [][]byte{input, output, model}
	root := Root(leaves)
	path := GetPath(leaves, 0)

	elapsed := time.Since(start).Milliseconds()
	if elapsed < 1 {
		elapsed = 1
	}

	if mode == "" {
		mode = "local"
	}

	p := &Proof{
		ID:      pid,
		Agent:   agent,
		PH:      ph,
		IH:      ih,
		OH:      oh,
		MH:      mh,
		Root:    root,
		Path:    path,
		Idx:     0,
		TS:      time.Now().Unix(),
		Valid:   true,
		UseCase: uc,
		PubKey:  pubKey,
		GenMs:   elapsed,
		Mode:    mode,
	}
	e.proofs[pid] = p
	return p
}

func (e *ProofEngine) Get(pid string) (*Proof, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p, ok := e.proofs[pid]
	return p, ok
}

func (e *ProofEngine) Revoke(pid, reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.proofs[pid]
	if !ok {
		return fmt.Errorf("proof %s not found", pid)
	}
	if p.Revoked {
		return fmt.Errorf("proof %s already revoked", pid)
	}
	p.Valid = false
	p.Revoked = true
	return nil
}

// EvictRevoked removes revoked proofs older than maxAge to prevent unbounded memory growth.
func (e *ProofEngine) EvictRevoked(maxAge time.Duration) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	cutoff := time.Now().Add(-maxAge).Unix()
	evicted := 0
	for id, p := range e.proofs {
		if p.Revoked && p.TS < cutoff {
			delete(e.proofs, id)
			evicted++
		}
	}
	return evicted
}

// MaxProofs is the upper bound for in-memory proofs. Oldest non-revoked proofs
// are kept when the limit is hit; eviction runs on each Generate call.
const MaxProofs = 100_000

func (e *ProofEngine) evictIfNeeded() {
	if len(e.proofs) <= MaxProofs {
		return
	}
	// Remove revoked proofs first (any age)
	for id, p := range e.proofs {
		if p.Revoked {
			delete(e.proofs, id)
		}
	}
}

func (e *ProofEngine) Verify(pid string) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p, ok := e.proofs[pid]
	if !ok {
		return false, fmt.Errorf("proof %s not found", pid)
	}
	return p.Valid && !p.Revoked, nil
}

func (e *ProofEngine) List() []*Proof {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]*Proof, 0, len(e.proofs))
	for _, p := range e.proofs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	return out
}

type ListFilter struct {
	Agent  string
	PubKey string
	Mode   string
	Page   int
	Limit  int
}

func (e *ProofEngine) ListFiltered(f ListFilter) ([]*Proof, int) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var filtered []*Proof
	for _, p := range e.proofs {
		if f.Agent != "" && p.Agent != f.Agent {
			continue
		}
		if f.PubKey != "" && p.PubKey != f.PubKey {
			continue
		}
		if f.Mode != "" && p.Mode != f.Mode {
			continue
		}
		filtered = append(filtered, p)
	}

	sort.Slice(filtered, func(i, j int) bool { return filtered[i].TS > filtered[j].TS })

	total := len(filtered)

	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	start := (f.Page - 1) * f.Limit
	if start >= total {
		return nil, total
	}
	end := start + f.Limit
	if end > total {
		end = total
	}

	return filtered[start:end], total
}

// Restore adds a pre-built proof to the engine (used for DB loading).
func (e *ProofEngine) Restore(p *Proof) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cp := *p
	e.proofs[cp.ID] = &cp

	var n int
	if _, err := fmt.Sscanf(cp.ID, "P-%d", &n); err == nil && n > e.seq {
		e.seq = n
	}
}

func (e *ProofEngine) GetStats() *Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	s := &Stats{UseCases: make(map[string]int)}
	agents := make(map[string]bool)
	var totalMs int64
	maxDepth := 0

	for _, p := range e.proofs {
		s.Total++
		if p.Valid && !p.Revoked {
			s.Valid++
		}
		if p.Revoked {
			s.Revoked++
		}
		agents[p.Agent] = true
		totalMs += p.GenMs
		if len(p.Path) > maxDepth {
			maxDepth = len(p.Path)
		}
		uc := p.UseCase
		if uc == "" {
			uc = "general"
		}
		s.UseCases[uc]++
	}

	s.Agents = len(agents)
	s.MaxDepth = maxDepth
	if s.Total > 0 {
		s.AvgGenMs = float64(totalMs) / float64(s.Total)
	}

	return s
}
