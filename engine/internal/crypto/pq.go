package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

// This package provides real signature primitives for the "post-quantum
// proof signing" roadmap item:
//
//   - Classic component: real Ed25519 (Go stdlib crypto/ed25519).
//   - Post-quantum component (ML-DSA): real ML-DSA-65 (FIPS 204, NIST
//     security category 3) via github.com/cloudflare/circl, an audited,
//     widely-used PQC library. Sign/Verify here call straight into circl -
//     there is no simulation left in this path.
//   - Post-quantum component (hash-based, "SPHINCS+ slot"): historically a
//     genuine Lamport one-time signature (Lamport 1979) - real crypto, but
//     single-use per key pair.  circl v1.6.4 now ships a FIPS 205 SLH-DSA
//     implementation, and the recommended PQ hash-based slot is now the
//     SLH-DSA wrapper in slhdsa.go (safe for repeated use with one key).
//     Lamport stays in this file for callers that specifically want its
//     tiny OTS profile; new code SHOULD prefer SLHDSAKeygen / SLHDSASign /
//     SLHDSAVerify.
//
// Prior versions of this file used a made-up scheme (public key = SHA-256
// of the private key, "signature" = SHA-256(part of the signing key ||
// message hash)) where Verify hashed the *public* key instead of the
// private key. Since the public key is not a prefix of the private key,
// Verify(pub, msg, Sign(priv, msg)) never returned true for two independently
// generated keys - the scheme could not actually verify a signature it had
// just produced. Every code path calling both Sign and Verify with real,
// distinct key pairs was untested; this file replaces that scheme.

var (
	errInvalidKeyLength = errors.New("invalid key length")
	errInvalidSignature = errors.New("invalid signature format or length")
)

// ---------------------------------------------------------------------------
// Classic component: Ed25519 (real, stdlib)
// ---------------------------------------------------------------------------

// GenerateEd25519KeyPair generates a real Ed25519 key pair.
func GenerateEd25519KeyPair() (priv ed25519.PrivateKey, pub ed25519.PublicKey, err error) {
	pub, priv, err = ed25519.GenerateKey(rand.Reader)
	return priv, pub, err
}

// SignClassic signs message with a real Ed25519 private key.
func SignClassic(privateKey ed25519.PrivateKey, message []byte) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errInvalidKeyLength
	}
	return ed25519.Sign(privateKey, message), nil
}

// VerifyClassic verifies a real Ed25519 signature.
func VerifyClassic(publicKey ed25519.PublicKey, message, signature []byte) (bool, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return false, errInvalidKeyLength
	}
	if len(signature) != ed25519.SignatureSize {
		return false, errInvalidSignature
	}
	return ed25519.Verify(publicKey, message, signature), nil
}

// ---------------------------------------------------------------------------
// Post-quantum component: ML-DSA-65 (real, FIPS 204, via circl)
// ---------------------------------------------------------------------------

// GenerateMLDSAKeyPair generates a real ML-DSA-65 key pair.
func GenerateMLDSAKeyPair() (priv *mldsa65.PrivateKey, pub *mldsa65.PublicKey, err error) {
	pub, priv, err = mldsa65.GenerateKey(rand.Reader)
	return priv, pub, err
}

// SignMLDSA signs message with a real ML-DSA-65 private key.
func SignMLDSA(privateKey *mldsa65.PrivateKey, message []byte) ([]byte, error) {
	if privateKey == nil {
		return nil, errInvalidKeyLength
	}
	sig := make([]byte, mldsa65.SignatureSize)
	if err := mldsa65.SignTo(privateKey, message, nil, false, sig); err != nil {
		return nil, fmt.Errorf("mldsa65 sign: %w", err)
	}
	return sig, nil
}

// VerifyMLDSA verifies a real ML-DSA-65 signature.
func VerifyMLDSA(publicKey *mldsa65.PublicKey, message, signature []byte) (bool, error) {
	if publicKey == nil {
		return false, errInvalidKeyLength
	}
	if len(signature) != mldsa65.SignatureSize {
		return false, errInvalidSignature
	}
	return mldsa65.Verify(publicKey, message, nil, signature), nil
}

// ---------------------------------------------------------------------------
// Hash-based one-time signature ("SPHINCS+ slot"): real Lamport OTS
// ---------------------------------------------------------------------------
//
// This is a genuine Lamport one-time signature over a SHA-256 digest of the
// message: the private key is 256 pairs of random 32-byte preimages (one
// pair per digest bit), the public key is the SHA-256 hash of every
// preimage, and the signature reveals one preimage per bit of the digest
// (chosen by the bit's value). Verification recomputes each revealed
// preimage's hash and checks it against the corresponding public key entry.
// This is real, working crypto - not a simulation - but it is a one-time
// scheme: signing a second message with the same key pair leaks half of
// each pair and breaks security. Callers must generate a fresh key pair
// per signature.

const lamportDigestBits = sha256.Size * 8 // 256

// LamportPrivateKey holds 256 pairs of 32-byte preimages.
type LamportPrivateKey [lamportDigestBits][2][sha256.Size]byte

// LamportPublicKey holds SHA-256(preimage) for each entry of the private key.
type LamportPublicKey [lamportDigestBits][2][sha256.Size]byte

const lamportKeySize = lamportDigestBits * 2 * sha256.Size // 16384 bytes

// Bytes flattens the private key into a byte slice for transport/storage.
func (priv *LamportPrivateKey) Bytes() []byte {
	out := make([]byte, 0, lamportKeySize)
	for i := 0; i < lamportDigestBits; i++ {
		out = append(out, priv[i][0][:]...)
		out = append(out, priv[i][1][:]...)
	}
	return out
}

// LamportPrivateKeyFromBytes reconstructs a private key produced by Bytes().
func LamportPrivateKeyFromBytes(b []byte) (*LamportPrivateKey, error) {
	if len(b) != lamportKeySize {
		return nil, errInvalidKeyLength
	}
	var priv LamportPrivateKey
	for i := 0; i < lamportDigestBits; i++ {
		copy(priv[i][0][:], b[i*2*sha256.Size:i*2*sha256.Size+sha256.Size])
		copy(priv[i][1][:], b[i*2*sha256.Size+sha256.Size:i*2*sha256.Size+2*sha256.Size])
	}
	return &priv, nil
}

// Bytes flattens the public key into a byte slice for transport/storage.
func (pub *LamportPublicKey) Bytes() []byte {
	out := make([]byte, 0, lamportKeySize)
	for i := 0; i < lamportDigestBits; i++ {
		out = append(out, pub[i][0][:]...)
		out = append(out, pub[i][1][:]...)
	}
	return out
}

// LamportPublicKeyFromBytes reconstructs a public key produced by Bytes().
func LamportPublicKeyFromBytes(b []byte) (*LamportPublicKey, error) {
	if len(b) != lamportKeySize {
		return nil, errInvalidKeyLength
	}
	var pub LamportPublicKey
	for i := 0; i < lamportDigestBits; i++ {
		copy(pub[i][0][:], b[i*2*sha256.Size:i*2*sha256.Size+sha256.Size])
		copy(pub[i][1][:], b[i*2*sha256.Size+sha256.Size:i*2*sha256.Size+2*sha256.Size])
	}
	return &pub, nil
}

// GenerateLamportKeyPair generates a fresh, single-use Lamport OTS key pair.
func GenerateLamportKeyPair() (*LamportPrivateKey, *LamportPublicKey, error) {
	var priv LamportPrivateKey
	var pub LamportPublicKey
	for i := 0; i < lamportDigestBits; i++ {
		for b := 0; b < 2; b++ {
			if _, err := io.ReadFull(rand.Reader, priv[i][b][:]); err != nil {
				return nil, nil, fmt.Errorf("failed to generate lamport key material: %w", err)
			}
			pub[i][b] = sha256.Sum256(priv[i][b][:])
		}
	}
	return &priv, &pub, nil
}

// SignSPHINCS produces a real Lamport one-time signature over message.
// The returned signature is 256*32 = 8192 bytes. See package doc for why
// this stands in for SPHINCS+ rather than being a real SPHINCS+ signature.
func SignSPHINCS(privateKey *LamportPrivateKey, message []byte) ([]byte, error) {
	if privateKey == nil {
		return nil, errInvalidKeyLength
	}
	digest := sha256.Sum256(message)
	sig := make([]byte, 0, lamportDigestBits*sha256.Size)
	for i := 0; i < lamportDigestBits; i++ {
		bit := (digest[i/8] >> (7 - uint(i%8))) & 1
		sig = append(sig, privateKey[i][bit][:]...)
	}
	slog.Default().Debug("signed with lamport OTS (SPHINCS+ slot)", "sig_len", len(sig))
	return sig, nil
}

// VerifySPHINCS verifies a real Lamport one-time signature over message.
func VerifySPHINCS(publicKey *LamportPublicKey, message, signature []byte) (bool, error) {
	if publicKey == nil {
		return false, errInvalidKeyLength
	}
	if len(signature) != lamportDigestBits*sha256.Size {
		return false, errInvalidSignature
	}
	digest := sha256.Sum256(message)
	for i := 0; i < lamportDigestBits; i++ {
		bit := (digest[i/8] >> (7 - uint(i%8))) & 1
		preimage := signature[i*sha256.Size : (i+1)*sha256.Size]
		h := sha256.Sum256(preimage)
		if !bytes.Equal(h[:], publicKey[i][bit][:]) {
			return false, nil
		}
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Hybrid signature: real Ed25519 + real ML-DSA-65
// ---------------------------------------------------------------------------

// HybridSignature combines a classic and a post-quantum signature. Both
// must independently verify for the hybrid signature to be considered valid.
type HybridSignature struct {
	ClassicSig []byte
	PQSig      []byte
}

// MarshalBinary encodes the HybridSignature as [1-byte version][2-byte
// classic len][classic sig][2-byte pq len][pq sig].
func (hs *HybridSignature) MarshalBinary() ([]byte, error) {
	if len(hs.ClassicSig) > 0xFFFF || len(hs.PQSig) > 0xFFFF {
		return nil, errInvalidSignature
	}
	var buf bytes.Buffer
	buf.WriteByte(0x01)
	buf.WriteByte(byte(len(hs.ClassicSig) >> 8))
	buf.WriteByte(byte(len(hs.ClassicSig)))
	buf.Write(hs.ClassicSig)
	buf.WriteByte(byte(len(hs.PQSig) >> 8))
	buf.WriteByte(byte(len(hs.PQSig)))
	buf.Write(hs.PQSig)
	return buf.Bytes(), nil
}

// UnmarshalBinary decodes a byte slice produced by MarshalBinary.
func (hs *HybridSignature) UnmarshalBinary(data []byte) error {
	if len(data) < 5 {
		return errInvalidSignature
	}
	data = data[1:] // version byte
	classicLen := int(data[0])<<8 | int(data[1])
	data = data[2:]
	if len(data) < classicLen {
		return errInvalidSignature
	}
	hs.ClassicSig = data[:classicLen]
	data = data[classicLen:]

	if len(data) < 2 {
		return errInvalidSignature
	}
	pqLen := int(data[0])<<8 | int(data[1])
	data = data[2:]
	if len(data) < pqLen {
		return errInvalidSignature
	}
	hs.PQSig = data[:pqLen]
	return nil
}

// HybridSign produces a hybrid signature: real Ed25519 + real ML-DSA-65.
func HybridSign(classicPriv ed25519.PrivateKey, pqPriv *mldsa65.PrivateKey, message []byte) ([]byte, error) {
	classicSig, err := SignClassic(classicPriv, message)
	if err != nil {
		return nil, fmt.Errorf("classic signature failed: %w", err)
	}
	pqSig, err := SignMLDSA(pqPriv, message)
	if err != nil {
		return nil, fmt.Errorf("pq signature failed: %w", err)
	}
	hs := &HybridSignature{ClassicSig: classicSig, PQSig: pqSig}
	return hs.MarshalBinary()
}

// HybridVerify verifies a hybrid signature and reports both the overall
// result and each component's individual validity. The overall signature is
// valid only if both components are valid.
func HybridVerify(classicPub ed25519.PublicKey, pqPub *mldsa65.PublicKey, message, hybridSigBytes []byte) (valid, classicValid, pqValid bool, err error) {
	hs := &HybridSignature{}
	if err := hs.UnmarshalBinary(hybridSigBytes); err != nil {
		return false, false, false, fmt.Errorf("failed to unmarshal hybrid signature: %w", err)
	}

	classicValid, err = VerifyClassic(classicPub, message, hs.ClassicSig)
	if err != nil {
		return false, false, false, fmt.Errorf("classic signature verification failed: %w", err)
	}

	pqValid, err = VerifyMLDSA(pqPub, message, hs.PQSig)
	if err != nil {
		return false, classicValid, false, fmt.Errorf("pq signature verification failed: %w", err)
	}

	return classicValid && pqValid, classicValid, pqValid, nil
}
