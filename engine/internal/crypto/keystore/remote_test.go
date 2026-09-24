package keystore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

func TestRemoteStub_NotConfigured(t *testing.T) {
	r := NewRemote("", "")
	info := r.Info(context.Background())
	if info.Kind != KindRemote {
		t.Fatalf("kind: %q", info.Kind)
	}
	if info.HardwareBacked {
		t.Fatal("unconfigured remote must not claim hardware backing")
	}
	if !strings.Contains(info.Backing, "NOT configured") {
		t.Fatalf("backing should say not configured: %q", info.Backing)
	}
	if _, err := r.CreateKey(context.Background(), pqcrypto.AlgoEd25519); err == nil {
		t.Fatal("CreateKey on unconfigured stub must error")
	}
}

// TestRemoteStub_Roundtrip runs against a mock HTTP gateway that mimics
// the documented contract. It's a harness sanity check — not a real HSM
// integration test.
func TestRemoteStub_Roundtrip(t *testing.T) {
	// In-memory ring backing the mock gateway.
	ring := pqcrypto.NewKeyRing()

	handler := http.NewServeMux()
	handler.HandleFunc("POST /keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Algo string `json:"algo"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		algo, err := pqcrypto.ParseAlgo(req.Algo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := ring.CreateKey(algo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meta)
	})
	handler.HandleFunc("POST /keys/{id}/sign", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		body, _ := io.ReadAll(r.Body)
		var req struct {
			MessageHex string `json:"message_hex"`
		}
		_ = json.Unmarshal(body, &req)
		msg, err := hex.DecodeString(req.MessageHex)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sig, err := ring.SignWithKey(id, msg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"signature_hex": hex.EncodeToString(sig)})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	r := NewRemote(srv.URL, "secret")
	ctx := context.Background()
	info := r.Info(ctx)
	if !info.HardwareBacked {
		t.Fatal("configured remote should claim hardware_backed=true in Info (docs contract)")
	}

	meta, err := r.CreateKey(ctx, pqcrypto.AlgoEd25519)
	if err != nil {
		t.Fatalf("remote create: %v", err)
	}
	if meta.Algo != pqcrypto.AlgoEd25519 {
		t.Fatalf("bad meta: %+v", meta)
	}

	msg := []byte("remote-signed")
	sig, id, err := r.Sign(ctx, pqcrypto.AlgoEd25519, msg)
	if err != nil {
		t.Fatalf("remote sign: %v", err)
	}
	if id != meta.ID {
		t.Fatalf("sign id: got %q want %q", id, meta.ID)
	}
	// Verify locally against the mirrored public key.
	ok, err := r.Verify(ctx, id, msg, sig)
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}
}

func TestRemoteStub_UnauthorizedRejected(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /keys", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	r := NewRemote(srv.URL, "wrong-token")
	_, err := r.CreateKey(context.Background(), pqcrypto.AlgoEd25519)
	if err == nil {
		t.Fatal("401 gateway response must surface as error")
	}
}
