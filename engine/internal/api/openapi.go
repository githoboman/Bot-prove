package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// OpenAPI auto-generation.
//
// The generator walks the registered mux routes plus the routeScopes
// map (which is the authoritative list of scope requirements) and
// produces a minimal but valid OpenAPI 3.1 document. It is
// intentionally NOT a full spec — request/response schemas for every
// endpoint would be a hand-maintained tax we can't afford before the
// deadline. Instead, each route documents:
//
//   * summary                 — a one-liner derived from the handler name
//   * required X-API-Key      — enforced by authMiddleware / scopeMiddleware
//   * required scope          — from routeScopes
//   * request/response type   — application/json (generic object)
//   * error responses         — 400/401/403/404/500 with the standard
//                               { error: string } envelope
//
// A future PR can enrich the spec by tagging routes with $ref to
// hand-written component schemas as they get added; this generator
// leaves room for that via the routeSchemas hook below.

const openAPIVersion = "3.1.0"

// routeSchemas lets future code override request/response bodies for
// specific routes. Empty for now — every route uses the generic
// object shape. Kept as a map so a new schema is a one-line addition.
var routeSchemas = map[string]struct {
	RequestSchema  map[string]any
	ResponseSchema map[string]any
}{}

// openAPIRoute is one row in the generated spec.
type openAPIRoute struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Pattern string `json:"pattern"` // "METHOD /path" — the key used in routeScopes
	Scope   string `json:"scope,omitempty"`
	Summary string `json:"summary"`
}

// enumerateRoutes reconstructs the route table from routeScopes plus
// the versioned webhook routes. It's the single source of truth
// consumed by both the OpenAPI generator and the /v1/routes debug
// endpoint.
func enumerateRoutes() []openAPIRoute {
	out := make([]openAPIRoute, 0, len(routeScopes)+8)
	// Untyped extras — endpoints without a declared scope that we
	// still want to publish in the spec.
	extras := []struct{ pattern, summary string }{
		{"GET /health", "Liveness probe. Returns build metadata and uptime."},
		{"GET /v1/openapi.json", "Auto-generated OpenAPI 3.1 document."},
		{"GET /v1/routes", "Debug: list of registered routes with their required scopes."},
	}
	for _, e := range extras {
		method, path := splitPattern(e.pattern)
		out = append(out, openAPIRoute{
			Method:  method,
			Path:    path,
			Pattern: e.pattern,
			Scope:   "",
			Summary: e.summary,
		})
	}
	for pat, scope := range routeScopes {
		method, path := splitPattern(pat)
		out = append(out, openAPIRoute{
			Method:  method,
			Path:    path,
			Pattern: pat,
			Scope:   scope,
			Summary: humanizePattern(pat),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

func splitPattern(pat string) (string, string) {
	parts := strings.SplitN(pat, " ", 2)
	if len(parts) != 2 {
		return "GET", pat
	}
	return parts[0], parts[1]
}

func humanizePattern(pat string) string {
	method, path := splitPattern(pat)
	trimmed := strings.TrimPrefix(path, "/v1")
	trimmed = strings.TrimPrefix(trimmed, "/")
	trimmed = strings.ReplaceAll(trimmed, "/", " ")
	trimmed = strings.ReplaceAll(trimmed, "{", "")
	trimmed = strings.ReplaceAll(trimmed, "}", "")
	switch method {
	case "POST":
		return "Create/execute " + trimmed
	case "GET":
		return "Read " + trimmed
	case "PUT":
		return "Update " + trimmed
	case "DELETE":
		return "Delete " + trimmed
	default:
		return method + " " + trimmed
	}
}

// pathParams extracts {name} placeholders from a route path and
// returns them as OpenAPI parameter objects. Order-preserving.
func pathParams(path string) []map[string]any {
	out := []map[string]any{}
	depth := 0
	buf := strings.Builder{}
	for _, r := range path {
		switch r {
		case '{':
			depth++
			buf.Reset()
		case '}':
			if depth > 0 {
				out = append(out, map[string]any{
					"name":     buf.String(),
					"in":       "path",
					"required": true,
					"schema":   map[string]any{"type": "string"},
				})
			}
			depth--
		default:
			if depth > 0 {
				buf.WriteRune(r)
			}
		}
	}
	return out
}

// GenerateOpenAPI produces the full spec as a Go map that can be
// json.Marshalled. Exported so tests and admin tools can inspect it
// without going through HTTP.
func GenerateOpenAPI() map[string]any {
	routes := enumerateRoutes()
	paths := map[string]any{}
	for _, rt := range routes {
		pathEntry, ok := paths[rt.Path].(map[string]any)
		if !ok {
			pathEntry = map[string]any{}
			paths[rt.Path] = pathEntry
		}
		op := map[string]any{
			"summary":     rt.Summary,
			"description": openAPIDescription(rt),
			"parameters":  pathParams(rt.Path),
			"responses": map[string]any{
				"200": responseObject("Success"),
				"400": responseObject("Invalid request"),
				"401": responseObject("Missing or invalid X-API-Key"),
				"403": responseObject("Missing required scope"),
				"404": responseObject("Not found"),
				"500": responseObject("Server error"),
			},
		}
		if rt.Method == "POST" || rt.Method == "PUT" {
			op["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": genericObjectSchema(rt.Pattern, true),
					},
				},
			}
		}
		if rt.Scope != "" {
			op["security"] = []map[string]any{{"ApiKeyAuth": []string{rt.Scope}}}
		} else {
			op["security"] = []map[string]any{}
		}
		pathEntry[strings.ToLower(rt.Method)] = op
	}
	return map[string]any{
		"openapi": openAPIVersion,
		"info": map[string]any{
			"title":       "CasperProver API",
			"version":     "1.0.0",
			"description": "Verifiable-compute API. Every mutating call requires X-API-Key; scoped keys additionally restrict which endpoints a caller may invoke.",
			"contact": map[string]any{
				"name": "anna-stolbovskaja",
			},
		},
		"servers": []map[string]any{
			{"url": "/", "description": "This host"},
		},
		"security": []map[string]any{
			{"ApiKeyAuth": []string{}},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiKeyAuth": map[string]any{
					"type": "apiKey",
					"in":   "header",
					"name": "X-API-Key",
				},
			},
			"schemas": map[string]any{
				"Error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error": map[string]any{"type": "string"},
					},
					"required": []string{"error"},
				},
				"WebhookSubscription": webhookSubscriptionSchema(),
			},
		},
		"paths":   paths,
		"x-webhooks": openAPIWebhookEnum(),
	}
}

func openAPIDescription(rt openAPIRoute) string {
	if rt.Scope != "" {
		return fmt.Sprintf("Requires X-API-Key with scope `%s`.", rt.Scope)
	}
	return "Open endpoint — no scope required."
}

func responseObject(desc string) map[string]any {
	return map[string]any{
		"description": desc,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/Error"},
			},
		},
	}
}

func genericObjectSchema(pattern string, req bool) map[string]any {
	if custom, ok := routeSchemas[pattern]; ok {
		if req && custom.RequestSchema != nil {
			return custom.RequestSchema
		}
		if !req && custom.ResponseSchema != nil {
			return custom.ResponseSchema
		}
	}
	return map[string]any{"type": "object", "additionalProperties": true}
}

func webhookSubscriptionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":                map[string]any{"type": "string"},
			"url":               map[string]any{"type": "string", "format": "uri"},
			"events":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"created_at":        map[string]any{"type": "string", "format": "date-time"},
			"attempts":          map[string]any{"type": "integer"},
			"deliveries":        map[string]any{"type": "integer"},
			"failures":          map[string]any{"type": "integer"},
			"last_attempt_at":   map[string]any{"type": "string", "format": "date-time"},
			"last_status_code":  map[string]any{"type": "integer"},
			"last_error":        map[string]any{"type": "string"},
		},
		"required": []string{"id", "url", "events", "created_at"},
	}
}

func openAPIWebhookEnum() map[string]any {
	return map[string]any{
		"events": KnownWebhookEvents,
		"headers": map[string]any{
			"X-CP-Signature": "sha256=<hex HMAC-SHA256 of body with subscription secret>",
			"X-CP-Timestamp": "RFC3339 dispatch timestamp",
			"X-CP-Event":     "event kind, matches events enum",
			"X-CP-Delivery":  "per-attempt id: <subscription_id>-<seq>",
		},
	}
}

// openAPIHandler serves the generated spec.
func (s *Server) openAPIHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(GenerateOpenAPI())
}

// routesHandler serves a compact, human-friendly enumeration of
// routes and their scopes. Useful for debugging without a JSON parser
// nearby.
func (s *Server) routesHandler(w http.ResponseWriter, _ *http.Request) {
	routes := enumerateRoutes()
	out := struct {
		Count  int            `json:"count"`
		Routes []openAPIRoute `json:"routes"`
	}{Count: len(routes), Routes: routes}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
