package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/quorum"
)

// enableQuorum populates a fresh registry with n signers (ids A, B, C, …)
// and returns the committee alongside the server. Signers are all
// ACTIVE at bond=1000.
func enableQuorum(t *testing.T, s *Server, n int) []committeeSK {
	t.Helper()
	s.quorumRegistry = quorum.NewRegistry()
	out := make([]committeeSK, 0, n)
	for i := 0; i < n; i++ {
		sk, pk, err := quorum.GenerateSecretKey()
		if err != nil {
			t.Fatalf("GenerateSecretKey: %v", err)
		}
		id := fmt.Sprintf("%c", 'A'+i)
		if err := s.quorumRegistry.Register(id, pk, 1000); err != nil {
			t.Fatalf("Register: %v", err)
		}
		out = append(out, committeeSK{id: id, sk: sk, pk: pk})
	}
	return out
}

type committeeSK struct {
	id string
	sk *quorum.SecretKey
	pk *quorum.PublicKey
}

func TestQuorumRegisterDisabledReturns503(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest(http.MethodPost, "/v1/quorum/signers", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.quorumRegisterSigner(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestQuorumRegisterSignerHappyPath(t *testing.T) {
	s := newTestServer("")
	s.quorumRegistry = quorum.NewRegistry()
	_, pk, _ := quorum.GenerateSecretKey()
	body, _ := json.Marshal(quorumRegisterRequest{ID: "A", PubKey: pk.Hex(), Bond: 500})
	req := httptest.NewRequest(http.MethodPost, "/v1/quorum/signers", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.quorumRegisterSigner(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp quorumSignerWire
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != "A" || resp.Status != "active" || resp.Bond != 500 {
		t.Fatalf("bad response: %+v", resp)
	}
}

func TestQuorumRegisterDuplicateReturns409(t *testing.T) {
	s := newTestServer("")
	s.quorumRegistry = quorum.NewRegistry()
	_, pk, _ := quorum.GenerateSecretKey()
	body, _ := json.Marshal(quorumRegisterRequest{ID: "A", PubKey: pk.Hex()})
	// First register — 201
	req := httptest.NewRequest(http.MethodPost, "/v1/quorum/signers", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.quorumRegisterSigner(rec, req)
	// Second — 409
	req2 := httptest.NewRequest(http.MethodPost, "/v1/quorum/signers", strings.NewReader(string(body)))
	rec2 := httptest.NewRecorder()
	s.quorumRegisterSigner(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestQuorumListSigners(t *testing.T) {
	s := newTestServer("")
	enableQuorum(t, s, 3)
	req := httptest.NewRequest(http.MethodGet, "/v1/quorum/signers", nil)
	rec := httptest.NewRecorder()
	s.quorumListSigners(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp quorumListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Total != 3 || resp.ActiveCount != 3 {
		t.Fatalf("bad list: %+v", resp)
	}
}

func TestQuorumVerifyHappyPath(t *testing.T) {
	s := newTestServer("")
	cs := enableQuorum(t, s, 5)
	msg := []byte("evidence-happy")
	sigs := []*quorum.Signature{}
	ids := []string{}
	for _, c := range cs[:4] {
		sig, _ := c.sk.Sign(msg)
		sigs = append(sigs, sig)
		ids = append(ids, c.id)
	}
	aggSig, _ := quorum.Aggregate(sigs)

	body, _ := json.Marshal(quorumVerifyRequest{
		EvidenceRootHex: hex.EncodeToString(msg),
		SignerBitset:    ids,
		AggregateSigHex: aggSig.Hex(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/quorum/verify", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.quorumVerify(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var w quorum.QuorumWitness
	if err := json.Unmarshal(rec.Body.Bytes(), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Verdict != "APPROVE" || w.WitnessHashHex == "" {
		t.Fatalf("bad witness: %+v", w)
	}
}

func TestQuorumVerifyTamperedSigReturns422(t *testing.T) {
	s := newTestServer("")
	cs := enableQuorum(t, s, 5)
	sigs := []*quorum.Signature{}
	ids := []string{}
	for _, c := range cs[:4] {
		sig, _ := c.sk.Sign([]byte("original"))
		sigs = append(sigs, sig)
		ids = append(ids, c.id)
	}
	aggSig, _ := quorum.Aggregate(sigs)

	// Verify against a different message.
	body, _ := json.Marshal(quorumVerifyRequest{
		EvidenceRootHex: hex.EncodeToString([]byte("tampered")),
		SignerBitset:    ids,
		AggregateSigHex: aggSig.Hex(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/quorum/verify", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.quorumVerify(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestQuorumVerifyBelowThresholdReturns403(t *testing.T) {
	s := newTestServer("")
	cs := enableQuorum(t, s, 5)
	sigs := []*quorum.Signature{}
	ids := []string{}
	for _, c := range cs[:2] { // only 2/5 — below threshold=4
		sig, _ := c.sk.Sign([]byte("msg"))
		sigs = append(sigs, sig)
		ids = append(ids, c.id)
	}
	aggSig, _ := quorum.Aggregate(sigs)
	body, _ := json.Marshal(quorumVerifyRequest{
		EvidenceRootHex: hex.EncodeToString([]byte("msg")),
		SignerBitset:    ids,
		AggregateSigHex: aggSig.Hex(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/quorum/verify", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.quorumVerify(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestQuorumThresholdEndpoint(t *testing.T) {
	s := newTestServer("")
	enableQuorum(t, s, 10)
	req := httptest.NewRequest(http.MethodGet, "/v1/quorum/threshold", nil)
	rec := httptest.NewRecorder()
	s.quorumThreshold(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp quorumThresholdResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ActiveCount != 10 || resp.Threshold != 7 { // floor(20/3)+1 = 7
		t.Fatalf("bad threshold: %+v", resp)
	}
}

func TestQuorumSlashSigner(t *testing.T) {
	s := newTestServer("")
	enableQuorum(t, s, 3)
	req := httptest.NewRequest(http.MethodPost, "/v1/quorum/signers/A/slash", strings.NewReader(`{"reason":"equivocation"}`))
	req.SetPathValue("id", "A")
	rec := httptest.NewRecorder()
	s.quorumSlashSigner(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out quorumSignerWire
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Status != "slashed" || out.SlashReason != "equivocation" {
		t.Fatalf("bad slash: %+v", out)
	}
	if s.quorumRegistry.ActiveCount() != 2 {
		t.Fatalf("ActiveCount = %d, want 2", s.quorumRegistry.ActiveCount())
	}
}
