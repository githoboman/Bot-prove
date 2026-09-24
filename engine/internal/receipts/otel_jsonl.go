package receipts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JSONLSink writes each receipt as one JSON object per line to a file.
//
// It is NOT an OpenTelemetry SDK: an OTel collector cannot read JSONL
// natively. What it IS: an on-disk audit trail with the same
// attribute names an OTel-native sink would use, so a deployment can
// point Vector / Fluent Bit at the file and forward to any OTel
// receiver. See docs/PROVENANCE_LINEAGE.md → "OTel bridging" for the
// deployment pattern.
//
// The sink is thread-safe: Record grabs a mutex around Write. It
// creates parent directories on first Record; a missing directory is
// created, a permission error is returned to the caller (Emit will
// bubble it up).
type JSONLSink struct {
	path    string
	mu      sync.Mutex
	writer  io.WriteCloser
	timeout time.Duration
}

// NewJSONLSink returns a sink that appends to path. Path is created on
// first Record; parent directories are created eagerly here so a
// misconfigured deployment surfaces the error before the first
// decision.
func NewJSONLSink(path string) (*JSONLSink, error) {
	if path == "" {
		return nil, fmt.Errorf("receipts: JSONLSink requires non-empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("receipts: JSONLSink mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("receipts: JSONLSink open: %w", err)
	}
	return &JSONLSink{path: path, writer: f, timeout: 5 * time.Second}, nil
}

// Record appends a single JSON line for r. The line shape is the OTel
// span attribute map — one attribute per top-level receipt field. A
// collector-side test in docs/PROVENANCE_LINEAGE.md walks the mapping.
func (s *JSONLSink) Record(_ context.Context, r DecisionReceipt) error {
	attrs := s.spanAttrs(r)
	b, err := json.Marshal(attrs)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.writer.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("receipts: JSONLSink write: %w", err)
	}
	return nil
}

// Close releases the underlying file.
func (s *JSONLSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return nil
	}
	err := s.writer.Close()
	s.writer = nil
	return err
}

func (s *JSONLSink) spanAttrs(r DecisionReceipt) map[string]any {
	attrs := map[string]any{
		"trace.name":         "cp.decision",
		"cp.receipt_id":      r.ID,
		"cp.receipt_hash":    canonicalHashUnsigned(r),
		"cp.issued_at":       r.IssuedAt.Format(time.RFC3339Nano),
		"cp.issuer":          r.Issuer,
		"cp.subject":         r.Subject,
		"cp.spec_id":         r.SpecID,
		"cp.verdict":         string(r.Aggregate),
		"cp.confidence":      r.Confidence,
		"cp.facet_count":     len(r.Facets),
		"cp.provider_count":  len(r.ProviderReceipts),
		"cp.evidence_root":   r.EvidenceRoot,
		"cp.model_id":        r.ModelID,
		"cp.hitl":            r.HITL != nil,
	}
	if r.VetoedBy != "" {
		attrs["cp.vetoed_by"] = r.VetoedBy
	}
	if r.HITL != nil {
		attrs["cp.hitl_action"] = r.HITL.Action
		attrs["cp.hitl_ticket"] = r.HITL.TicketID
		if r.HITL.Reviewer != "" {
			attrs["cp.hitl_reviewer"] = r.HITL.Reviewer
		}
	}
	return attrs
}

func canonicalHashUnsigned(r DecisionReceipt) string {
	unsigned := r
	unsigned.Proof = nil
	return CanonicalHash(unsigned)
}
