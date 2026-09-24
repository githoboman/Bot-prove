// Package config exposes the canonical on-chain deployment manifest to the
// engine.
//
// The single source of truth is deploy-out/onchain.json at the repo root.
// scripts/gen-manifest.mjs regenerates frontend/public/onchain.json from it.
// The engine reads the canonical file directly at startup so that a redeploy
// only requires updating deploy-out/onchain.json (no code change).
//
// Fallback resolution order:
//  1. env override CP_MANIFEST_PATH (absolute path)
//  2. ../deploy-out/onchain.json relative to the running binary
//  3. ./deploy-out/onchain.json (repo root when running `go run ./engine/...`)
//
// Callers that want a specific contract hash should call ContractHash instead
// of hardcoding it. A hardcoded hash in Go source is a Gate 1.5 violation.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ContractEntry is one deployed contract row from the manifest.
type ContractEntry struct {
	ContractHash        string   `json:"contract_hash"`
	ContractPackageHash string   `json:"contract_package_hash,omitempty"`
	DeployHash          string   `json:"deploy_hash"`
	Version             int      `json:"version,omitempty"`
	DeployedAt          string   `json:"deployed_at,omitempty"`
	Source              string   `json:"source"`
	EntryPoints         []string `json:"entry_points,omitempty"`
	Notes               string   `json:"notes,omitempty"`
}

// UndeployedContractEntry describes a contract that exists in source but is
// not yet deployed to the network.
type UndeployedContractEntry struct {
	Source string `json:"source"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Verification bundles the runtime health/frontend URLs.
type Verification struct {
	APIHealth     string `json:"api_health"`
	Frontend      string `json:"frontend"`
	VerifyScript  string `json:"verify_script,omitempty"`
}

// Manifest is the parsed canonical deploy-out/onchain.json.
type Manifest struct {
	Network              string                             `json:"network"`
	ChainName            string                             `json:"chain_name,omitempty"`
	Project              string                             `json:"project"`
	Deployer             string                             `json:"deployer"`
	Explorer             string                             `json:"explorer,omitempty"`
	CSPRCloud            string                             `json:"cspr_cloud,omitempty"`
	Contracts            map[string]ContractEntry           `json:"contracts"`
	UndeployedContracts  map[string]UndeployedContractEntry `json:"undeployed_contracts,omitempty"`
	Verification         Verification                       `json:"verification"`
}

var (
	loadOnce sync.Once
	loaded   *Manifest
	loadErr  error
)

// Load returns the cached parsed manifest (loading it on first call).
func Load() (*Manifest, error) {
	loadOnce.Do(func() {
		path, err := resolvePath()
		if err != nil {
			loadErr = err
			return
		}
		b, err := os.ReadFile(path)
		if err != nil {
			loadErr = fmt.Errorf("read manifest %s: %w", path, err)
			return
		}
		var m Manifest
		if err := json.Unmarshal(b, &m); err != nil {
			loadErr = fmt.Errorf("parse manifest %s: %w", path, err)
			return
		}
		loaded = &m
	})
	return loaded, loadErr
}

// MustLoad panics on error. Use only from init paths that have no other way
// to signal a fatal misconfiguration.
func MustLoad() *Manifest {
	m, err := Load()
	if err != nil {
		panic(fmt.Errorf("config.MustLoad: %w", err))
	}
	return m
}

// ContractHash returns the on-chain hash for a named contract, or "" when the
// contract key is not present in the manifest (undeployed contracts return "").
func ContractHash(key string) (string, error) {
	m, err := Load()
	if err != nil {
		return "", err
	}
	c, ok := m.Contracts[key]
	if !ok {
		return "", nil
	}
	return c.ContractHash, nil
}

// APIHealthURL is a small convenience for the verify surface.
func APIHealthURL() (string, error) {
	m, err := Load()
	if err != nil {
		return "", err
	}
	return m.Verification.APIHealth, nil
}

// Reload clears the cached manifest. Only meant for tests.
func Reload() {
	loadOnce = sync.Once{}
	loaded = nil
	loadErr = nil
}

func resolvePath() (string, error) {
	if p := os.Getenv("CP_MANIFEST_PATH"); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("CP_MANIFEST_PATH=%q not readable: %w", p, err)
		}
		return p, nil
	}
	// Walk up from the working directory looking for deploy-out/onchain.json.
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; {
		candidate := filepath.Join(dir, "deploy-out", "onchain.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("deploy-out/onchain.json not found from %s (upwards) and CP_MANIFEST_PATH unset", cwd)
}
