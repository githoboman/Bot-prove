package crypto

// SLH-DSA (FIPS 205) — real hash-based post-quantum signatures via
// github.com/cloudflare/circl/sign/slhdsa (audited FIPS 205 implementation).
//
// This is the honest upgrade path for the "SPHINCS+ slot" formerly filled
// by Lamport OTS in pq.go. Lamport is a genuine but single-use hash-based
// signature; SLH-DSA is the standardised FIPS 205 stateless hash-based
// signature and is safe for repeated use with a single key. Both stay in
// the package: Lamport remains for callers that want its tiny OTS profile,
// SLH-DSA is the recommended choice for the PQ signature slot.
//
// Backlog item: 2.10 (partial finish - real SLH-DSA plugged in).
//
// SLH-DSA parameter sets exposed here map to FIPS 205 §11 named parameter
// sets. Selection guide (security category / signature size / signing speed):
//
//   SLH-DSA-SHA2-128s   cat 1   ~7.9 KB  slow    (small signature)
//   SLH-DSA-SHA2-128f   cat 1   ~17 KB   fast
//   SLH-DSA-SHA2-192s   cat 3   ~16 KB   slow
//   SLH-DSA-SHA2-192f   cat 3   ~35 KB   fast
//   SLH-DSA-SHA2-256s   cat 5   ~29 KB   slow
//   SLH-DSA-SHA2-256f   cat 5   ~49 KB   fast
//
// We expose only the four SHA-2 variants used by NIST reference tests to
// keep the surface small. Add SHAKE variants when a caller needs one.

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/cloudflare/circl/sign/slhdsa"
)

// SLHDSAParamSet names a FIPS 205 parameter set.
type SLHDSAParamSet string

const (
	SLHDSA128s SLHDSAParamSet = "slh-dsa-sha2-128s"
	SLHDSA128f SLHDSAParamSet = "slh-dsa-sha2-128f"
	SLHDSA192s SLHDSAParamSet = "slh-dsa-sha2-192s"
	SLHDSA256s SLHDSAParamSet = "slh-dsa-sha2-256s"
)

// SLHDSAKeypair holds a serialized public/private key pair for a parameter
// set. The bytes are the canonical FIPS 205 encoding.
type SLHDSAKeypair struct {
	Set     SLHDSAParamSet
	Public  []byte // canonical FIPS 205 public key
	Private []byte // canonical FIPS 205 private key
}

func paramSetToID(p SLHDSAParamSet) (slhdsa.ID, error) {
	switch p {
	case SLHDSA128s:
		return slhdsa.SHA2_128s, nil
	case SLHDSA128f:
		return slhdsa.SHA2_128f, nil
	case SLHDSA192s:
		return slhdsa.SHA2_192s, nil
	case SLHDSA256s:
		return slhdsa.SHA2_256s, nil
	default:
		return 0, fmt.Errorf("slhdsa: unknown parameter set %q", p)
	}
}

// SLHDSAKeygen generates a fresh keypair for the requested parameter set.
func SLHDSAKeygen(set SLHDSAParamSet) (*SLHDSAKeypair, error) {
	id, err := paramSetToID(set)
	if err != nil {
		return nil, err
	}
	pub, priv, err := slhdsa.GenerateKey(rand.Reader, id)
	if err != nil {
		return nil, fmt.Errorf("slhdsa: GenerateKey: %w", err)
	}
	pubB, err := pub.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("slhdsa: MarshalBinary(pub): %w", err)
	}
	privB, err := priv.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("slhdsa: MarshalBinary(priv): %w", err)
	}
	return &SLHDSAKeypair{Set: set, Public: pubB, Private: privB}, nil
}

// SLHDSASign returns a deterministic FIPS 205 signature over msg. Context
// bytes are appended per FIPS 205 §10.2 (pass nil for empty context).
func SLHDSASign(set SLHDSAParamSet, privKey, msg, context []byte) ([]byte, error) {
	id, err := paramSetToID(set)
	if err != nil {
		return nil, err
	}
	priv := slhdsa.PrivateKey{ID: id}
	if err := priv.UnmarshalBinary(privKey); err != nil {
		return nil, fmt.Errorf("slhdsa: UnmarshalBinary(priv): %w", err)
	}
	m := slhdsa.NewMessage(msg)
	sig, err := slhdsa.SignDeterministic(&priv, m, context)
	if err != nil {
		return nil, fmt.Errorf("slhdsa: SignDeterministic: %w", err)
	}
	return sig, nil
}

// SLHDSAVerify checks a FIPS 205 signature.
func SLHDSAVerify(set SLHDSAParamSet, pubKey, msg, context, sig []byte) error {
	id, err := paramSetToID(set)
	if err != nil {
		return err
	}
	pub := slhdsa.PublicKey{ID: id}
	if err := pub.UnmarshalBinary(pubKey); err != nil {
		return fmt.Errorf("slhdsa: UnmarshalBinary(pub): %w", err)
	}
	m := slhdsa.NewMessage(msg)
	if !slhdsa.Verify(&pub, m, sig, context) {
		return errors.New("slhdsa: signature does not verify")
	}
	return nil
}

// SLHDSASignatureSize returns the canonical signature length for a
// parameter set. Handy for pre-allocating buffers on wire encoders.
func SLHDSASignatureSize(set SLHDSAParamSet) (int, error) {
	// Empty keypair to interrogate size. Small overhead - runs once at boot.
	kp, err := SLHDSAKeygen(set)
	if err != nil {
		return 0, err
	}
	sig, err := SLHDSASign(set, kp.Private, []byte("probe"), nil)
	if err != nil {
		return 0, err
	}
	return len(sig), nil
}
