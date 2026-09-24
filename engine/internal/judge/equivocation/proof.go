// Package equivocation emits on-chain-verifiable evidence when a judge task
// resolves to DISAGREE.
//
// The evidence is a canonical, byte-deterministic Proof that pins down:
//
//   - the task input & system message that were graded,
//   - every provider's vote (including raw content, latency, error),
//   - which facets disagreed and how the votes split,
//   - a keyed hash a slashing contract can recompute from on-chain state.
//
// The design goal is that a smart contract with only the Proof bytes + the
// aggregation rule can independently reach the same DISAGREE verdict. No
// trust in the aggregator, no re-running LLMs — the Proof is self-contained
// evidence of equivocation.
//
// The Proof is NOT signed here. Signing (ed25519 by the operator, plus the
// PQ ML-DSA co-signature Gate 4 will add) happens one layer up, over the
// canonical bytes returned by MarshalCanonical. That keeps this package
// crypto-agnostic and easy to fuzz.
package equivocation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
)

// Proof is the canonical evidence blob for one DISAGREE task result.
//
// Field ordering, map key ordering, and slice ordering are all enforced by
// MarshalCanonical — do NOT rely on Go's encoding/json field ordering.
type Proof struct {
	// Version pins the serialization format. Bump on any breaking change.
	Version int `json:"version"`

	// TaskID mirrors judge.Task.ID.
	TaskID string `json:"task_id"`

	// Input is the raw text that was graded.
	Input string `json:"input"`

	// SystemMsg is the exact system prompt the judge sent.
	SystemMsg string `json:"system_msg"`

	// Facets is the ordered list of per-facet evidence. Sort key is FacetID.
	Facets []FacetEvidence `json:"facets"`

	// StartedAt / CompletedAt are RFC3339-nano timestamps for audit.
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`

	// OverallVerdict is always "DISAGREE" for an emitted proof.
	// (AGREE/ABSTAIN task results never surface here.)
	OverallVerdict string `json:"overall_verdict"`

	// DigestHex is sha256(canonical bytes with digest_hex omitted). Consumers
	// verify by re-canonicalizing the Proof with DigestHex cleared and
	// hashing.
	DigestHex string `json:"digest_hex"`
}

// FacetEvidence captures one facet's votes.
type FacetEvidence struct {
	FacetID           string  `json:"facet_id"`
	Verdict           string  `json:"verdict"`
	Winner            string  `json:"winner,omitempty"`
	LiveCount         int     `json:"live_count"`
	AgreementFraction float64 `json:"agreement_fraction"`
	Votes             []Vote  `json:"votes"`
}

// Vote captures one provider's answer. Provider IDs are sorted for
// deterministic ordering.
type Vote struct {
	ProviderID string `json:"provider_id"`
	Value      string `json:"value"`
	Raw        string `json:"raw"`
	LatencyMs  int64  `json:"latency_ms"`
	Err        string `json:"err,omitempty"`
}

// FromTaskResult builds a Proof from a judge.TaskResult. Returns an error if
// the result's OverallVerdict is not DISAGREE — proofs are only emitted for
// equivocation, not for AGREE or ABSTAIN.
func FromTaskResult(task *judge.Task, result *judge.TaskResult) (*Proof, error) {
	if result == nil {
		return nil, errors.New("equivocation: nil result")
	}
	if task == nil {
		return nil, errors.New("equivocation: nil task")
	}
	if result.OverallVerdict != judge.VerdictDisagree {
		return nil, fmt.Errorf("equivocation: refuse to emit proof for verdict=%s (only DISAGREE)", result.OverallVerdict)
	}

	facetIDs := make([]string, 0, len(result.Facets))
	for id := range result.Facets {
		facetIDs = append(facetIDs, id)
	}
	sort.Strings(facetIDs)

	evidences := make([]FacetEvidence, 0, len(facetIDs))
	for _, fid := range facetIDs {
		fr := result.Facets[fid]
		if fr == nil {
			continue
		}
		votes := make([]Vote, 0, len(fr.Votes))
		for _, v := range fr.Votes {
			votes = append(votes, Vote{
				ProviderID: v.ProviderID,
				Value:      v.Value,
				Raw:        v.Raw,
				LatencyMs:  v.Latency.Milliseconds(),
				Err:        v.Err,
			})
		}
		sort.Slice(votes, func(i, j int) bool { return votes[i].ProviderID < votes[j].ProviderID })

		evidences = append(evidences, FacetEvidence{
			FacetID:           fr.FacetID,
			Verdict:           string(fr.Verdict),
			Winner:            fr.Winner,
			LiveCount:         fr.LiveCount,
			AgreementFraction: fr.AgreementFraction,
			Votes:             votes,
		})
	}

	p := &Proof{
		Version:        1,
		TaskID:         task.ID,
		Input:          task.Input,
		SystemMsg:      task.SystemMsg,
		Facets:         evidences,
		StartedAt:      formatTime(result.StartedAt),
		CompletedAt:    formatTime(result.CompletedAt),
		OverallVerdict: string(result.OverallVerdict),
	}

	// Digest = sha256 over the canonical bytes with DigestHex zeroed.
	digest, err := p.digest()
	if err != nil {
		return nil, fmt.Errorf("equivocation: digest failure: %w", err)
	}
	p.DigestHex = digest
	return p, nil
}

// MarshalCanonical serializes the Proof to canonical JSON bytes: map keys are
// alphabetized, no extra whitespace, no HTML escaping. Two Proofs with the
// same content produce byte-identical outputs regardless of Go runtime.
func (p *Proof) MarshalCanonical() ([]byte, error) {
	// Round-trip through map[string]any so key order comes out sorted by
	// encoding/json (which sorts map keys). Struct field order is not
	// alphabetical, so we cannot rely on struct marshaling alone.
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return nil, err
	}
	return canonicalMarshal(asMap)
}

// Verify re-derives the digest and compares to Proof.DigestHex. Returns nil
// on match, error otherwise. Consumers on-chain do the same check byte-for-
// byte.
func (p *Proof) Verify() error {
	want := p.DigestHex
	got, err := p.digest()
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("equivocation: digest mismatch: got %s, want %s", got, want)
	}
	return nil
}

// digest computes sha256 over the canonical bytes with DigestHex cleared.
func (p *Proof) digest() (string, error) {
	tmp := *p
	tmp.DigestHex = ""
	b, err := tmp.MarshalCanonical()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// canonicalMarshal serializes v with alphabetical map key order, no HTML
// escaping, and no extra whitespace. Handles nested map[string]any and
// []any recursively.
func canonicalMarshal(v any) ([]byte, error) {
	buf, err := canonicalEncode(v)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func canonicalEncode(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				out = append(out, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			out = append(out, kb...)
			out = append(out, ':')
			vb, err := canonicalEncode(t[k])
			if err != nil {
				return nil, err
			}
			out = append(out, vb...)
		}
		out = append(out, '}')
		return out, nil
	case []any:
		out := []byte{'['}
		for i, e := range t {
			if i > 0 {
				out = append(out, ',')
			}
			eb, err := canonicalEncode(e)
			if err != nil {
				return nil, err
			}
			out = append(out, eb...)
		}
		out = append(out, ']')
		return out, nil
	default:
		return json.Marshal(v)
	}
}

// formatTime returns an RFC3339Nano-formatted timestamp in UTC, or "" for the
// zero time. Zero times must serialize to "" so on-chain verifiers don't need
// to special-case the Go zero value.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
