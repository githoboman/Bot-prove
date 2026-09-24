package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// onchainManifestHandler serves the canonical on-chain contract manifest.
//
// Gate 1 DoD: root deploy-out/onchain.json is the single source of truth for
// contract addresses. This endpoint exposes it verbatim over HTTP so
// frontends, CI, third-party integrations, and verify.sh can pull the same
// bytes over the wire instead of hardcoding hashes. Cached in memory with a
// 60s refresh window — the file rarely changes and we never want a redeploy
// to be blocked by disk IO.
//
// Resolution order for the manifest path (first hit wins):
//  1. ONCHAIN_MANIFEST_PATH env override
//  2. ./deploy-out/onchain.json relative to cwd (production, Render workdir)
//  3. ../deploy-out/onchain.json (when running from ./engine locally)
//
// If nothing is found, returns 503 with a diagnostic — never a stale copy.

type manifestCache struct {
	mu       sync.Mutex
	bytes    []byte
	modtime  time.Time
	loadedAt time.Time
	path     string
}

var manifestState = &manifestCache{}

const manifestTTL = 60 * time.Second

func (s *Server) onchainManifest(w http.ResponseWriter, r *http.Request) {
	body, path, err := loadOnchainManifest()
	if err != nil {
		s.log.Warn("onchain manifest unavailable", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":  "onchain manifest unavailable",
			"detail": err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("X-Manifest-Source", path)
	_, _ = w.Write(body)
	slog.Debug("served onchain manifest", "bytes", len(body), "path", path)
}

func loadOnchainManifest() ([]byte, string, error) {
	manifestState.mu.Lock()
	defer manifestState.mu.Unlock()

	// Fast path: cached, still within TTL, source unchanged.
	if manifestState.bytes != nil && time.Since(manifestState.loadedAt) < manifestTTL {
		return manifestState.bytes, manifestState.path, nil
	}

	path, err := resolveManifestPath()
	if err != nil {
		return nil, "", err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	// If TTL expired but mtime unchanged, just refresh the loadedAt stamp.
	if manifestState.bytes != nil && info.ModTime().Equal(manifestState.modtime) && manifestState.path == path {
		manifestState.loadedAt = time.Now()
		return manifestState.bytes, manifestState.path, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	// Sanity check: valid JSON with a non-empty contracts map. Refuse to serve
	// something that would silently break downstream consumers.
	var probe struct {
		Contracts map[string]json.RawMessage `json:"contracts"`
	}
	if jerr := json.Unmarshal(raw, &probe); jerr != nil {
		return nil, "", jerr
	}
	if len(probe.Contracts) == 0 {
		return nil, "", errEmptyManifest{path: path}
	}

	manifestState.bytes = raw
	manifestState.modtime = info.ModTime()
	manifestState.loadedAt = time.Now()
	manifestState.path = path
	return manifestState.bytes, manifestState.path, nil
}

func resolveManifestPath() (string, error) {
	if p := os.Getenv("ONCHAIN_MANIFEST_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	candidates := []string{
		filepath.Join("deploy-out", "onchain.json"),
		filepath.Join("..", "deploy-out", "onchain.json"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	return "", errNoManifest{}
}

type errNoManifest struct{}

func (errNoManifest) Error() string {
	return "canonical onchain manifest not found (looked at $ONCHAIN_MANIFEST_PATH, ./deploy-out/onchain.json, ../deploy-out/onchain.json)"
}

type errEmptyManifest struct{ path string }

func (e errEmptyManifest) Error() string {
	return "canonical onchain manifest has no contracts: " + e.path
}
