package prover

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func SaveBundle(dir string, bundle *ProofBundle) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, bundle.Root+".json"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return json.NewEncoder(f).Encode(bundle)
}

func LoadBundle(path string) (*ProofBundle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var b ProofBundle
	if err := json.NewDecoder(f).Decode(&b); err != nil {
		return nil, err
	}
	return &b, nil
}
