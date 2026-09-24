// Package api — routing prefix (/v1) alias and deprecation header injection.
//
// Task 7.4: expose every mutating + read endpoint under /v1/... as the
// stable versioned path. Old, unprefixed paths keep working but respond
// with `X-CP-Deprecation: <ISO-8601 sunset date>` so integrators see the
// warning in logs and can migrate.
//
// Task 7.13: sunset date is a soft signal, not enforced by the server. The
// api CHANGELOG documents the timeline.

package api

import (
	"net/http"
	"strings"
)

// v1SunsetDate is the ISO-8601 date after which the unprefixed paths are
// expected to be removed. It's a signalling value — the router keeps
// serving both prefixes until the day the codebase drops the alias.
const v1SunsetDate = "2027-01-01"

// v1AliasMiddleware rewrites any request path that already starts with
// `/v1/` down to its unprefixed form BEFORE routing, and stamps a
// `X-CP-API-Version` response header for observability.
//
// The rewrite is one-way: internal handlers keep their original patterns
// (`/proofs`, `/verify`, …), so no downstream code has to change to accept
// v1 traffic.
func (s *Server) v1AliasMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CP-API-Version", "v1")
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			// Strip the prefix, but keep the leading slash. `/v1/proofs`
			// becomes `/proofs`.
			r.URL.Path = r.URL.Path[len("/v1"):]
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
			// Also update RawPath so mux path matching is consistent
			// with URL.Path (net/http mux uses URL.Path).
			if r.URL.RawPath != "" && strings.HasPrefix(r.URL.RawPath, "/v1/") {
				r.URL.RawPath = r.URL.RawPath[len("/v1"):]
			}
			next.ServeHTTP(w, r)
			return
		}
		// Old-style unprefixed request. Emit deprecation header.
		if r.URL.Path != "/health" && !strings.HasPrefix(r.URL.Path, "/v1/") {
			w.Header().Set("X-CP-Deprecation", v1SunsetDate)
			w.Header().Set("Sunset", v1SunsetDate)
			w.Header().Set(
				"Link",
				`</v1`+r.URL.Path+`>; rel="successor-version"`,
			)
		}
		next.ServeHTTP(w, r)
	})
}
