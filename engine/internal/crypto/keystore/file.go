package keystore

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// FileKeystore persists the ring's private + public key material to a file
// on disk, encrypted at rest with ChaCha20-Poly1305 using an Argon2id-derived
// key from a caller-supplied passphrase.
//
// Threat model — what FileKeystore protects against:
//
//   - Process restart wiping the ring: passphrase + file survive; the ring
//     rehydrates on startup.
//   - Passive disk theft / backup exfiltration: without the passphrase, the
//     ciphertext is useless.
//
// Threat model — what FileKeystore does NOT protect against:
//
//   - A compromised running engine process. To sign, the ring is unwrapped
//     into engine memory; from that point on the threat model equals
//     MemoryKeystore.
//   - Passphrase reuse or a passphrase committed to source control. Wire the
//     passphrase from a secret manager (Vault, K8s Secret with encryption at
//     rest, etc.).
//   - Side-channel attacks on the engine host. That's what an HSM / KMS is
//     for. FileKeystore is a stepping stone, not a substitute.
//
// On-disk layout (magic-prefixed for future format bumps):
//
//   bytes 0..3    : "CPFK"                          magic
//   bytes 4..5    : uint16 big-endian format version (currently 1)
//   bytes 6..21   : 16-byte Argon2id salt
//   bytes 22..33  : 12-byte ChaCha20-Poly1305 nonce
//   bytes 34..    : ciphertext + auth tag from Seal(ring_json)
//
// Argon2id parameters are conservative (time=3, memory=64 MiB, threads=4,
// keyLen=32). They are pinned in file version 1; a future v2 could bump.
type FileKeystore struct {
	mu   sync.Mutex
	path string
	pass []byte
	ring *pqcrypto.KeyRing
}

const (
	fileMagic       = "CPFK"
	fileFormatV1    = uint16(1)
	argon2Time      = uint32(3)
	argon2Memory    = uint32(64 * 1024)
	argon2Threads   = uint8(4)
	argon2KeyLen    = uint32(32)
	argon2SaltLen   = 16
	fileHeaderSize  = 4 + 2 + argon2SaltLen + chacha20poly1305.NonceSize // 34
)

// NewFile opens (or creates) an encrypted keystore at path. If the file
// exists, it is decrypted with passphrase and the ring rehydrated.
// If the file does not exist, an empty ring is created and persisted on
// the first mutation.
//
// passphrase must be non-empty. In production wire it from a secret
// manager, never a static default.
func NewFile(path string, passphrase []byte) (*FileKeystore, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: CP_KEYSTORE_PATH is empty", ErrNotConfigured)
	}
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("%w: CP_KEYSTORE_PASSPHRASE is empty", ErrNotConfigured)
	}
	fk := &FileKeystore{
		path: path,
		pass: append([]byte(nil), passphrase...),
	}
	// Ensure parent dir exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("keystore: mkdir parent: %w", err)
	}
	// Try to load existing file. Not-exists = fresh ring.
	if _, err := os.Stat(path); err == nil {
		if err := fk.load(); err != nil {
			return nil, err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		fk.ring = pqcrypto.NewKeyRing()
	} else {
		return nil, fmt.Errorf("keystore: stat %s: %w", path, err)
	}
	return fk, nil
}

// load decrypts fk.path into a KeyRing.
func (fk *FileKeystore) load() error {
	raw, err := os.ReadFile(fk.path)
	if err != nil {
		return fmt.Errorf("keystore: read %s: %w", fk.path, err)
	}
	if len(raw) < fileHeaderSize {
		return fmt.Errorf("keystore: file too small (%d bytes)", len(raw))
	}
	if string(raw[0:4]) != fileMagic {
		return fmt.Errorf("keystore: bad magic (want %q)", fileMagic)
	}
	ver := uint16(raw[4])<<8 | uint16(raw[5])
	if ver != fileFormatV1 {
		return fmt.Errorf("keystore: unsupported format version %d", ver)
	}
	salt := raw[6 : 6+argon2SaltLen]
	nonce := raw[6+argon2SaltLen : fileHeaderSize]
	ciphertext := raw[fileHeaderSize:]

	key := argon2.IDKey(fk.pass, salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return fmt.Errorf("keystore: aead init: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(fileMagic))
	if err != nil {
		return fmt.Errorf("keystore: decrypt (wrong passphrase or tampered file): %w", err)
	}
	ring, err := pqcrypto.LoadFullKeyRing(plaintext)
	if err != nil {
		return fmt.Errorf("keystore: load ring: %w", err)
	}
	fk.ring = ring
	// Best-effort zeroization of the plaintext buffer.
	for i := range plaintext {
		plaintext[i] = 0
	}
	return nil
}

// persist writes the ring back to fk.path atomically (tmp + rename).
// Caller must hold fk.mu.
func (fk *FileKeystore) persist() error {
	plaintext, err := fk.ring.MarshalFull()
	if err != nil {
		return fmt.Errorf("keystore: marshal ring: %w", err)
	}
	salt := make([]byte, argon2SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("keystore: salt: %w", err)
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("keystore: nonce: %w", err)
	}
	key := argon2.IDKey(fk.pass, salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return fmt.Errorf("keystore: aead init: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, []byte(fileMagic))
	// Zeroize the plaintext copy in memory (best-effort; Go's runtime may
	// still hold copies, but we do what we can).
	for i := range plaintext {
		plaintext[i] = 0
	}

	out := make([]byte, 0, fileHeaderSize+len(ciphertext))
	out = append(out, []byte(fileMagic)...)
	out = append(out, byte(fileFormatV1>>8), byte(fileFormatV1))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)

	tmp := fk.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("keystore: write tmp: %w", err)
	}
	if err := os.Rename(tmp, fk.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("keystore: rename: %w", err)
	}
	return nil
}

func (fk *FileKeystore) Info(_ context.Context) Info {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return Info{
		Kind:           KindFile,
		Backing:        "encrypted file at " + fk.path,
		KeyCount:       len(fk.ring.List()),
		Persistent:     true,
		HardwareBacked: false,
	}
}

func (fk *FileKeystore) CreateKey(_ context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	meta, err := fk.ring.CreateKey(algo)
	if err != nil {
		return pqcrypto.KeyMeta{}, err
	}
	if err := fk.persist(); err != nil {
		return pqcrypto.KeyMeta{}, err
	}
	return meta, nil
}

func (fk *FileKeystore) RotateKey(ctx context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error) {
	return fk.CreateKey(ctx, algo)
}

func (fk *FileKeystore) ActiveKeyID(_ context.Context, algo pqcrypto.Algo) (string, bool) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return fk.ring.ActiveKeyID(algo)
}

func (fk *FileKeystore) GetMeta(_ context.Context, id string) (pqcrypto.KeyMeta, error) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return fk.ring.GetMeta(id)
}

func (fk *FileKeystore) List(_ context.Context) []pqcrypto.KeyMeta {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return fk.ring.List()
}

func (fk *FileKeystore) Sign(_ context.Context, algo pqcrypto.Algo, message []byte) ([]byte, string, error) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return fk.ring.Sign(algo, message)
}

func (fk *FileKeystore) SignWithKey(_ context.Context, id string, message []byte) ([]byte, error) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return fk.ring.SignWithKey(id, message)
}

func (fk *FileKeystore) Verify(_ context.Context, id string, message, signature []byte) (bool, error) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return fk.ring.Verify(id, message, signature)
}

func (fk *FileKeystore) MigrateSignature(_ context.Context, oldKeyID string, message, oldSig []byte, toAlgo pqcrypto.Algo) ([]byte, string, error) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	newSig, newID, err := fk.ring.MigrateSignature(oldKeyID, message, oldSig, toAlgo)
	if err != nil {
		return nil, "", err
	}
	if err := fk.persist(); err != nil {
		return nil, "", err
	}
	return newSig, newID, nil
}

// PublicSnapshot dumps the public-only view (metadata + public keys only).
// Useful for backup/audit outside the encrypted file. NOT a substitute for
// the encrypted backup itself.
func (fk *FileKeystore) PublicSnapshot() ([]byte, error) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return fk.ring.MarshalPublic()
}

// Rewrap re-encrypts the ring under a new passphrase (rotation of the
// wrapping key). Best-effort atomic via persist().
func (fk *FileKeystore) Rewrap(newPassphrase []byte) error {
	if len(newPassphrase) == 0 {
		return fmt.Errorf("%w: empty new passphrase", ErrNotConfigured)
	}
	fk.mu.Lock()
	defer fk.mu.Unlock()
	old := fk.pass
	fk.pass = append([]byte(nil), newPassphrase...)
	if err := fk.persist(); err != nil {
		fk.pass = old
		return err
	}
	// Best-effort zeroize old passphrase.
	for i := range old {
		old[i] = 0
	}
	return nil
}

// Path returns the on-disk file path (useful for diagnostics/tests).
func (fk *FileKeystore) Path() string { return fk.path }

// Compile-time interface assertion.
var _ Keystore = (*FileKeystore)(nil)

// Silence json import if refactored away.
var _ = json.Marshal
