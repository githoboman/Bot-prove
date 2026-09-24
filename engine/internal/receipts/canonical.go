package receipts

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"
	"sort"
)

// CanonicalHash returns the sha256-hex of the length-prefixed,
// sort-normalised canonical serialisation of r. This is the digest the
// Proof signs. It is DETERMINISTIC and INDEPENDENT of the JSON encoder's
// key order, whitespace, and numeric formatting — a downstream that
// re-hashes the receipt from any of the three shapes (internal, W3C-VC,
// agent-receipt) MUST arrive at the same value.
//
// The Proof field is deliberately excluded from the hash: the whole
// point of the digest is to bind the receipt before the signature is
// computed, and including a self-reference would be circular.
func CanonicalHash(r DecisionReceipt) string {
	h := sha256.New()

	writeLP(h, []byte(r.ID))
	writeLP(h, []byte(r.IssuedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")))
	writeLP(h, []byte(r.Issuer))
	writeLP(h, []byte(r.Subject))
	writeLP(h, []byte(r.SpecID))
	writeLP(h, []byte(r.EvidenceRoot))
	writeLP(h, []byte(r.ModelID))
	writeLP(h, []byte(string(r.Aggregate)))
	writeLP(h, []byte(r.VetoedBy))
	writeF64(h, r.Confidence)

	sortedFacets := append([]FacetOutput(nil), r.Facets...)
	sort.Slice(sortedFacets, func(i, j int) bool { return sortedFacets[i].Kind < sortedFacets[j].Kind })
	writeU32(h, uint32(len(sortedFacets)))
	for _, f := range sortedFacets {
		writeLP(h, []byte(f.Kind))
		writeLP(h, []byte(string(f.Verdict)))
		writeF64(h, f.Confidence)
		writeLP(h, []byte(f.Reason))
	}

	sortedProvs := append([]ProviderReceipt(nil), r.ProviderReceipts...)
	sort.Slice(sortedProvs, func(i, j int) bool { return sortedProvs[i].ReceiptHash < sortedProvs[j].ReceiptHash })
	writeU32(h, uint32(len(sortedProvs)))
	for _, p := range sortedProvs {
		writeLP(h, []byte(p.Provider))
		writeLP(h, []byte(p.TrustLevel))
		writeLP(h, []byte(p.ReceiptHash))
	}

	if r.HITL != nil {
		writeU8(h, 1)
		writeLP(h, []byte(r.HITL.TicketID))
		writeLP(h, []byte(r.HITL.Action))
		writeLP(h, []byte(r.HITL.Reviewer))
		writeLP(h, []byte(r.HITL.ResolvedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")))
		writeLP(h, []byte(r.HITL.Note))
	} else {
		writeU8(h, 0)
	}

	return hex.EncodeToString(h.Sum(nil))
}

func writeLP(h hash.Hash, b []byte) {
	var lp [4]byte
	binary.BigEndian.PutUint32(lp[:], uint32(len(b)))
	_, _ = h.Write(lp[:])
	_, _ = h.Write(b)
}

func writeU32(h hash.Hash, v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	_, _ = h.Write(b[:])
}

func writeU8(h hash.Hash, v uint8) {
	_, _ = h.Write([]byte{v})
}

func writeF64(h hash.Hash, v float64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(v))
	_, _ = h.Write(b[:])
}
