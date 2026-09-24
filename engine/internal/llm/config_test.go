package llm

import (
	"testing"
	"time"
)

// TestDefaultEnvSpecs_MatchRenderNames pins the provider→env-var mapping
// to exactly the variable names configured on the Render backend service
// as of 2026-07-20. If the ops team renames a variable on Render, THIS
// TEST FAILS and forces us to update both sides in one commit.
func TestDefaultEnvSpecs_MatchRenderNames(t *testing.T) {
	want := map[string][]string{
		"groq":       {"GROQ_API_KEY", "GROQ2_API_KEY"},
		"nvidia":     {"NVIDIA_API_KEY"},
		"openrouter": {"OPENROUTER_API_KEY", "OPENROUTER2_API_KEY"},
		"zai":        {"ZAI_API_KEY", "ZAI2_API_KEY"},
		"gemini":     {"GEMINI_API_KEY"},
	}
	specs := DefaultEnvSpecs()
	if len(specs) != len(want) {
		t.Fatalf("provider count = %d, want %d", len(specs), len(want))
	}
	for _, s := range specs {
		got := want[s.ProviderID]
		if got == nil {
			t.Errorf("unexpected provider id %q", s.ProviderID)
			continue
		}
		if len(s.KeyEnvs) != len(got) {
			t.Errorf("%s: key-env count = %d, want %d", s.ProviderID, len(s.KeyEnvs), len(got))
			continue
		}
		for i := range got {
			if s.KeyEnvs[i] != got[i] {
				t.Errorf("%s: KeyEnvs[%d] = %q, want %q", s.ProviderID, i, s.KeyEnvs[i], got[i])
			}
		}
	}
}

func TestDefaultEnvSpecs_TierPolicy(t *testing.T) {
	tiers := map[string]Tier{}
	for _, s := range DefaultEnvSpecs() {
		tiers[s.ProviderID] = s.Tier
	}
	// Fast tier: Groq, NVIDIA, OpenRouter (parallel fan-out).
	// Reliability tier: Z.AI, Gemini (fallback).
	if tiers["groq"] != TierFast {
		t.Errorf("groq should be fast, got %v", tiers["groq"])
	}
	if tiers["nvidia"] != TierFast {
		t.Errorf("nvidia should be fast, got %v", tiers["nvidia"])
	}
	if tiers["openrouter"] != TierReliability {
		t.Errorf("openrouter should be reliability (aggregator, slower), got %v", tiers["openrouter"])
	}
	if tiers["zai"] != TierFast {
		t.Errorf("zai (glm-4-flash) should be fast, got %v", tiers["zai"])
	}
	if tiers["gemini"] != TierReliability {
		t.Errorf("gemini should be reliability, got %v", tiers["gemini"])
	}
}

func TestLoadKeys_DedupAndTrim(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "  key-A  ")
	t.Setenv("GROQ2_API_KEY", "key-B")
	t.Setenv("GROQ_API_KEYS", "key-B, key-C , , key-A")
	spec := DefaultEnvSpecs()[0]
	if spec.ProviderID != "groq" {
		t.Fatalf("first spec should be groq, got %s", spec.ProviderID)
	}
	got := spec.LoadKeys()
	want := []string{"key-A", "key-B", "key-C"}
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadKeys_AllEmpty(t *testing.T) {
	for _, name := range []string{"GEMINI_API_KEY", "GEMINI_API_KEYS"} {
		t.Setenv(name, "")
	}
	var geminiSpec EnvSpec
	for _, s := range DefaultEnvSpecs() {
		if s.ProviderID == "gemini" {
			geminiSpec = s
			break
		}
	}
	if got := geminiSpec.LoadKeys(); len(got) != 0 {
		t.Errorf("expected zero keys, got %v", got)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Explicitly clear the env so the test is hermetic even if the host
	// happens to have LLM_* vars exported.
	t.Setenv("LLM_PROVIDER_TIMEOUT_MS", "")
	t.Setenv("LLM_TOTAL_BUDGET_MS", "")
	t.Setenv("LLM_FIXTURE_MODE", "")
	c := LoadConfig()
	if c.PerProviderTimeout != 3*time.Second {
		t.Errorf("default PerProviderTimeout = %v, want 3s", c.PerProviderTimeout)
	}
	if c.TotalBudget != 8*time.Second {
		t.Errorf("default TotalBudget = %v, want 8s", c.TotalBudget)
	}
	if c.ForceFixture() {
		t.Error("default ForceFixture should be false")
	}
	if !c.EnableFixtureFallback {
		t.Error("default EnableFixtureFallback should be true")
	}
}

func TestLoadConfig_Overrides(t *testing.T) {
	t.Setenv("LLM_PROVIDER_TIMEOUT_MS", "1500")
	t.Setenv("LLM_TOTAL_BUDGET_MS", "5000")
	t.Setenv("LLM_FIXTURE_MODE", "1")
	c := LoadConfig()
	if c.PerProviderTimeout != 1500*time.Millisecond {
		t.Errorf("PerProviderTimeout = %v, want 1.5s", c.PerProviderTimeout)
	}
	if c.TotalBudget != 5*time.Second {
		t.Errorf("TotalBudget = %v, want 5s", c.TotalBudget)
	}
	if !c.ForceFixture() {
		t.Error("ForceFixture should be true when LLM_FIXTURE_MODE=1")
	}
}

func TestLoadConfig_BudgetClamping(t *testing.T) {
	t.Setenv("LLM_PROVIDER_TIMEOUT_MS", "9000")
	t.Setenv("LLM_TOTAL_BUDGET_MS", "1000")
	t.Setenv("LLM_FIXTURE_MODE", "")
	c := LoadConfig()
	// PerProviderTimeout must be clamped DOWN to TotalBudget so a single
	// provider can’t exceed the total budget.
	if c.PerProviderTimeout > c.TotalBudget {
		t.Errorf("PerProviderTimeout=%v must be clamped to <= TotalBudget=%v",
			c.PerProviderTimeout, c.TotalBudget)
	}
	if c.PerProviderTimeout != c.TotalBudget {
		t.Errorf("expected PerProviderTimeout == TotalBudget after clamp, got %v vs %v",
			c.PerProviderTimeout, c.TotalBudget)
	}
}
