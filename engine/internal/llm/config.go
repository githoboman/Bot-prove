// Package llm — env-driven configuration loader.
//
// The loader reads provider API keys from the process environment ONLY.
// Keys never enter the git tree; the repo ships only .env.example with
// empty placeholders. Production reads them from the host env (Render
// service env vars); local dev reads them from an ignored .env.local.
package llm

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvSpec describes one provider's environment binding.
type EnvSpec struct {
	// ProviderID is the stable short name used in Response.Provider.
	ProviderID string

	// Tier is TierFast (parallel fan-out) or TierReliability (fallback).
	Tier Tier

	// KeyEnvs lists the env-variable names to read for API keys, in
	// preference order. Empty or unset values are dropped.
	KeyEnvs []string

	// InlineListEnv, when non-empty, is an env variable that may contain
	// comma-separated keys (e.g. GROQ_API_KEYS="k1,k2,k3"). Any keys
	// found there are appended to whatever KeyEnvs produced.
	InlineListEnv string
}

// DefaultEnvSpecs returns the canonical binding used in production.
// It matches the Render env-var names Quentin uses today (2026-07-20):
//   GROQ_API_KEY / GROQ2_API_KEY
//   NVIDIA_API_KEY
//   OPENROUTER_API_KEY / OPENROUTER2_API_KEY
//   ZAI_API_KEY / ZAI2_API_KEY
//   GEMINI_API_KEY
func DefaultEnvSpecs() []EnvSpec {
	return []EnvSpec{
		{
			ProviderID:    "groq",
			Tier:          TierFast,
			KeyEnvs:       []string{"GROQ_API_KEY", "GROQ2_API_KEY"},
			InlineListEnv: "GROQ_API_KEYS",
		},
		{
			ProviderID:    "nvidia",
			Tier:          TierFast,
			KeyEnvs:       []string{"NVIDIA_API_KEY"},
			InlineListEnv: "NVIDIA_API_KEYS",
		},
		{
			ProviderID:    "openrouter",
			Tier:          TierReliability,
			KeyEnvs:       []string{"OPENROUTER_API_KEY", "OPENROUTER2_API_KEY"},
			InlineListEnv: "OPENROUTER_API_KEYS",
		},
		{
			ProviderID:    "zai",
			Tier:          TierFast,
			KeyEnvs:       []string{"ZAI_API_KEY", "ZAI2_API_KEY"},
			InlineListEnv: "ZAI_API_KEYS",
		},
		{
			ProviderID:    "gemini",
			Tier:          TierReliability,
			KeyEnvs:       []string{"GEMINI_API_KEY"},
			InlineListEnv: "GEMINI_API_KEYS",
		},
	}
}

// LoadKeys returns the deduplicated, non-empty key list for a spec by
// reading os.Getenv. Order: individual KeyEnvs first, then InlineListEnv.
func (s EnvSpec) LoadKeys() []string {
	seen := make(map[string]struct{}, 8)
	var out []string
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	for _, name := range s.KeyEnvs {
		add(os.Getenv(name))
	}
	if s.InlineListEnv != "" {
		if raw := os.Getenv(s.InlineListEnv); raw != "" {
			for _, part := range strings.Split(raw, ",") {
				add(part)
			}
		}
	}
	return out
}

// LoadConfig reads Config timing knobs from the process env,
// falling back to DefaultConfig() values when unset or invalid.
func LoadConfig() Config {
	c := DefaultConfig()
	if v := os.Getenv("LLM_PROVIDER_TIMEOUT_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			c.PerProviderTimeout = time.Duration(ms) * time.Millisecond
		}
	}
	if v := os.Getenv("LLM_TOTAL_BUDGET_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			c.TotalBudget = time.Duration(ms) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(os.Getenv("LLM_FIXTURE_MODE")); v == "1" || strings.EqualFold(v, "true") {
		// Force fixture mode: caller may inspect this via ForceFixture().
		c.forceFixture = true
	}
	// PerProviderTimeout must never exceed TotalBudget (see runner.go); clamp
	// per-provider down rather than budget up so a misconfigured operator
	// can't accidentally extend the total budget.
	if c.PerProviderTimeout > c.TotalBudget {
		c.PerProviderTimeout = c.TotalBudget
	}
	return c
}

// ForceFixture reports whether LLM_FIXTURE_MODE=1 was set. When true,
// the runner should skip real providers entirely and serve the fixture.
// Kept as a method so external code doesn't touch the private field.
func (c Config) ForceFixture() bool { return c.forceFixture }
