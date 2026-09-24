package config

import (
	"path/filepath"
	"testing"
)

func TestLoad_ReadsCanonicalManifest(t *testing.T) {
	// Point the loader at the repo-root deploy-out/onchain.json regardless
	// of `go test` working directory.
	wd, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	t.Setenv("CP_MANIFEST_PATH", filepath.Join(wd, "deploy-out", "onchain.json"))
	Reload()

	m, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Project != "CasperProver" {
		t.Fatalf("project = %q, want %q", m.Project, "CasperProver")
	}
	if m.Network != "casper-test" && m.Network != "casper-mainnet" {
		t.Fatalf("unexpected network %q", m.Network)
	}
	if len(m.Contracts) < 4 {
		t.Fatalf("expected >=4 deployed contracts, got %d", len(m.Contracts))
	}
	for name, c := range m.Contracts {
		if len(c.ContractHash) != 64 {
			t.Errorf("%s: contract_hash must be 64 hex chars, got %d", name, len(c.ContractHash))
		}
	}
	// Sanity: known keys present.
	for _, key := range []string{"proof_registry", "verifier_gate", "defi_mock", "stake_slashing"} {
		if _, ok := m.Contracts[key]; !ok {
			t.Errorf("missing required contract entry %q", key)
		}
	}
}

func TestContractHash_UnknownReturnsEmpty(t *testing.T) {
	wd, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	t.Setenv("CP_MANIFEST_PATH", filepath.Join(wd, "deploy-out", "onchain.json"))
	Reload()

	h, err := ContractHash("nonexistent_contract")
	if err != nil {
		t.Fatalf("ContractHash err: %v", err)
	}
	if h != "" {
		t.Fatalf("unknown contract should return empty, got %q", h)
	}
}

func TestContractHash_KnownReturnsHex(t *testing.T) {
	wd, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	t.Setenv("CP_MANIFEST_PATH", filepath.Join(wd, "deploy-out", "onchain.json"))
	Reload()

	h, err := ContractHash("proof_registry")
	if err != nil {
		t.Fatalf("ContractHash err: %v", err)
	}
	if len(h) != 64 {
		t.Fatalf("proof_registry hash length = %d, want 64", len(h))
	}
}
