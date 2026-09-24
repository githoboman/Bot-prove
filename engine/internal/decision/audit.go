// Package decision — auditable decision logging.
//
// Backlog 3.2: every agent decision produces an immutable audit record
// containing enough hashed evidence for a third party to reconstruct
// what the agent saw and what it decided, WITHOUT leaking the raw
// user prompt, PII, or provider payloads.
//
// The layer is deliberately storage-agnostic: an in-memory ring buffer
// is provided for hackathon runs. Wire a real sink (SQLite / Postgres /
// object store) via the Sink interface for production.
//
// Redaction contract:
//   - Raw prompt, raw response, and any keys inside metadata are hashed
//     (SHA-256, hex) but never persisted verbatim.
//   - A short "trace preview" (first N runes) is kept ONLY when the
//     record's PreviewOptIn is true. Default: false. This lets a
//     replayer prove hash match without ever storing PII.
//   - Metadata keys matching the redaction list are replaced with
//     "<redacted:sha256:XXXX...>" so a query can still match on the
//     hash without exposing the value.

package decision

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Verdict is the discrete outcome of a decision.
type Verdict string

const (
	VerdictAllow    Verdict = "allow"
	VerdictAbstain  Verdict = "abstain"
	VerdictReject   Verdict = "reject"
	VerdictHITL     Verdict = "hitl_required"
	VerdictMalicious Verdict = "malicious"
)

// Record is the immutable audit entry. Every field is either a hash or a
// low-cardinality label safe to persist. Timestamps are UTC.
type Record struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	AgentID         string            `json:"agent_id"`
	ModelID         string            `json:"model_id"`
	ModelVersion    string            `json:"model_version"`
	RequestHash     string            `json:"request_hash"`     // sha256 of raw prompt
	ResponseHash    string            `json:"response_hash"`    // sha256 of raw response
	InputBytes      int               `json:"input_bytes"`
	OutputBytes     int               `json:"output_bytes"`
	Verdict         Verdict           `json:"verdict"`
	RiskTier        string            `json:"risk_tier"`        // low | medium | high
	PolicyID        string            `json:"policy_id"`
	Metadata        map[string]string `json:"metadata,omitempty"` // redacted where flagged
	TracePreview    string            `json:"trace_preview,omitempty"`
	PreviewOptIn    bool              `json:"preview_opt_in"`
	ParentRecordID  string            `json:"parent_record_id,omitempty"`
	ChainRootHash   string            `json:"chain_root_hash"`  // sha256 of everything above, for tamper-evidence
}

// Sink accepts records. Implementations MUST be safe for concurrent use.
type Sink interface {
	Append(rec Record) error
	Get(id string) (Record, bool, error)
	Lineage(id string, maxDepth int) ([]Record, error)
	Recent(limit int) ([]Record, error)
}

// InMemorySink is a ring buffer for hackathon runs. Not persistent.
type InMemorySink struct {
	mu      sync.RWMutex
	records map[string]Record
	order   []string
	max     int
}

// NewInMemorySink returns a sink retaining the last `max` records.
func NewInMemorySink(max int) *InMemorySink {
	if max <= 0 {
		max = 4096
	}
	return &InMemorySink{records: map[string]Record{}, max: max}
}

func (s *InMemorySink) Append(rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.records[rec.ID]; dup {
		return fmt.Errorf("duplicate record id %q", rec.ID)
	}
	s.records[rec.ID] = rec
	s.order = append(s.order, rec.ID)
	if len(s.order) > s.max {
		drop := s.order[0]
		s.order = s.order[1:]
		delete(s.records, drop)
	}
	return nil
}

func (s *InMemorySink) Get(id string) (Record, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[id]
	return r, ok, nil
}

// Lineage walks parent pointers up to maxDepth. Root first.
func (s *InMemorySink) Lineage(id string, maxDepth int) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if maxDepth <= 0 {
		maxDepth = 32
	}
	var chain []Record
	cursor := id
	for i := 0; i < maxDepth && cursor != ""; i++ {
		r, ok := s.records[cursor]
		if !ok {
			break
		}
		chain = append([]Record{r}, chain...)
		cursor = r.ParentRecordID
	}
	return chain, nil
}

func (s *InMemorySink) Recent(limit int) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	out := make([]Record, 0, limit)
	// newest first
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.records[s.order[i]])
	}
	return out, nil
}

// defaultRedactedKeys is applied when caller does not override.
var defaultRedactedKeys = map[string]bool{
	"api_key":       true,
	"authorization": true,
	"cookie":        true,
	"password":      true,
	"secret":        true,
	"token":         true,
	"pii":           true,
	"email":         true,
	"phone":         true,
	"ssn":           true,
}

// BuildRecord produces a fully-hashed, redacted Record. It does NOT
// persist anything — pass the result to a Sink.
//
//	agent, model, modelVersion — low-cardinality labels
//	rawRequest, rawResponse    — MAY contain PII; only hashes escape
//	metadata                   — redaction applied per defaultRedactedKeys
//	verdict, riskTier, policyID — low-cardinality
//	parentID                   — chain pointer (empty for root)
//	previewOptIn=true          — persist first 128 runes of request; caller MUST have consent
func BuildRecord(
	id, agent, model, modelVersion string,
	rawRequest, rawResponse []byte,
	metadata map[string]string,
	verdict Verdict, riskTier, policyID, parentID string,
	previewOptIn bool,
) Record {
	rec := Record{
		ID:             id,
		Timestamp:      time.Now().UTC(),
		AgentID:        agent,
		ModelID:        model,
		ModelVersion:   modelVersion,
		RequestHash:    sha256Hex(rawRequest),
		ResponseHash:   sha256Hex(rawResponse),
		InputBytes:     len(rawRequest),
		OutputBytes:    len(rawResponse),
		Verdict:        verdict,
		RiskTier:       riskTier,
		PolicyID:       policyID,
		Metadata:       redactMetadata(metadata),
		PreviewOptIn:   previewOptIn,
		ParentRecordID: parentID,
	}
	if previewOptIn {
		rec.TracePreview = truncateRunes(string(rawRequest), 128)
	}
	rec.ChainRootHash = chainRoot(rec)
	return rec
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\u2026"
}

// redactMetadata replaces sensitive values with a hash marker so the
// map's shape survives serialization but the value doesn't.
func redactMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		redact := defaultRedactedKeys[lk]
		if !redact {
			for keyword := range defaultRedactedKeys {
				if strings.Contains(lk, keyword) {
					redact = true
					break
				}
			}
		}
		if redact {
			out[k] = "<redacted:sha256:" + sha256Hex([]byte(v))[:16] + ">"
			continue
		}
		out[k] = v
	}
	return out
}

// chainRoot re-hashes the record's canonical fields for tamper-evidence.
// Excludes the ChainRootHash itself.
func chainRoot(r Record) string {
	shadow := r
	shadow.ChainRootHash = ""
	buf, _ := json.Marshal(shadow)
	return sha256Hex(buf)
}

// VerifyRecord recomputes the chain root and returns true iff it matches.
func VerifyRecord(r Record) bool {
	return chainRoot(r) == r.ChainRootHash
}
