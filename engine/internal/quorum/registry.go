package quorum

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// SignerStatus lifecycle: active → slashed (terminal) or active → removed.
type SignerStatus string

const (
	StatusActive  SignerStatus = "active"
	StatusSlashed SignerStatus = "slashed"
	StatusRemoved SignerStatus = "removed"
)

// SignerRecord is the in-memory representation of one committee member.
// Mirrors the on-chain `signer-registry` contract fields (see
// docs/roadmap/BLS_QUORUM.md).
type SignerRecord struct {
	ID           string       `json:"id"`
	PublicKeyHex string       `json:"public_key_hex"`
	Bond         uint64       `json:"bond"`
	Status       SignerStatus `json:"status"`
	RegisteredAt time.Time    `json:"registered_at"`
	SlashedAt    *time.Time   `json:"slashed_at,omitempty"`
	SlashReason  string       `json:"slash_reason,omitempty"`

	// Parsed pubkey — nil if the hex could not be decoded on load.
	pk *PublicKey
}

// Registry is a thread-safe in-memory signer registry. The Postgres
// backing is a follow-up (documented in BLS_QUORUM.md §"Persistence"),
// same shape as receipts.Store — swap the map for a driver, no change to
// callers.
type Registry struct {
	mu      sync.RWMutex
	signers map[string]*SignerRecord
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{signers: make(map[string]*SignerRecord)}
}

// Register adds a signer under the canonical author's authority.
// Duplicate id → error. Bond amount is stored but not enforced here
// (the on-chain contract escrows; this is bookkeeping).
func (r *Registry) Register(id string, pk *PublicKey, bond uint64) error {
	if id == "" {
		return fmt.Errorf("quorum/registry: empty signer id")
	}
	if pk == nil {
		return ErrEmptyPubKey
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.signers[id]; exists {
		return fmt.Errorf("quorum/registry: signer %q already registered", id)
	}
	pkBytes, _ := pk.MarshalBinary()
	r.signers[id] = &SignerRecord{
		ID:           id,
		PublicKeyHex: fmt.Sprintf("%x", pkBytes),
		Bond:         bond,
		Status:       StatusActive,
		RegisteredAt: time.Now().UTC(),
		pk:           pk,
	}
	return nil
}

// Get returns a snapshot copy of the record (or nil if missing).
func (r *Registry) Get(id string) *SignerRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.signers[id]
	if !ok {
		return nil
	}
	// Copy so callers can't mutate the internal record.
	out := *rec
	return &out
}

// List returns all records in id-sorted order. Status filter is
// applied when non-empty.
func (r *Registry) List(filter SignerStatus) []*SignerRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*SignerRecord, 0, len(r.signers))
	for _, rec := range r.signers {
		if filter != "" && rec.Status != filter {
			continue
		}
		cp := *rec
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Slash marks a signer's bond forfeit. Reason is free-text audit only.
// Idempotent: a second slash on the same id is a no-op and returns nil
// (the on-chain contract has similar guard).
func (r *Registry) Slash(id, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.signers[id]
	if !ok {
		return ErrUnknownSigner
	}
	if rec.Status == StatusSlashed {
		return nil
	}
	now := time.Now().UTC()
	rec.Status = StatusSlashed
	rec.SlashedAt = &now
	rec.SlashReason = reason
	return nil
}

// Retire marks a signer as removed (voluntary exit — no bond forfeit).
// A slashed signer cannot be retired (state is terminal). Idempotent
// on already-removed signers.
func (r *Registry) Retire(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.signers[id]
	if !ok {
		return ErrUnknownSigner
	}
	if rec.Status == StatusSlashed {
		return fmt.Errorf("quorum/registry: signer %q is slashed — cannot retire", id)
	}
	if rec.Status == StatusRemoved {
		return nil
	}
	rec.Status = StatusRemoved
	return nil
}

// activePubKeys returns the pubkeys for the ids listed, in the given
// order, refusing ids that are missing / slashed / decode-failed. On
// error it names the offending id.
func (r *Registry) activePubKeys(ids []string) ([]*PublicKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*PublicKey, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateSigner, id)
		}
		seen[id] = struct{}{}
		rec, ok := r.signers[id]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownSigner, id)
		}
		if rec.Status != StatusActive {
			return nil, fmt.Errorf("%w: %s (status=%s)", ErrInactiveSigner, id, rec.Status)
		}
		if rec.pk == nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidPubKey, id)
		}
		out = append(out, rec.pk)
	}
	return out, nil
}

// ActiveCount returns the number of ACTIVE signers. Cheap; used by
// callers that need to compute a threshold before assembling a bitset.
func (r *Registry) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, rec := range r.signers {
		if rec.Status == StatusActive {
			n++
		}
	}
	return n
}
