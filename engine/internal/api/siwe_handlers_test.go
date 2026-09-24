package api

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/api/siwe"
)

func newSIWEServer() *Server {
	return &Server{
		siwe: siwe.NewStore(0),
	}
}

func doJSON(t *testing.T, h http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func TestSiweChallenge_HappyPath(t *testing.T) {
	s := newSIWEServer()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	rr := doJSON(t, s.siweChallenge, "/auth/siwe/challenge", siweChallengeReq{
		Pubkey:  hex.EncodeToString(pub),
		Purpose: "submit-batch",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp siweChallengeResp
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Message == "" || resp.Nonce == "" {
		t.Fatalf("empty message/nonce")
	}
	if resp.TTLSecs <= 0 {
		t.Fatalf("bad ttl: %d", resp.TTLSecs)
	}
}

func TestSiweChallenge_InvalidPubkey(t *testing.T) {
	s := newSIWEServer()
	rr := doJSON(t, s.siweChallenge, "/auth/siwe/challenge", siweChallengeReq{
		Pubkey:  "zzz",
		Purpose: "submit-batch",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSiweChallenge_InvalidPurpose(t *testing.T) {
	s := newSIWEServer()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	rr := doJSON(t, s.siweChallenge, "/auth/siwe/challenge", siweChallengeReq{
		Pubkey:  hex.EncodeToString(pub),
		Purpose: "UPPER CASE",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSiweVerify_HappyPath(t *testing.T) {
	s := newSIWEServer()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	// Step 1: issue.
	rr := doJSON(t, s.siweChallenge, "/auth/siwe/challenge", siweChallengeReq{
		Pubkey: pubHex, Purpose: "submit-batch",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("issue status=%d", rr.Code)
	}
	var issued siweChallengeResp
	_ = json.NewDecoder(rr.Body).Decode(&issued)
	// Step 2: verify.
	sig := ed25519.Sign(priv, []byte(issued.Message))
	rr2 := doJSON(t, s.siweVerify, "/auth/siwe/verify", siweVerifyReq{
		Pubkey:    pubHex,
		Purpose:   "submit-batch",
		Message:   issued.Message,
		Signature: hex.EncodeToString(sig),
	})
	if rr2.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var vr siweVerifyResp
	_ = json.NewDecoder(rr2.Body).Decode(&vr)
	if !vr.OK {
		t.Fatalf("verify ok=false")
	}
	// Step 3: replay must 401.
	rr3 := doJSON(t, s.siweVerify, "/auth/siwe/verify", siweVerifyReq{
		Pubkey:    pubHex,
		Purpose:   "submit-batch",
		Message:   issued.Message,
		Signature: hex.EncodeToString(sig),
	})
	if rr3.Code != http.StatusUnauthorized {
		t.Fatalf("replay status=%d, want 401", rr3.Code)
	}
}

func TestSiweVerify_WrongSignature(t *testing.T) {
	s := newSIWEServer()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	rr := doJSON(t, s.siweChallenge, "/auth/siwe/challenge", siweChallengeReq{
		Pubkey: pubHex, Purpose: "revoke-proof",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("issue status=%d", rr.Code)
	}
	var issued siweChallengeResp
	_ = json.NewDecoder(rr.Body).Decode(&issued)
	// Sign with other key.
	sig := ed25519.Sign(otherPriv, []byte(issued.Message))
	rr2 := doJSON(t, s.siweVerify, "/auth/siwe/verify", siweVerifyReq{
		Pubkey:    pubHex,
		Purpose:   "revoke-proof",
		Message:   issued.Message,
		Signature: hex.EncodeToString(sig),
	})
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rr2.Code)
	}
}

func TestSiweVerify_MalformedInputs(t *testing.T) {
	s := newSIWEServer()
	// bad pubkey
	rr := doJSON(t, s.siweVerify, "/auth/siwe/verify", siweVerifyReq{
		Pubkey: "zz", Purpose: "submit-batch", Message: "x",
		Signature: hex.EncodeToString(make([]byte, 64)),
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad pubkey: status=%d", rr.Code)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	// bad signature length
	rr2 := doJSON(t, s.siweVerify, "/auth/siwe/verify", siweVerifyReq{
		Pubkey: hex.EncodeToString(pub), Purpose: "submit-batch", Message: "x",
		Signature: "aabb",
	})
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("bad sig: status=%d", rr2.Code)
	}
	// empty message
	rr3 := doJSON(t, s.siweVerify, "/auth/siwe/verify", siweVerifyReq{
		Pubkey: hex.EncodeToString(pub), Purpose: "submit-batch", Message: "",
		Signature: hex.EncodeToString(make([]byte, 64)),
	})
	if rr3.Code != http.StatusBadRequest {
		t.Fatalf("empty msg: status=%d", rr3.Code)
	}
}

func TestSiweChallenge_MalformedBody(t *testing.T) {
	s := newSIWEServer()
	req := httptest.NewRequest(http.MethodPost, "/auth/siwe/challenge", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.siweChallenge(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rr.Code)
	}
}
