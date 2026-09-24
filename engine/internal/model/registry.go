package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var hexHashRe = regexp.MustCompile(`^[a-f0-9]{64}$`)

type ModelVersion struct {
	Major int
	Minor int
	Patch int
}

func (v ModelVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

type ModelStatus string

const (
	StatusActive     ModelStatus = "active"
	StatusDeprecated ModelStatus = "deprecated"
	StatusRevoked    ModelStatus = "revoked"
)

type ModelEntry struct {
	ID                 string
	Name               string
	Hash               string
	Owner              string
	Version            ModelVersion
	IPFSCID            string
	InputSchema        string
	OutputSchema       string
	RegisteredAt       time.Time
	UpdatedAt          time.Time
	Status             ModelStatus
	AttestationCount   int
	LastAttestationAt  *time.Time
}

type ModelAttestation struct {
	ModelID    string
	AttesterID string
	Hash       string
	Timestamp  time.Time
	Valid      bool
	Evidence   string
}

type Registry struct {
	mu           sync.RWMutex
	models       map[string]*ModelEntry
	attestations map[string][]*ModelAttestation
	byHash       map[string]string
}

func New() *Registry {
	return &Registry{
		models:       make(map[string]*ModelEntry),
		attestations: make(map[string][]*ModelAttestation),
		byHash:       make(map[string]string),
	}
}

func ComputeModelHash(architecture, weights, hyperparams []byte) string {
	h := sha256.New()
	h.Write(architecture)
	h.Write(weights)
	h.Write(hyperparams)
	return hex.EncodeToString(h.Sum(nil))
}

func genID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is extremely unlikely but must not crash the process.
		// Fall back to a timestamp-based ID — unique enough for in-memory registry.
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (r *Registry) Register(name, owner string, hash string, ipfsCID string, schema ...string) (*ModelEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" || owner == "" || hash == "" {
		return nil, fmt.Errorf("name, owner, and hash required")
	}

	if !hexHashRe.MatchString(hash) {
		return nil, fmt.Errorf("hash must be 64-character hex SHA-256")
	}

	if len(name) > 256 {
		return nil, fmt.Errorf("name too long (max 256 chars)")
	}

	if _, ok := r.byHash[hash]; ok {
		return nil, fmt.Errorf("model with hash already exists")
	}

	id := genID()
	// Ensure no ID collision
	for _, exists := r.models[id]; exists; _, exists = r.models[id] {
		id = genID()
	}
	now := time.Now()

	inputSchema := ""
	outputSchema := ""
	if len(schema) > 0 {
		inputSchema = schema[0]
	}
	if len(schema) > 1 {
		outputSchema = schema[1]
	}

	m := &ModelEntry{
		ID:           id,
		Name:         name,
		Hash:         hash,
		Owner:        owner,
		RegisteredAt: now,
		UpdatedAt:    now,
		Status:       StatusActive,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		IPFSCID:      ipfsCID,
	}

	r.models[id] = m
	r.byHash[hash] = id

	return m, nil
}

func (r *Registry) Get(id string) (*ModelEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.models[id]
	return m, ok
}

func (r *Registry) GetByHash(hash string) (*ModelEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byHash[hash]
	if !ok {
		return nil, false
	}
	m, ok := r.models[id]
	return m, ok
}

func (r *Registry) Attest(modelID, attesterID, evidence string) (*ModelAttestation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.models[modelID]
	if !ok {
		return nil, fmt.Errorf("model not found")
	}

	if m.Status != StatusActive {
		return nil, fmt.Errorf("model not active")
	}

	a := &ModelAttestation{
		ModelID:    modelID,
		AttesterID: attesterID,
		Hash:       m.Hash,
		Timestamp:  time.Now(),
		Valid:      true,
		Evidence:   evidence,
	}

	r.attestations[modelID] = append(r.attestations[modelID], a)
	m.AttestationCount++
	m.LastAttestationAt = &a.Timestamp
	m.UpdatedAt = a.Timestamp

	return a, nil
}

func (r *Registry) ListAttestations(modelID string) []*ModelAttestation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ModelAttestation, len(r.attestations[modelID]))
	copy(out, r.attestations[modelID])
	return out
}

func (r *Registry) Deprecate(modelID, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setStatus(modelID, reason, StatusDeprecated)
}

func (r *Registry) Revoke(modelID, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setStatus(modelID, reason, StatusRevoked)
}

func (r *Registry) setStatus(modelID, reason string, status ModelStatus) error {
	m, ok := r.models[modelID]
	if !ok {
		return fmt.Errorf("model not found")
	}
	_ = reason
	m.Status = status
	m.UpdatedAt = time.Now()
	return nil
}

func (r *Registry) VerifyIntegrity(modelID string, candidateHash string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.models[modelID]
	if !ok {
		return false
	}
	return m.Hash == candidateHash
}

func (r *Registry) ListByOwner(owner string) []*ModelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*ModelEntry
	for _, m := range r.models {
		if m.Owner == owner {
			out = append(out, m)
		}
	}
	return out
}

func (r *Registry) Search(query string) []*ModelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*ModelEntry
	q := strings.ToLower(query)
	for _, m := range r.models {
		if strings.HasPrefix(strings.ToLower(m.Name), q) ||
			strings.HasPrefix(m.Hash, q) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalAttestations := 0
	byStatus := map[string]int{}
	for _, m := range r.models {
		byStatus[string(m.Status)]++
	}
	for _, a := range r.attestations {
		totalAttestations += len(a)
	}

	return map[string]interface{}{
		"total_models":       len(r.models),
		"total_attestations": totalAttestations,
		"by_status":          byStatus,
	}
}
