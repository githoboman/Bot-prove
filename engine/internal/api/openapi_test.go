package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateOpenAPIStructure(t *testing.T) {
	spec := GenerateOpenAPI()
	if v, _ := spec["openapi"].(string); v != openAPIVersion {
		t.Fatalf("openapi version: %v", spec["openapi"])
	}
	// Every routeScopes entry should surface under paths.
	paths, _ := spec["paths"].(map[string]any)
	if paths == nil {
		t.Fatal("no paths")
	}
	// Sample check: /proofs must exist and have POST + GET.
	proofs, ok := paths["/proofs"].(map[string]any)
	if !ok {
		t.Fatalf("no /proofs entry, paths: %v", paths)
	}
	if _, ok := proofs["post"]; !ok {
		t.Fatal("no POST /proofs")
	}
	if _, ok := proofs["get"]; !ok {
		t.Fatal("no GET /proofs")
	}
	// Security scheme present.
	comps, _ := spec["components"].(map[string]any)
	schemes, ok := comps["securitySchemes"].(map[string]any)
	if !ok {
		t.Fatalf("securitySchemes: expected map[string]any, got %T", comps["securitySchemes"])
	}
	if _, ok := schemes["ApiKeyAuth"]; !ok {
		t.Fatal("no ApiKeyAuth")
	}
	// x-webhooks enumeration.
	wh, ok := spec["x-webhooks"].(map[string]any)
	if !ok {
		t.Fatal("no x-webhooks")
	}
	if events, _ := wh["events"].([]string); len(events) != len(KnownWebhookEvents) {
		t.Fatalf("event enum mismatch: %v", events)
	}
}

func TestOpenAPIHandlerServesJSON(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	s.openAPIHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("openapi: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: %q", ct)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["openapi"] != openAPIVersion {
		t.Fatalf("bad openapi field: %v", out["openapi"])
	}
}

func TestRoutesHandler(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest(http.MethodGet, "/v1/routes", nil)
	rec := httptest.NewRecorder()
	s.routesHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("routes: got %d", rec.Code)
	}
	var out struct {
		Count  int            `json:"count"`
		Routes []openAPIRoute `json:"routes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count < 20 {
		t.Fatalf("expected many routes, got %d", out.Count)
	}
	// Sanity: at least one route with a scope.
	found := false
	for _, r := range out.Routes {
		if r.Scope != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no scoped routes in output")
	}
}

func TestPathParamsExtraction(t *testing.T) {
	params := pathParams("/proofs/{id}/revoke")
	if len(params) != 1 || params[0]["name"] != "id" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if len(pathParams("/health")) != 0 {
		t.Fatal("expected no params for /health")
	}
	if p := pathParams("/foo/{a}/bar/{b}"); len(p) != 2 || p[0]["name"] != "a" || p[1]["name"] != "b" {
		t.Fatalf("nested: %+v", p)
	}
}
