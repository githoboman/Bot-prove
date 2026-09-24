package api

import (
	"strings"
	"testing"
)

func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestPreflight_NonStrict_AlwaysNil(t *testing.T) {
	if err := Preflight(fakeEnv(map[string]string{})); err != nil {
		t.Fatalf("non-strict must be permissive, got %v", err)
	}
	if err := Preflight(fakeEnv(map[string]string{"CP_STRICT": "0"})); err != nil {
		t.Fatalf("CP_STRICT=0 must be permissive, got %v", err)
	}
}

func TestPreflight_Strict_MissingAllFive(t *testing.T) {
	err := Preflight(fakeEnv(map[string]string{"CP_STRICT": "1"}))
	if err == nil {
		t.Fatal("expected error when strict + no vars set")
	}
	msg := err.Error()
	for _, want := range []string{
		"API_KEY",
		"CONTRACT_PROOF_REGISTRY",
		"CONTRACT_VERIFIER_GATE",
		"CONTRACT_DEFI_MOCK",
		"CONTRACT_STAKE_SLASHING",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in error message: %s", want, msg)
		}
	}
}

func TestPreflight_Strict_MissingOnlyOne(t *testing.T) {
	env := map[string]string{
		"CP_STRICT":                "1",
		"API_KEY":                  "k",
		"CONTRACT_PROOF_REGISTRY":  "a",
		"CONTRACT_VERIFIER_GATE":   "b",
		"CONTRACT_DEFI_MOCK":       "c",
		"CONTRACT_STAKE_SLASHING":  "",
	}
	err := Preflight(fakeEnv(env))
	if err == nil {
		t.Fatal("expected error when one var missing")
	}
	if !strings.Contains(err.Error(), "CONTRACT_STAKE_SLASHING") {
		t.Errorf("expected missing var name, got: %s", err.Error())
	}
	if strings.Contains(err.Error(), "API_KEY,") {
		t.Errorf("API_KEY should not be in missing list: %s", err.Error())
	}
}

func TestPreflight_Strict_WhitespaceCountsAsMissing(t *testing.T) {
	env := map[string]string{
		"CP_STRICT":                "1",
		"API_KEY":                  "   ",
		"CONTRACT_PROOF_REGISTRY":  "a",
		"CONTRACT_VERIFIER_GATE":   "b",
		"CONTRACT_DEFI_MOCK":       "c",
		"CONTRACT_STAKE_SLASHING":  "d",
	}
	err := Preflight(fakeEnv(env))
	if err == nil {
		t.Fatal("whitespace-only API_KEY must fail")
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Errorf("expected API_KEY in error, got: %s", err.Error())
	}
}

func TestPreflight_Strict_AllSet_OK(t *testing.T) {
	env := map[string]string{
		"CP_STRICT":                "1",
		"API_KEY":                  "k",
		"CONTRACT_PROOF_REGISTRY":  "a",
		"CONTRACT_VERIFIER_GATE":   "b",
		"CONTRACT_DEFI_MOCK":       "c",
		"CONTRACT_STAKE_SLASHING":  "d",
	}
	if err := Preflight(fakeEnv(env)); err != nil {
		t.Fatalf("all vars set — expected no error, got %v", err)
	}
}

func TestPreflight_NilEnv_DefaultsToOsGetenv(t *testing.T) {
	// Should not panic; production callers pass nil to use os.Getenv.
	// In test env CP_STRICT is not set, so this is nil (non-strict).
	if err := Preflight(nil); err != nil {
		t.Fatalf("nil env must default to os.Getenv, got %v", err)
	}
}
