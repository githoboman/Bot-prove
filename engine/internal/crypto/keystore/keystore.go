// Package keystore — abstraction over PQ key storage backends.
//
// The engine's signing surface (see internal/crypto.KeyRing) originally
// held every private key in process memory. That's fine for local dev and
// integration tests, but in production a key must live in hardware-backed
// storage (HSM, cloud KMS, or a sealed enclave) so a compromised engine
// process cannot exfiltrate it.
//
// This package defines a small interface — Keystore — that the API layer
// depends on instead of the concrete KeyRing type. Three reference
// implementations are provided:
//
//   - MemoryKeystore  — thin wrapper around KeyRing. Default. In-memory
//                       only, lost on process restart. Demo-grade.
//   - FileKeystore    — persists private-key material to a file, encrypted
//                       at rest with ChaCha20-Poly1305, KDF Argon2id. Opt-in
//                       via CP_KEYSTORE_KIND=file + CP_KEYSTORE_PATH +
//                       CP_KEYSTORE_PASSPHRASE (from env or Vault). Better
//                       than memory, still SOFTWARE key storage — not HSM.
//   - RemoteKeystoreStub — HTTP client that delegates Sign to a configurable
//                       endpoint. Present ONLY to document the interface an
//                       HSM/KMS gateway must implement; the endpoint URL is
//                       not shipped and the stub returns "not_configured"
//                       until wired. See docs/KEYSTORE.md for the full
//                       gateway contract and a reference server-side sketch.
//
// Honest scope note (2.10 follow-up):
//
//   Neither FileKeystore nor RemoteKeystoreStub is a substitute for a real
//   HSM. FileKeystore is a stepping-stone that removes the "restart wipes
//   everything" failure mode and encrypts the key material at rest, but the
//   moment the engine unwraps a key to sign, the plaintext exists in engine
//   memory — same threat model as MemoryKeystore for a running process.
//   RemoteKeystoreStub documents the boundary; the actual HSM/KMS driver
//   (AWS KMS, Google Cloud KMS, YubiHSM, Nitrokey HSM) belongs in a
//   downstream deployment and is intentionally out of scope for this repo.
package keystore

import (
	"context"
	"errors"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

// Kind identifies a keystore backend.
type Kind string

const (
	KindMemory       Kind = "memory"
	KindFile         Kind = "file"
	KindRemote       Kind = "remote"
	KindVaultTransit Kind = "vault-transit"
)

// Info describes a keystore instance for the /v1/pq/keystore/info endpoint.
type Info struct {
	// Kind is one of memory/file/remote.
	Kind Kind `json:"kind"`
	// Backing is a short human-readable description of where private keys
	// live: "process memory", "encrypted file at <path>", "HSM gateway at
	// <url>", etc. Never includes secret material or credentials.
	Backing string `json:"backing"`
	// KeyCount is the number of keys (active + retired) known to the
	// keystore at the moment of the call.
	KeyCount int `json:"key_count"`
	// Persistent is true iff private-key material survives a process
	// restart. MemoryKeystore is false; File/Remote are true.
	Persistent bool `json:"persistent"`
	// HardwareBacked is true iff the private key never appears in engine
	// process memory in plaintext form. MemoryKeystore and FileKeystore
	// are both false. Only a properly-wired RemoteKeystore that delegates
	// Sign to an HSM/KMS can honestly set this to true.
	HardwareBacked bool `json:"hardware_backed"`
}

// Keystore is the interface the API layer depends on.
//
// The three "write" methods (CreateKey / RotateKey / MigrateSignature) are
// distinct from Sign because a hardware-backed keystore typically ships
// key-generation to the HSM (Generate command) but exposes only the public
// half back, whereas Sign issues a delegated cryptographic operation on the
// existing handle. Callers use IDs, not raw material.
//
// A Keystore is expected to be safe for concurrent use.
type Keystore interface {
	// Info returns a snapshot of the keystore's state and backing.
	Info(ctx context.Context) Info

	// CreateKey generates a fresh key pair for algo. Returns the metadata
	// of the new key, which becomes the active key for that algo (any
	// previous active key is retired).
	CreateKey(ctx context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error)

	// RotateKey is a semantic alias for CreateKey — provided so callers can
	// express intent ("rotate" vs "create") explicitly.
	RotateKey(ctx context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error)

	// ActiveKeyID returns the active key ID for algo, if any.
	ActiveKeyID(ctx context.Context, algo pqcrypto.Algo) (string, bool)

	// GetMeta returns metadata for a specific key.
	GetMeta(ctx context.Context, id string) (pqcrypto.KeyMeta, error)

	// List returns all keys (active + retired), sorted stably.
	List(ctx context.Context) []pqcrypto.KeyMeta

	// Sign signs message with the active key for algo. Returns signature
	// bytes and the key ID that produced them.
	Sign(ctx context.Context, algo pqcrypto.Algo, message []byte) (signature []byte, keyID string, err error)

	// SignWithKey signs message with a specific key ID (may be retired).
	SignWithKey(ctx context.Context, id string, message []byte) ([]byte, error)

	// Verify verifies signature against key ID's public half.
	Verify(ctx context.Context, id string, message, signature []byte) (bool, error)

	// MigrateSignature verifies oldSig under oldKeyID and re-signs the same
	// message with the active key of toAlgo.
	MigrateSignature(ctx context.Context, oldKeyID string, message, oldSig []byte, toAlgo pqcrypto.Algo) (newSig []byte, newKeyID string, err error)
}

// ErrNotConfigured is returned by keystores that require explicit env-var
// wiring (FileKeystore, RemoteKeystoreStub) but haven't been configured.
var ErrNotConfigured = errors.New("keystore: backend not configured")

// ErrNotSupported is returned by RemoteKeystoreStub when a real HSM/KMS
// driver has not been plugged in.
var ErrNotSupported = errors.New("keystore: operation requires HSM/KMS driver — see docs/KEYSTORE.md")
