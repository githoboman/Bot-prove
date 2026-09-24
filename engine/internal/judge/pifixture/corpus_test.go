package pifixture_test

import (
	"context"
	"testing"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/pifixture"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/llm"
)

// runCase spins up a Runner with the fixture providers seeded for the case,
// then runs the judge and asserts overall + per-facet verdicts.
func runCase(t *testing.T, c pifixture.Case) {
	t.Helper()

	fixtures := pifixture.SeederFor(c)
	// Adapt to []llm.Provider (interface).
	provs := make([]llm.Provider, 0, len(fixtures))
	for _, f := range fixtures {
		provs = append(provs, f)
	}
	// Runner treats fixtures as TierReliability — Poll() polls both tiers, so
	// they will all be polled regardless.
	runner := llm.NewRunner(provs, nil, llm.Config{
		TotalBudget:        2 * time.Second,
		PerProviderTimeout: 500 * time.Millisecond,
	})

	j := judge.NewFacetJudge(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := j.Decide(ctx, c.Task)
	if err != nil {
		t.Fatalf("case %s: Decide error: %v", c.Name, err)
	}

	if res.OverallVerdict != c.ExpectedOverall {
		t.Errorf("case %s: OverallVerdict = %s, want %s", c.Name, res.OverallVerdict, c.ExpectedOverall)
		for fid, fr := range res.Facets {
			t.Logf("  facet %s: verdict=%s winner=%s live=%d frac=%.2f",
				fid, fr.Verdict, fr.Winner, fr.LiveCount, fr.AgreementFraction)
			for _, v := range fr.Votes {
				t.Logf("    vote %s: value=%q raw=%q err=%q", v.ProviderID, v.Value, v.Raw, v.Err)
			}
		}
	}

	for fid, want := range c.ExpectedFacets {
		got, ok := res.Facets[fid]
		if !ok {
			t.Errorf("case %s: missing facet result for %s", c.Name, fid)
			continue
		}
		if got.Verdict != want {
			t.Errorf("case %s: facet %s verdict = %s, want %s (winner=%q live=%d frac=%.2f)",
				c.Name, fid, got.Verdict, want, got.Winner, got.LiveCount, got.AgreementFraction)
		}
	}
}

func TestCorpusFullRun(t *testing.T) {
	cases := pifixture.Corpus()
	if len(cases) < 8 {
		t.Fatalf("corpus should have at least 8 cases, got %d", len(cases))
	}
	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			runCase(t, c)
		})
	}
}

// TestSeederIsDeterministic guarantees the same case produces byte-identical
// fixture tables across seeder calls. Downstream equivocation-proof consumers
// depend on this: if provider tables drift, the "proof" is not reproducible.
func TestSeederIsDeterministic(t *testing.T) {
	cases := pifixture.Corpus()
	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			a := pifixture.SeederFor(c)
			b := pifixture.SeederFor(c)
			if len(a) != len(b) {
				t.Fatalf("provider count changed: %d vs %d", len(a), len(b))
			}
			ai := indexByID(a)
			bi := indexByID(b)
			for id, pa := range ai {
				pb, ok := bi[id]
				if !ok {
					t.Errorf("provider %s missing in second seeder", id)
					continue
				}
				ka := pa.SortedTableKeys()
				kb := pb.SortedTableKeys()
				if len(ka) != len(kb) {
					t.Errorf("provider %s: key count %d vs %d", id, len(ka), len(kb))
					continue
				}
				for i := range ka {
					if ka[i] != kb[i] {
						t.Errorf("provider %s key %d drift: %q vs %q", id, i, ka[i], kb[i])
					}
				}
			}
		})
	}
}

func indexByID(ps []*llm.FixtureProvider) map[string]*llm.FixtureProvider {
	out := make(map[string]*llm.FixtureProvider, len(ps))
	for _, p := range ps {
		out[p.ID()] = p
	}
	return out
}

// TestSeederOutageSimulation verifies that when a case omits a provider from
// ProviderAnswers, the seeder does not produce a fixture for it. This is how
// we simulate a live outage in the deterministic corpus.
func TestSeederOutageSimulation(t *testing.T) {
	outageCase := pifixture.Case{
		Name: "outage-check",
		Task: &judge.Task{
			ID:                 "outage-check",
			Input:              "hello",
			SystemMsg:          "sys",
			Facets:             []judge.Facet{{ID: "x", Prompt: "q?", AllowedValues: []string{"yes", "no"}, Weight: 1.0}},
			MinProviders:       2,
			AgreementThreshold: 0.66,
		},
		ProviderAnswers: map[string]map[string]string{
			"only-groq": {"x": "yes"},
		},
	}
	fixtures := pifixture.SeederFor(outageCase)
	if len(fixtures) != 1 {
		t.Fatalf("expected 1 fixture, got %d", len(fixtures))
	}
	if fixtures[0].ID() != "only-groq" {
		t.Errorf("fixture ID = %q, want only-groq", fixtures[0].ID())
	}
}
