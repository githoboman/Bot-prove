package api

import (
	"fmt"
	"os"
	"strings"
)

// Preflight validates production startup requirements.
//
// Item 7.1 from API hardening v2. Two modes:
//
//   - CP_STRICT=1 → fail loud on ANY missing required env
//     (API_KEY, CONTRACT_PROOF_REGISTRY, CONTRACT_VERIFIER_GATE,
//     CONTRACT_DEFI_MOCK, CONTRACT_STAKE_SLASHING). Returns
//     a non-nil error that main should log and exit(2) on.
//   - Default (dev/demo) → returns nil; warnings still surface
//     via slog inside api.New(). Local-dev keeps the low-friction
//     path documented in KNOWN_LIMITATIONS.md.
//
// The list of required CONTRACT_* variables is intentionally the
// canonical 4 that the API ships with; the 3 newly-deployed ones
// (proof_of_inference, model_registry, proof_aggregation) come
// from `deploy-out/onchain.json`, not from env, so a strict-mode
// operator does NOT have to set 7 vars just to boot.
func Preflight(env func(string) string) error {
	if env == nil {
		env = os.Getenv
	}
	strict := env("CP_STRICT") == "1"
	if !strict {
		return nil
	}
	var missing []string
	if strings.TrimSpace(env("API_KEY")) == "" {
		missing = append(missing, "API_KEY")
	}
	for _, k := range []string{
		"CONTRACT_PROOF_REGISTRY",
		"CONTRACT_VERIFIER_GATE",
		"CONTRACT_DEFI_MOCK",
		"CONTRACT_STAKE_SLASHING",
	} {
		if strings.TrimSpace(env(k)) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"CP_STRICT=1 requires the following env vars to be set: %s "+
			"(fail-loud policy — see docs/API_CHANGELOG_POLICY.md §Startup preflight)",
		strings.Join(missing, ", "),
	)
}
