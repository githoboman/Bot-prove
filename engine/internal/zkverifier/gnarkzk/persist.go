// Package gnarkzk — on-disk persistence of ccs + pk + vk per circuit.
//
// Disk layout under a caller-provided base dir (e.g. CP_ZK_KEYS_DIR):
//
//	<base>/<circuit_id>/manifest.json   (backend, curve, sha256 digests)
//	<base>/<circuit_id>/circuit.ccs     (compiled R1CS)
//	<base>/<circuit_id>/proving.key
//	<base>/<circuit_id>/verifying.key
//
// Each artifact is written atomically (temp file + rename) so a crash
// mid-write leaves the previous snapshot intact. LoadOrCreate is the
// entry point Server bootstrap uses: try Load, and on ENOENT (or explicit
// force-regenerate) fall back to Compile + Save.
package gnarkzk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

const (
	manifestFile = "manifest.json"
	ccsFile      = "circuit.ccs"
	pkFile       = "proving.key"
	vkFile       = "verifying.key"
)

// KeyManifest records the artifacts stored on disk for one circuit.
type KeyManifest struct {
	CircuitID  string    `json:"circuit_id"`
	Version    string    `json:"version"`
	Backend    string    `json:"backend"`
	Curve      string    `json:"curve"`
	CreatedAt  time.Time `json:"created_at"`
	Constraints int      `json:"constraints"`
	CCSDigest  string    `json:"ccs_sha256"`
	PKDigest   string    `json:"pk_sha256"`
	VKDigest   string    `json:"vk_sha256"`
	// Warning is a plain-language reminder that this is a session-local /
	// developer setup, not a multi-party trusted setup ceremony. Written
	// into every manifest so we can't quietly ship an audit-relevant
	// artifact that pretends otherwise.
	Warning string `json:"warning"`
}

// LoadOrCreate first tries to load persisted artifacts for the given
// circuit_id from base. If any required file is missing (or forceRegenerate
// is true), it runs Compile + Save. In all cases the registry's compiled
// slot for id is populated on success.
func (r *Registry) LoadOrCreate(id, base string, forceRegenerate bool) (*KeyManifest, error) {
	r.mu.RLock()
	c, ok := r.circuits[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("circuit %q not registered", id)
	}

	dir := filepath.Join(base, id)
	manifestPath := filepath.Join(dir, manifestFile)

	if !forceRegenerate {
		if manifest, err := loadManifest(manifestPath); err == nil {
			// try to load ccs+pk+vk
			ccs, ccsDigest, err := loadCCS(filepath.Join(dir, ccsFile))
			if err == nil {
				pk, pkDigest, err2 := loadPK(filepath.Join(dir, pkFile))
				if err2 == nil {
					vk, vkDigest, err3 := loadVK(filepath.Join(dir, vkFile))
					if err3 == nil {
						// digest sanity check
						if manifest.CCSDigest != ccsDigest || manifest.PKDigest != pkDigest || manifest.VKDigest != vkDigest {
							slog.Warn("gnarkzk: manifest digest mismatch, regenerating", "circuit", id, "dir", dir)
						} else {
							r.mu.Lock()
							c.ccs = ccs
							c.pk = pk
							c.vk = vk
							c.descriptor.Constraints = ccs.GetNbConstraints()
							c.descriptor.KeyDigest = vkDigest
							r.mu.Unlock()
							return manifest, nil
						}
					}
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("gnarkzk: manifest load failed, regenerating", "circuit", id, "error", err)
		}
	}

	// (re)generate
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	inst := c.circuit.NewCircuit()
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, inst)
	if err != nil {
		return nil, fmt.Errorf("compile %q: %w", id, err)
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return nil, fmt.Errorf("setup %q: %w", id, err)
	}

	ccsDigest, err := saveDigest(filepath.Join(dir, ccsFile), ccs)
	if err != nil {
		return nil, err
	}
	pkDigest, err := saveDigest(filepath.Join(dir, pkFile), pk)
	if err != nil {
		return nil, err
	}
	vkDigest, err := saveDigest(filepath.Join(dir, vkFile), vk)
	if err != nil {
		return nil, err
	}

	manifest := &KeyManifest{
		CircuitID:   id,
		Version:     c.descriptor.Version,
		Backend:     "groth16",
		Curve:       "BN254",
		CreatedAt:   time.Now().UTC(),
		Constraints: ccs.GetNbConstraints(),
		CCSDigest:   ccsDigest,
		PKDigest:    pkDigest,
		VKDigest:    vkDigest,
		Warning:     "session-local / developer trusted setup — NOT a production MPC ceremony. See docs/roadmap/CEREMONY.md.",
	}
	if err := saveJSONAtomic(manifestPath, manifest); err != nil {
		return nil, err
	}

	r.mu.Lock()
	c.ccs = ccs
	c.pk = pk
	c.vk = vk
	c.descriptor.Constraints = ccs.GetNbConstraints()
	c.descriptor.KeyDigest = vkDigest
	r.mu.Unlock()

	return manifest, nil
}

// LoadManifest returns the on-disk manifest for a circuit id under base,
// without touching the registry state. Handy for /v1/circuits/{id}/manifest
// endpoints.
func LoadManifest(id, base string) (*KeyManifest, error) {
	return loadManifest(filepath.Join(base, id, manifestFile))
}

// --- io helpers ------------------------------------------------------------

type writerTo interface {
	WriteTo(io.Writer) (int64, error)
}

func saveDigest(path string, obj writerTo) (string, error) {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("open tmp %s: %w", tmp, err)
	}
	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)
	if _, err := obj.WriteTo(mw); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("sync %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("rename %s→%s: %w", tmp, path, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func loadCCS(path string) (constraint.ConstraintSystem, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	ccs := groth16.NewCS(ecc.BN254)
	buf := &byteReader{b: data}
	if _, err := ccs.ReadFrom(buf); err != nil {
		return nil, "", fmt.Errorf("read ccs: %w", err)
	}
	return ccs, hexSHA(data), nil
}

func loadPK(path string) (groth16.ProvingKey, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	pk := groth16.NewProvingKey(ecc.BN254)
	if _, err := pk.ReadFrom(&byteReader{b: data}); err != nil {
		return nil, "", fmt.Errorf("read pk: %w", err)
	}
	return pk, hexSHA(data), nil
}

func loadVK(path string) (groth16.VerifyingKey, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	vk := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := vk.ReadFrom(&byteReader{b: data}); err != nil {
		return nil, "", fmt.Errorf("read vk: %w", err)
	}
	return vk, hexSHA(data), nil
}

func loadManifest(path string) (*KeyManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m KeyManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return &m, nil
}

func saveJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// digestVK computes the sha256 of vk.WriteTo bytes — used to fill
// Descriptor.KeyDigest for callers that haven't (or won't) persist.
func digestVK(vk groth16.VerifyingKey) string {
	hasher := sha256.New()
	if _, err := vk.WriteTo(hasher); err != nil {
		return ""
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func hexSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// byteReader is a tiny io.Reader over a []byte so we don't need to import
// bytes just for this.
type byteReader struct {
	b []byte
	i int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
