package inference

import (
	"encoding/json"
	"testing"
)

// NOTE: a prior version of this file tested an imagined API (a
// NewModelRegistryEntry constructor, and a JSON shape with only 3 fields)
// that never matched the real ModelRegistryEntry struct (6 fields, no
// omitempty on most) and had never been compiled. Replaced with honest tests
// of the real struct and the store <-> inference conversion helpers, which
// don't require a live Postgres/Casper connection.

func TestModelRegistryEntry_JSONRoundTrip(t *testing.T) {
	entry := ModelRegistryEntry{
		ModelID:          "model-1",
		ModelHash:        "hash-1",
		VerifierContract: "contract-1",
		Metadata:         map[string]string{"version": "1.0"},
		RegisteredAt:     1720000000,
		DeployHash:       "deploy-1",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out ModelRegistryEntry
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if out.ModelID != entry.ModelID || out.ModelHash != entry.ModelHash ||
		out.VerifierContract != entry.VerifierContract || out.RegisteredAt != entry.RegisteredAt ||
		out.DeployHash != entry.DeployHash || out.Metadata["version"] != entry.Metadata["version"] {
		t.Errorf("round-tripped entry = %+v, want %+v", out, entry)
	}
}

func TestModelRegistryEntry_MetadataOmittedWhenEmpty(t *testing.T) {
	entry := ModelRegistryEntry{ModelID: "m", ModelHash: "h", VerifierContract: "c"}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}
	if _, present := raw["metadata"]; present {
		t.Error("expected metadata field to be omitted when empty (see `omitempty` tag)")
	}
}

func TestToStoreEntry_FromStoreEntry_RoundTrip(t *testing.T) {
	entry := &ModelRegistryEntry{
		ModelID:          "model-42",
		ModelHash:        "abc123",
		VerifierContract: "verifier-hash",
		Metadata:         map[string]string{"author": "test"},
		RegisteredAt:     1720000000,
		DeployHash:       "deploy-42",
	}

	stored := toStoreEntry(entry)
	if stored.ModelID != entry.ModelID || stored.ModelHash != entry.ModelHash {
		t.Fatalf("toStoreEntry lost fields: got %+v", stored)
	}

	back := fromStoreEntry(stored)
	if back.ModelID != entry.ModelID || back.ModelHash != entry.ModelHash ||
		back.VerifierContract != entry.VerifierContract || back.RegisteredAt != entry.RegisteredAt ||
		back.DeployHash != entry.DeployHash || back.Metadata["author"] != entry.Metadata["author"] {
		t.Errorf("round trip through store.ModelRegistryEntry changed data: got %+v, want %+v", back, entry)
	}
}

func TestToStoreEntry_ReturnsStoreType(t *testing.T) {
	entry := &ModelRegistryEntry{ModelID: "x", ModelHash: "y", VerifierContract: "z"}
	var _ = toStoreEntry(entry) // compile-time type check (type inferred)
}
