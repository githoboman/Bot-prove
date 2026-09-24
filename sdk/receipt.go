package sdk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// This file implements the CasperProver "proof receipt" client-side
// validator. It is intentionally minimal and stdlib-only so an SDK consumer
// with just Go stdlib can verify a receipt without pulling in gnark, the
// server crypto package or a Casper RPC dependency.
//
// A "receipt" here is the JSON shape the server returns from POST
// /v1/proofs and GET /v1/proofs/{id}/export. This validator re-derives the
// content-addressed identity hashes (input_hash, output_hash, model_hash,
// proof_hash) from the raw fields and asserts they match the server-supplied
// values. It intentionally does not verify the underlying Groth16 proof;
// pairing verification requires the on-chain verifier (see engine crypto).
//
// The exact same algorithm is implemented in the Python and TS SDKs; the
// smoke tests in sdk/tests exercise bit-identical output across all three.

// ProofReceipt is the normalized view returned by VerifyReceipt. It is a
// subset of the raw server response focused on the fields we can validate
// client-side.
type ProofReceipt struct {
	ID         string `json:"id"`
	Agent      string `json:"agent,omitempty"`
	Model      string `json:"model,omitempty"`
	Input      string `json:"input,omitempty"`
	Output     string `json:"output,omitempty"`
	UseCase    string `json:"use_case,omitempty"`
	ProofHash  string `json:"proof_hash,omitempty"`
	VKHash     string `json:"vk_hash,omitempty"`
	InputHash  string `json:"input_hash,omitempty"`
	OutputHash string `json:"output_hash,omitempty"`
	ModelHash  string `json:"model_hash,omitempty"`
	Verdict    string `json:"verdict,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// ReceiptValidationError is returned when a field mismatch is detected.
type ReceiptValidationError struct {
	Field    string
	Expected string
	Actual   string
}

func (e *ReceiptValidationError) Error() string {
	return fmt.Sprintf("receipt field %q mismatch: expected %s, got %s", e.Field, e.Expected, e.Actual)
}

// VerifyReceiptBytes parses `payload` as JSON, re-derives the identity
// hashes, and returns the normalized ProofReceipt. Any hash the server
// supplied that does not re-derive to the same value yields
// *ReceiptValidationError. Missing hashes are tolerated (partial receipts).
func VerifyReceiptBytes(payload []byte) (*ProofReceipt, error) {
	var r ProofReceipt
	if err := json.Unmarshal(payload, &r); err != nil {
		return nil, fmt.Errorf("parse receipt: %w", err)
	}
	if r.ID == "" {
		return nil, errors.New("receipt missing id")
	}
	// Re-derive content-addressed hashes and cross-check.
	rederive := map[string]string{
		"input_hash":  HashField(r.Input),
		"output_hash": HashField(r.Output),
		"model_hash":  HashField(r.Model),
	}
	server := map[string]string{
		"input_hash":  r.InputHash,
		"output_hash": r.OutputHash,
		"model_hash":  r.ModelHash,
	}
	for _, field := range sortedKeys(rederive) {
		expected := rederive[field]
		got := server[field]
		if got == "" {
			// Server did not include this hash; skip.
			continue
		}
		if !hexEqualIgnoreCase(expected, got) {
			return nil, &ReceiptValidationError{Field: field, Expected: expected, Actual: got}
		}
	}
	// proof_hash and vk_hash are opaque to this validator; we cannot
	// re-derive them without the actual gnark proof + circuit. Keep them
	// on the returned struct so the caller can hand them to a full
	// verifier if desired.
	return &r, nil
}

// HashField returns the canonical hex-encoded SHA-256 of a UTF-8 string.
// This is the exact routine the server uses for input_hash / output_hash /
// model_hash content addressing.
func HashField(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// hexEqualIgnoreCase compares two hex strings after stripping a common 0x
// prefix and lowercasing. Returns false if either side has a non-hex byte.
func hexEqualIgnoreCase(a, b string) bool {
	na := strings.ToLower(strings.TrimPrefix(a, "0x"))
	nb := strings.ToLower(strings.TrimPrefix(b, "0x"))
	if len(na) != len(nb) {
		return false
	}
	for i := 0; i < len(na); i++ {
		if !isHex(na[i]) || !isHex(nb[i]) {
			return false
		}
	}
	return na == nb
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')
}

// sortedKeys returns map keys sorted alphabetically for deterministic
// iteration in tests.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
