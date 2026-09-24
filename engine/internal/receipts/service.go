package receipts

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto/keystore"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/hitl"
)

// Store is the durable receipt index. In-memory is the default; a
// production deployment plugs a Postgres-backed implementation in via
// this interface. The store is content-addressed by receipt ID and
// (separately) by canonical hash.
type Store interface {
	Put(ctx context.Context, r DecisionReceipt) error
	GetByID(ctx context.Context, id string) (DecisionReceipt, error)
	GetByHash(ctx context.Context, hash string) (DecisionReceipt, error)
	List(ctx context.Context, limit int) ([]DecisionReceipt, error)
	// Ancestors returns the chain of upstream receipts reachable from
	// id via ProviderReceipts. Depth-first, cycle-guarded, capped at
	// maxDepth. A DAG loop returns ErrCycle.
	Ancestors(ctx context.Context, id string, maxDepth int) ([]DecisionReceipt, error)
}

// ErrNotFound is returned by Store when a receipt id / hash is unknown.
var ErrNotFound = errors.New("receipts: not found")

// ErrCycle is returned by Store.Ancestors when the lineage graph
// contains a cycle. The engine never produces one (a receipt id is
// minted at emit time and can only reference receipts already stored),
// but a corrupted store or a hand-crafted payload could — the guard is
// defensive.
var ErrCycle = errors.New("receipts: lineage cycle detected")

// OtelSink is the observability plug. Every emitted receipt is passed
// to Record; the default implementation writes JSONL to a file.
// Production deployments swap in an OpenTelemetry-native implementation
// that emits a Span with the receipt fields as attributes — see
// docs/PROVENANCE_LINEAGE.md for the attribute mapping.
type OtelSink interface {
	Record(ctx context.Context, r DecisionReceipt) error
}

// noopSink is the default OtelSink — silent, always succeeds. Used when
// a deployment has not wired an OTel collector.
type noopSink struct{}

func (noopSink) Record(context.Context, DecisionReceipt) error { return nil }

// NoopSink returns an OtelSink that discards receipts. Exported so
// tests can pass it explicitly.
func NoopSink() OtelSink { return noopSink{} }

// Service is the emit path. It signs a receipt with the active
// signing key, indexes it in the store, and forwards a copy to the
// OTel sink. Sign order is:
//
//   1. Fill in ID and IssuedAt (once — the ID must not change if a caller
//      retries with the same input).
//   2. Compute CanonicalHash over the unsigned receipt.
//   3. Sign the hash with keystore.Sign(SigningAlgo).
//   4. Populate Proof, persist to Store, emit to OTel sink.
type Service struct {
	// Store is required.
	Store Store
	// Keystore is required.
	Keystore keystore.Keystore
	// SigningAlgo defaults to AlgoHybrid.
	SigningAlgo pqcrypto.Algo
	// IssuerDID identifies the engine. When empty, the service
	// derives a did:key form from the keystore's active key id.
	IssuerDID string
	// Sink defaults to NoopSink().
	Sink OtelSink
	// Now returns the emit time. Tests override this to freeze the clock.
	Now func() time.Time
	// NewID mints a fresh receipt ID. Tests override this to make
	// the digest reproducible.
	NewID func() string
}

// NewService returns a Service with production defaults.
func NewService(store Store, ks keystore.Keystore) *Service {
	if store == nil || ks == nil {
		panic("receipts: NewService requires non-nil store and keystore")
	}
	return &Service{
		Store:       store,
		Keystore:    ks,
		SigningAlgo: pqcrypto.AlgoHybrid,
		Sink:        NoopSink(),
		Now:         func() time.Time { return time.Now().UTC() },
		NewID:       defaultNewID,
	}
}

// EmitInput carries every field the caller has to supply. Fields left
// zero-valued fall back to their canonical defaults (empty facets, no
// lineage, no HITL). Aggregate MUST be non-zero.
type EmitInput struct {
	Commit           attest.DecisionCommit
	EvidenceRoot     string
	ModelID          string
	ProviderReceipts []ProviderReceipt
	HITL             *hitl.Response
	// Reviewer is populated only when HITL.Action == ActionEscalate and
	// the ticket has been resolved by a human. Empty for auto-passed /
	// vetoed paths.
	Reviewer string
}

// Emit builds, signs, stores and observes a DecisionReceipt from input.
func (s *Service) Emit(ctx context.Context, in EmitInput) (DecisionReceipt, error) {
	if in.Commit.DecisionID == "" {
		return DecisionReceipt{}, errors.New("receipts: EmitInput.Commit.DecisionID is empty")
	}
	if in.Commit.Aggregate == attest.VerdictUnknown {
		return DecisionReceipt{}, errors.New("receipts: EmitInput.Commit.Aggregate is UNKNOWN")
	}

	r := DecisionReceipt{
		ID:               s.NewID(),
		IssuedAt:         s.Now(),
		Subject:          in.Commit.DecisionID,
		SpecID:           in.Commit.Decision.SpecID,
		EvidenceRoot:     in.EvidenceRoot,
		ModelID:          in.ModelID,
		Aggregate:        FromDecision(in.Commit.Aggregate),
		VetoedBy:         string(in.Commit.VetoedBy),
		Confidence:       meanNonCriticalConfidence(in.Commit.FacetVerdicts),
		Facets:           facetsFromCommit(in.Commit.FacetVerdicts),
		ProviderReceipts: normaliseProviders(in.ProviderReceipts),
	}
	if in.HITL != nil {
		r.HITL = &HITLResolution{
			TicketID:   in.HITL.TicketID,
			Action:     string(in.HITL.Action),
			Reviewer:   in.Reviewer,
			ResolvedAt: r.IssuedAt,
			Note:       in.HITL.Reason,
		}
	}
	activeID, ok := s.Keystore.ActiveKeyID(ctx, s.SigningAlgo)
	if !ok {
		return DecisionReceipt{}, fmt.Errorf("receipts: no active key for algo %s", s.SigningAlgo)
	}
	if s.IssuerDID != "" {
		r.Issuer = s.IssuerDID
	} else {
		r.Issuer = "did:cp:" + activeID
	}

	digest := CanonicalHash(r)
	sig, keyID, err := s.Keystore.Sign(ctx, s.SigningAlgo, []byte(digest))
	if err != nil {
		return DecisionReceipt{}, fmt.Errorf("receipts: sign: %w", err)
	}
	r.Proof = &Proof{
		Scheme:             string(s.SigningAlgo),
		Signature:          base64.StdEncoding.EncodeToString(sig),
		VerificationMethod: r.Issuer + "#" + keyID,
		SignedAt:           r.IssuedAt,
	}
	if err := s.Store.Put(ctx, r); err != nil {
		return DecisionReceipt{}, fmt.Errorf("receipts: store: %w", err)
	}
	if s.Sink != nil {
		if err := s.Sink.Record(ctx, r); err != nil {
			// OTel emission failure MUST NOT roll back the store.
			// Downstream logging is the deployment's responsibility.
			return r, fmt.Errorf("receipts: sink: %w", err)
		}
	}
	return r, nil
}

// Verify checks r.Proof.Signature against CanonicalHash(r) using the
// keystore's public half. Returns (true, nil) on success, (false, nil)
// on signature mismatch, (false, err) on infrastructure error.
func (s *Service) Verify(ctx context.Context, r DecisionReceipt) (bool, error) {
	if r.Proof == nil {
		return false, errors.New("receipts: receipt is not signed")
	}
	sig, err := base64.StdEncoding.DecodeString(r.Proof.Signature)
	if err != nil {
		return false, fmt.Errorf("receipts: bad signature encoding: %w", err)
	}
	// Strip Proof for hashing.
	unsigned := r
	unsigned.Proof = nil
	digest := CanonicalHash(unsigned)
	// Verification method is "did:cp:<keyid>#<keyid>"; extract keyid.
	keyID := r.Proof.VerificationMethod
	if idx := lastIndex(keyID, '#'); idx >= 0 {
		keyID = keyID[idx+1:]
	}
	return s.Keystore.Verify(ctx, keyID, []byte(digest), sig)
}

func lastIndex(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func facetsFromCommit(fs []attest.FacetVerdict) []FacetOutput {
	out := make([]FacetOutput, 0, len(fs))
	for _, f := range fs {
		out = append(out, FacetOutput{
			Kind:       string(f.Kind),
			Verdict:    FromDecision(f.Verdict),
			Confidence: f.Confidence,
			Reason:     f.Reason,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

func meanNonCriticalConfidence(fs []attest.FacetVerdict) float64 {
	sum, n := 0.0, 0
	for _, f := range fs {
		if f.Kind == attest.FacetSafety || f.Kind == attest.FacetEquivocation {
			continue
		}
		sum += f.Confidence
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func normaliseProviders(ps []ProviderReceipt) []ProviderReceipt {
	if len(ps) == 0 {
		return nil
	}
	out := append([]ProviderReceipt(nil), ps...)
	sort.Slice(out, func(i, j int) bool { return out[i].ReceiptHash < out[j].ReceiptHash })
	return out
}

func defaultNewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// The engine's crypto RNG cannot legitimately fail; if it does
		// the process is compromised. Panic is safer than silently
		// emitting a receipt with a predictable id.
		panic(fmt.Sprintf("receipts: crypto/rand.Read failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

// InMemoryStore is the default Store implementation: content-addressed
// index in a mutex-protected map. Suitable for the demo path; a
// production deployment swaps in a Postgres-backed store via the Store
// interface.
type InMemoryStore struct {
	mu     sync.RWMutex
	byID   map[string]DecisionReceipt
	byHash map[string]DecisionReceipt
}

// NewInMemoryStore returns a fresh store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		byID:   map[string]DecisionReceipt{},
		byHash: map[string]DecisionReceipt{},
	}
}

// Put stores r under both its ID and its canonical hash.
func (s *InMemoryStore) Put(_ context.Context, r DecisionReceipt) error {
	if r.ID == "" {
		return errors.New("receipts: cannot store receipt without ID")
	}
	unsigned := r
	unsigned.Proof = nil
	hash := CanonicalHash(unsigned)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[r.ID] = r
	s.byHash[hash] = r
	return nil
}

// GetByID returns the receipt for id, or ErrNotFound.
func (s *InMemoryStore) GetByID(_ context.Context, id string) (DecisionReceipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.byID[id]
	if !ok {
		return DecisionReceipt{}, ErrNotFound
	}
	return r, nil
}

// GetByHash returns the receipt for a canonical hash, or ErrNotFound.
func (s *InMemoryStore) GetByHash(_ context.Context, hash string) (DecisionReceipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.byHash[hash]
	if !ok {
		return DecisionReceipt{}, ErrNotFound
	}
	return r, nil
}

// List returns receipts sorted by IssuedAt descending, capped at limit.
func (s *InMemoryStore) List(_ context.Context, limit int) ([]DecisionReceipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DecisionReceipt, 0, len(s.byID))
	for _, r := range s.byID {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Ancestors walks ProviderReceipts breadth-first, gathering every
// upstream receipt reachable from id. maxDepth caps the traversal.
func (s *InMemoryStore) Ancestors(ctx context.Context, id string, maxDepth int) ([]DecisionReceipt, error) {
	seen := map[string]bool{id: true}
	frontier := []string{id}
	var out []DecisionReceipt
	depth := 0
	for len(frontier) > 0 {
		if maxDepth > 0 && depth >= maxDepth {
			break
		}
		var next []string
		for _, fid := range frontier {
			r, err := s.GetByID(ctx, fid)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return nil, err
			}
			if fid != id {
				out = append(out, r)
			}
			for _, pr := range r.ProviderReceipts {
				anc, err := s.GetByHash(ctx, pr.ReceiptHash)
				if err != nil {
					// Missing upstream: skip, don't fail. Producer
					// receipts may live in a different store.
					continue
				}
				if seen[anc.ID] {
					return nil, ErrCycle
				}
				seen[anc.ID] = true
				next = append(next, anc.ID)
			}
		}
		frontier = next
		depth++
	}
	return out, nil
}
