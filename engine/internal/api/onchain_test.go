package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// resetManifestCache clears the package-level cache between tests. The cache
// is intentionally process-wide (loaded once, served for 60s), so tests that
// swap the underlying file must reset it explicitly.
func resetManifestCache() {
	manifestState.mu.Lock()
	defer manifestState.mu.Unlock()
	manifestState.bytes = nil
	manifestState.path = ""
	manifestState.loadedAt = manifestState.loadedAt.Truncate(0)
}

func writeManifest(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "onchain.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return p
}

func TestOnchainManifest_ServesCanonical(t *testing.T) {
	resetManifestCache()
	dir := t.TempDir()
	path := writeManifest(t, dir, `{"contracts":{"proof_registry":{"contract_hash":"aa"}}}`)
	t.Setenv("ONCHAIN_MANIFEST_PATH", path)

	s := &Server{log: slog.Default()}
	rec := httptest.NewRecorder()
	s.onchainManifest(rec, httptest.NewRequest(http.MethodGet, "/onchain.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type: got %q", got)
	}
	if got := rec.Header().Get("X-Manifest-Source"); got != path {
		t.Fatalf("X-Manifest-Source: got %q want %q", got, path)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if _, ok := out["contracts"]; !ok {
		t.Fatalf("body missing 'contracts': %v", out)
	}
}

func TestOnchainManifest_503_WhenMissing(t *testing.T) {
	resetManifestCache()
	// Point env at a non-existent file AND run inside a tempdir with no
	// deploy-out/ candidate so the fallback resolution fails too.
	dir := t.TempDir()
	t.Setenv("ONCHAIN_MANIFEST_PATH", filepath.Join(dir, "does-not-exist.json"))
	cwd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(cwd) }()

	s := &Server{log: slog.Default()}
	rec := httptest.NewRecorder()
	s.onchainManifest(rec, httptest.NewRequest(http.MethodGet, "/onchain.json", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestOnchainManifest_503_WhenEmpty(t *testing.T) {
	resetManifestCache()
	dir := t.TempDir()
	path := writeManifest(t, dir, `{"contracts":{}}`)
	t.Setenv("ONCHAIN_MANIFEST_PATH", path)

	s := &Server{log: slog.Default()}
	rec := httptest.NewRecorder()
	s.onchainManifest(rec, httptest.NewRequest(http.MethodGet, "/onchain.json", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestOnchainManifest_CachesBetweenCalls(t *testing.T) {
	resetManifestCache()
	dir := t.TempDir()
	path := writeManifest(t, dir, `{"contracts":{"proof_registry":{"contract_hash":"aa"}}}`)
	t.Setenv("ONCHAIN_MANIFEST_PATH", path)

	s := &Server{log: slog.Default()}
	rec1 := httptest.NewRecorder()
	s.onchainManifest(rec1, httptest.NewRequest(http.MethodGet, "/onchain.json", nil))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first call: %d", rec1.Code)
	}
	// Overwrite the source with garbage on disk. Cache should still serve the
	// previously-loaded good body until TTL/mtime forces a refresh.
	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	rec2 := httptest.NewRecorder()
	s.onchainManifest(rec2, httptest.NewRequest(http.MethodGet, "/onchain.json", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("second call (cached): %d body=%s", rec2.Code, rec2.Body.String())
	}
	if rec1.Body.String() != rec2.Body.String() {
		t.Fatalf("cache did not serve identical bytes")
	}
}
