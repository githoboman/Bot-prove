// Wire helpers: convert env-driven EnvSpecs into concrete Provider instances.
//
// This file is deliberately separate from config.go so tests can construct
// providers directly without touching the env loader, and vice-versa.
package llm

import (
	"log/slog"
	"time"
)

// BuildProvidersFromEnv assembles the production provider set from process
// env, using DefaultEnvSpecs() as the binding table.
//
// A provider is INCLUDED only when at least one key is present in env. This
// means the runner can be started with a partial set (e.g. Groq + Gemini only,
// no OpenRouter) and it will just skip the missing ones.
//
// Cooldown is the per-key backoff on 429/5xx. Default 30s if <= 0.
func BuildProvidersFromEnv(cooldown time.Duration) []Provider {
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	var providers []Provider
	for _, spec := range DefaultEnvSpecs() {
		keys := spec.LoadKeys()
		if len(keys) == 0 {
			slog.Info("llm: provider skipped (no keys)", "provider", spec.ProviderID)
			continue
		}
		ring := NewKeyRing(keys, cooldown)
		p := buildProvider(spec.ProviderID, ring)
		if p == nil {
			slog.Warn("llm: unknown provider id in EnvSpec", "provider", spec.ProviderID)
			continue
		}
		providers = append(providers, p)
		slog.Info("llm: provider wired", "provider", spec.ProviderID, "keys", len(keys), "tier", spec.Tier)
	}
	return providers
}

// buildProvider is a hand-rolled switch to avoid a reflection-based factory.
// New provider adapters must be added here.
func buildProvider(id string, ring *KeyRing) Provider {
	switch id {
	case "groq":
		return NewGroqProvider(ring)
	case "nvidia":
		return NewNVIDIA(ring)
	case "openrouter":
		return NewOpenRouter(ring)
	case "zai":
		return NewZAI(ring)
	case "gemini":
		return NewGemini(ring)
	default:
		return nil
	}
}
