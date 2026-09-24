package verifier

// Attack-evidence contract tests (CP-6)
//
// These tests double as the machine-readable spec for the frontend
// Attack Evidence Lab (frontend/src/components/lab/AttackEvidence.tsx).
// Every scenario shown in that UI has a corresponding test here that
// asserts:
//   1. the honest tuple verifies (baseline),
//   2. the mutated tuple is rejected, and
//   3. the error message contains the substring the UI shows to the user
//      as the "detection field".
//
// If backend error wording drifts, this suite fails loudly instead of the
// UI silently mis-classifying a rejection as "not detected".

import (
	"strings"
	"testing"
)

type attackCase struct {
	name              string
	honestIn          string
	honestOut         string
	honestModel       string
	attackIn          string
	attackOut         string
	attackModel       string
	// If true, we revoke the proof before the second verify; the attacker
	// then replays the honest tuple (no mutation) counting on downstream
	// systems missing the revocation flag.
	revokeBeforeAttack bool
	wantErrContains    string
}

func TestAttackEvidenceScenarios(t *testing.T) {
	cases := []attackCase{
		{
			name:            "input-tamper (frame injection)",
			honestIn:        "video-frame:0..N (untampered)",
			honestOut:       "label=safe;score=0.98",
			honestModel:     "vision-v3.2.0",
			attackIn:        "video-frame:0..N (frame 42 replaced)",
			attackOut:       "label=safe;score=0.98",
			attackModel:     "vision-v3.2.0",
			wantErrContains: "input hash mismatch",
		},
		{
			name:            "output-tamper (verdict swap)",
			honestIn:        "loan_application_42",
			honestOut:       "decision=reject;reason=insufficient_income",
			honestModel:     "credit-scoring-v1.4",
			attackIn:        "loan_application_42",
			attackOut:       "decision=approve;reason=insufficient_income",
			attackModel:     "credit-scoring-v1.4",
			wantErrContains: "output hash mismatch",
		},
		{
			name:            "model-substitution",
			honestIn:        "query=is_this_a_phishing_url?",
			honestOut:       "phishing=true;confidence=0.94",
			honestModel:     "phishing-detector-v2.1-audited",
			attackIn:        "query=is_this_a_phishing_url?",
			attackOut:       "phishing=true;confidence=0.94",
			attackModel:     "phishing-detector-v1.0-shadow",
			wantErrContains: "model hash mismatch",
		},
		{
			name:            "proof-swap (all three fields mutated)",
			honestIn:        "kyc-doc:passport:ABC123",
			honestOut:       "kyc=passed;jurisdiction=EU",
			honestModel:     "kyc-classifier-v0.9",
			attackIn:        "kyc-doc:passport:XYZ789",
			attackOut:       "kyc=passed;jurisdiction=EU",
			attackModel:     "kyc-classifier-v0.9",
			wantErrContains: "hash mismatch", // at least one of the three must trip
		},
		{
			name:               "replay-after-revoke",
			honestIn:           "session-cookie:signed",
			honestOut:          "auth=granted",
			honestModel:        "auth-verifier-v3",
			attackIn:           "session-cookie:signed",
			attackOut:          "auth=granted",
			attackModel:        "auth-verifier-v3",
			revokeBeforeAttack: true,
			wantErrContains:    "revoked",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			v, eng := setup()

			// 1. Mint the honest proof.
			p := eng.Generate(
				"attack-evidence-agent",
				[]byte(tc.honestIn),
				[]byte(tc.honestOut),
				[]byte(tc.honestModel),
				"attack-evidence",
			)

			// 2. Baseline: honest tuple MUST verify. If this fails the
			//    test itself is broken, not the security property.
			if err := v.VerifyProof(p, []byte(tc.honestIn), []byte(tc.honestOut), []byte(tc.honestModel)); err != nil {
				t.Fatalf("baseline honest tuple rejected: %v", err)
			}

			// 3. Optional: flip revoke flag before the attack replay.
			if tc.revokeBeforeAttack {
				if err := eng.Revoke(p.ID, "attack-evidence-test"); err != nil {
					t.Fatalf("could not revoke proof: %v", err)
				}
				// Refresh the proof view — Revoke updates the underlying record.
				refreshed, ok := eng.Get(p.ID)
				if !ok {
					t.Fatalf("proof disappeared after revoke")
				}
				p = refreshed
			}

			// 4. Attack replay.
			err := v.VerifyProof(
				p,
				[]byte(tc.attackIn),
				[]byte(tc.attackOut),
				[]byte(tc.attackModel),
			)
			if err == nil {
				t.Fatalf("attack was NOT detected — /verify accepted the mutated tuple")
			}
			if !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Fatalf("wrong detection field: got %q, want substring %q", err.Error(), tc.wantErrContains)
			}
		})
	}
}

// Sanity check: the honest replay of the exact same tuple must NEVER be
// classified as an attack. Guards against a regression where /verify's
// error path leaks into the success path.
func TestHonestReplayIsNotAnAttack(t *testing.T) {
	v, eng := setup()
	in, out, m := []byte("honest-in"), []byte("honest-out"), []byte("honest-model")
	p := eng.Generate("agent", in, out, m, "attack-evidence")
	for i := 0; i < 3; i++ {
		if err := v.VerifyProof(p, in, out, m); err != nil {
			t.Fatalf("honest replay #%d rejected: %v", i, err)
		}
	}
}
