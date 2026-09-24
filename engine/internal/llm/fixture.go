package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// FixtureProvider is a deterministic in-process Provider used as a last-
// resort fallback when every real provider fails. It never touches the
// network. Determinism matters: the same Request must produce byte-identical
// bytes across runs so demo pipelines and unit tests are reproducible.
//
// Two modes:
//   - Table lookup: a caller-supplied map[promptHash]response.
//   - Deterministic canned: a stable "safe" verdict if nothing matches.
type FixtureProvider struct {
	// id is the provider ID reported in Response.Provider. Default "fixture".
	id string
	// table maps a deterministic key (built from Request) to the canned reply.
	table map[string]string
	// canned is the fallback reply used when nothing in the table matches.
	canned string
	// model is the fake model tag reported in Response.Model.
	model string
}

// FixtureConfig configures a fixture.
type FixtureConfig struct {
	ID     string            // default "fixture"
	Table  map[string]string // optional prompt-key → content
	Canned string            // default "ABSTAIN: fixture fallback (no real provider available)"
	Model  string            // default "fixture-v1"
}

// NewFixtureProvider builds a FixtureProvider.
func NewFixtureProvider(cfg FixtureConfig) *FixtureProvider {
	if cfg.ID == "" {
		cfg.ID = "fixture"
	}
	if cfg.Canned == "" {
		cfg.Canned = "ABSTAIN: fixture fallback (no real provider available)"
	}
	if cfg.Model == "" {
		cfg.Model = "fixture-v1"
	}
	return &FixtureProvider{
		id:     cfg.ID,
		table:  cfg.Table,
		canned: cfg.Canned,
		model:  cfg.Model,
	}
}

// ID reports the provider ID.
func (f *FixtureProvider) ID() string { return f.id }

// Tier reports TierReliability — a fixture is a fallback path, not a fast
// primary. The runner is expected to select the fixture explicitly rather
// than fan it out.
func (f *FixtureProvider) Tier() Tier { return TierReliability }

// KeyCount is 1 (a fixture is always "available"). This keeps the config
// loader from disabling it.
func (f *FixtureProvider) KeyCount() int { return 1 }

// Complete returns a canned answer keyed off the request. Deterministic.
// The ctx is checked for cancellation but no I/O happens.
func (f *FixtureProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, &ProviderError{Provider: f.id, Cause: err, Retryable: false}
	}

	key := fixtureKey(req)
	content := f.canned
	if hit, ok := f.table[key]; ok {
		content = hit
	}

	raw, _ := json.Marshal(map[string]any{
		"provider": f.id,
		"model":    f.model,
		"key":      key,
	})

	return &Response{
		Content:      content,
		Provider:     f.id,
		Model:        f.model,
		KeyIndex:     0,
		LatencyMs:    0, // instantaneous by design
		FinishReason: "stop",
		Fixture:      true,
		RawJSON:      raw,
	}, nil
}

// fixtureKey builds a deterministic key from the request. It joins role+content
// pairs in order — meaning callers who want table entries must build the same
// canonical string. Case-sensitive by design.
func fixtureKey(req Request) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(string(m.Role))
		b.WriteString(":")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// FixtureKeyFromMessages is exposed so callers can pre-compute keys for their
// table entries (e.g. in test setup, or when authoring prompt-injection
// fixtures for judge evaluation).
func FixtureKeyFromMessages(messages []Message) string {
	return fixtureKey(Request{Messages: messages})
}

// SortedTableKeys returns the fixture's table keys in stable order. Handy for
// logging / audit debug.
func (f *FixtureProvider) SortedTableKeys() []string {
	if f == nil || len(f.table) == 0 {
		return nil
	}
	out := make([]string, 0, len(f.table))
	for k := range f.table {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// String makes debug logs less noisy.
func (f *FixtureProvider) String() string {
	return fmt.Sprintf("FixtureProvider{id=%s tableEntries=%d}", f.id, len(f.table))
}

// SinceZero is a tiny helper used by tests that want to assert "no wall clock
// passed" — the fixture always reports LatencyMs=0, but the caller may have
// consumed real time between building the request and inspecting the result.
// Returned as duration for symmetry with time.Since.
func SinceZero(_ time.Time) time.Duration { return 0 }
