// Package gnarkzk — circuit registry.
//
// Motivation: the original NewSetup() hard-wired a single PreimageCircuit
// and regenerated its trusted-setup artifacts on every process start.
// Real deployments need (a) named, versioned circuits selectable by
// circuit_id from the API surface, and (b) persistent proving/verifying
// keys so restarts don't invalidate previously-issued proofs and don't
// spend CPU re-running Setup.
//
// This file provides both: a Registry holding one Setup per named circuit,
// a Descriptor exposing metadata (id, description, public inputs, curve),
// and Load/Save that persist ccs+pk+vk to disk under CP_ZK_KEYS_DIR.
//
// See persist.go for the disk format and circuits.go for individual
// Circuit implementations.
package gnarkzk

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// PublicInput describes one public input the circuit exposes.
type PublicInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Encoding    string `json:"encoding"` // "big_int_decimal" | "hex_bytes32" | ...
}

// Descriptor is the human/machine-readable metadata for a registered
// circuit. Exposed via GET /v1/circuits.
type Descriptor struct {
	ID           string        `json:"id"`
	Version      string        `json:"version"`
	Description  string        `json:"description"`
	Curve        string        `json:"curve"`
	Backend      string        `json:"backend"`
	PublicInputs []PublicInput `json:"public_inputs"`
	Constraints  int           `json:"constraints,omitempty"`
	KeyDigest    string        `json:"key_digest,omitempty"` // sha256 hex of vk bytes
}

// Circuit is what a caller registers with the Registry: a struct describing
// the circuit definition, its descriptor, and a helper for constructing
// witness assignments from typed inputs.
type Circuit interface {
	// Descriptor returns the circuit's static metadata. Constraints and
	// KeyDigest fields on the descriptor are populated by the registry
	// after compilation/setup, not by the Circuit itself.
	Descriptor() Descriptor
	// NewCircuit returns a fresh, empty circuit instance for compile time
	// (all frontend.Variable fields zero). Called once per Setup.
	NewCircuit() frontend.Circuit
	// AssignFull returns a fully-assigned witness (private + public) for
	// proving. inputs is a free-form map keyed by public/private field
	// names as understood by the specific circuit.
	AssignFull(inputs map[string]any) (frontend.Circuit, error)
	// AssignPublic returns a public-only assignment for verification.
	AssignPublic(inputs map[string]any) (frontend.Circuit, error)
}

// compiled bundles the artifacts a single circuit has after setup.
type compiled struct {
	circuit    Circuit
	descriptor Descriptor
	ccs        constraint.ConstraintSystem
	pk         groth16.ProvingKey
	vk         groth16.VerifyingKey
}

// Registry is a thread-safe collection of compiled circuits.
type Registry struct {
	mu        sync.RWMutex
	circuits  map[string]*compiled
	defaultID string
}

// NewRegistry returns an empty registry. Use Register + Compile (or the
// LoadOrCreate helper) to populate it.
func NewRegistry() *Registry {
	return &Registry{circuits: map[string]*compiled{}}
}

// Register adds a Circuit to the registry WITHOUT compiling it yet.
// Compile(id) then runs the setup phase. Returns error on duplicate id.
func (r *Registry) Register(c Circuit) error {
	d := c.Descriptor()
	if d.ID == "" {
		return errors.New("circuit descriptor missing id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.circuits[d.ID]; exists {
		return fmt.Errorf("circuit %q already registered", d.ID)
	}
	r.circuits[d.ID] = &compiled{circuit: c, descriptor: d}
	if r.defaultID == "" {
		r.defaultID = d.ID
	}
	return nil
}

// SetDefault marks a registered circuit as the default (used when the API
// caller doesn't pass a circuit_id).
func (r *Registry) SetDefault(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.circuits[id]; !ok {
		return fmt.Errorf("cannot set default: circuit %q not registered", id)
	}
	r.defaultID = id
	return nil
}

// DefaultID returns the default circuit id, or "" if the registry is empty.
func (r *Registry) DefaultID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultID
}

// Compile runs frontend.Compile + groth16.Setup for a registered circuit
// (in-memory only — use LoadOrCreate to also persist).
func (r *Registry) Compile(id string) error {
	r.mu.Lock()
	c, ok := r.circuits[id]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("circuit %q not registered", id)
	}
	if c.ccs != nil {
		return nil // already compiled
	}
	inst := c.circuit.NewCircuit()
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, inst)
	if err != nil {
		return fmt.Errorf("compile %q: %w", id, err)
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return fmt.Errorf("setup %q: %w", id, err)
	}
	r.mu.Lock()
	c.ccs = ccs
	c.pk = pk
	c.vk = vk
	c.descriptor.Constraints = ccs.GetNbConstraints()
	c.descriptor.KeyDigest = digestVK(vk)
	r.mu.Unlock()
	return nil
}

// IDs returns registered circuit ids in stable (sorted) order.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.circuits))
	for id := range r.circuits {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Descriptors returns metadata for every registered circuit (sorted).
func (r *Registry) Descriptors() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Descriptor, 0, len(r.circuits))
	for _, c := range r.circuits {
		out = append(out, c.descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Descriptor returns the metadata for a single circuit id.
func (r *Registry) Descriptor(id string) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.circuits[id]
	if !ok {
		return Descriptor{}, false
	}
	return c.descriptor, true
}

// Prove generates a Groth16 proof for the named circuit given a free-form
// witness input map (whatever the circuit's AssignFull expects).
func (r *Registry) Prove(id string, inputs map[string]any) (groth16.Proof, error) {
	r.mu.RLock()
	c, ok := r.circuits[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("circuit %q not registered", id)
	}
	if c.ccs == nil {
		return nil, fmt.Errorf("circuit %q not compiled (call Compile or LoadOrCreate)", id)
	}
	assignment, err := c.circuit.AssignFull(inputs)
	if err != nil {
		return nil, fmt.Errorf("assign full witness: %w", err)
	}
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("build witness: %w", err)
	}
	proof, err := groth16.Prove(c.ccs, c.pk, witness)
	if err != nil {
		return nil, fmt.Errorf("groth16 prove: %w", err)
	}
	return proof, nil
}

// Verify runs Groth16 pairing verification against public inputs.
func (r *Registry) Verify(id string, proof groth16.Proof, publicInputs map[string]any) (bool, error) {
	r.mu.RLock()
	c, ok := r.circuits[id]
	r.mu.RUnlock()
	if !ok {
		return false, fmt.Errorf("circuit %q not registered", id)
	}
	if c.ccs == nil {
		return false, fmt.Errorf("circuit %q not compiled", id)
	}
	assignment, err := c.circuit.AssignPublic(publicInputs)
	if err != nil {
		return false, fmt.Errorf("assign public witness: %w", err)
	}
	publicWitness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return false, fmt.Errorf("build public witness: %w", err)
	}
	if err := groth16.Verify(proof, c.vk, publicWitness); err != nil {
		return false, nil //nolint:nilerr // a verification failure is a normal false, not a caller error
	}
	return true, nil
}

// VerifyingKey returns the (public) verifying key for a registered
// circuit, useful for callers wanting to export it (e.g. embed on-chain).
func (r *Registry) VerifyingKey(id string) (groth16.VerifyingKey, error) {
	r.mu.RLock()
	c, ok := r.circuits[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("circuit %q not registered", id)
	}
	if c.vk == nil {
		return nil, fmt.Errorf("circuit %q not compiled", id)
	}
	return c.vk, nil
}
