package keystore

import (
	"context"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

// MemoryKeystore is a thin adapter that satisfies the Keystore interface
// using the existing in-process KeyRing. It is the default backend and the
// only one CI actually exercises end-to-end.
//
// SECURITY: private keys live in process memory only. Suitable for local
// dev and integration tests; unsuitable for production. Use FileKeystore
// or a remote HSM/KMS driver for production deployments.
type MemoryKeystore struct {
	ring *pqcrypto.KeyRing
}

// NewMemory wraps an existing KeyRing (typically NewKeyRing()) into the
// keystore interface.
func NewMemory(ring *pqcrypto.KeyRing) *MemoryKeystore {
	if ring == nil {
		ring = pqcrypto.NewKeyRing()
	}
	return &MemoryKeystore{ring: ring}
}

// Ring exposes the underlying KeyRing. Provided so tests and back-compat
// call sites that still expect a *KeyRing can reach through the adapter.
// New code should depend on the Keystore interface, not on Ring().
func (m *MemoryKeystore) Ring() *pqcrypto.KeyRing {
	return m.ring
}

func (m *MemoryKeystore) Info(_ context.Context) Info {
	return Info{
		Kind:           KindMemory,
		Backing:        "process memory (lost on restart)",
		KeyCount:       len(m.ring.List()),
		Persistent:     false,
		HardwareBacked: false,
	}
}

func (m *MemoryKeystore) CreateKey(_ context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error) {
	return m.ring.CreateKey(algo)
}

func (m *MemoryKeystore) RotateKey(_ context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error) {
	return m.ring.RotateKey(algo)
}

func (m *MemoryKeystore) ActiveKeyID(_ context.Context, algo pqcrypto.Algo) (string, bool) {
	return m.ring.ActiveKeyID(algo)
}

func (m *MemoryKeystore) GetMeta(_ context.Context, id string) (pqcrypto.KeyMeta, error) {
	return m.ring.GetMeta(id)
}

func (m *MemoryKeystore) List(_ context.Context) []pqcrypto.KeyMeta {
	return m.ring.List()
}

func (m *MemoryKeystore) Sign(_ context.Context, algo pqcrypto.Algo, message []byte) ([]byte, string, error) {
	return m.ring.Sign(algo, message)
}

func (m *MemoryKeystore) SignWithKey(_ context.Context, id string, message []byte) ([]byte, error) {
	return m.ring.SignWithKey(id, message)
}

func (m *MemoryKeystore) Verify(_ context.Context, id string, message, signature []byte) (bool, error) {
	return m.ring.Verify(id, message, signature)
}

func (m *MemoryKeystore) MigrateSignature(_ context.Context, oldKeyID string, message, oldSig []byte, toAlgo pqcrypto.Algo) ([]byte, string, error) {
	return m.ring.MigrateSignature(oldKeyID, message, oldSig, toAlgo)
}

// compile-time interface assertion
var _ Keystore = (*MemoryKeystore)(nil)
