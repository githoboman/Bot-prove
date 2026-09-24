package api

import (
	"net/http"
	"strings"
)

// Item 7.5 — Accept-header versioning.
//
// Runs in parallel to path-versioning (`/v1/…`). A client MAY negotiate
// the wire version via:
//
//   Accept: application/vnd.cp+json; version=1
//   Accept: application/vnd.cp+json; version=1, application/json
//
// The middleware:
//   - Parses every Accept variant present on the request.
//   - If a `vnd.cp+json` variant is present with a version we do NOT
//     serve, respond 406 Not Acceptable so the client picks a match.
//   - Otherwise, stamps the negotiated version onto every response
//     via `X-CP-API-Version` and echoes it back inside the standard
//     `Vary: Accept` header so intermediary caches don't collapse.
//
// The default served version is v1. Adding v2 later means appending
// its number to servedVersions and letting v1 clients keep working.

const (
	cpMediaType = "application/vnd.cp+json"
)

var servedVersions = []string{"1"} // ordered oldest → newest

// negotiateVersion inspects the Accept header and returns the version
// the server WILL serve, together with whether the client explicitly
// asked for our media type.
//
// Rules:
//   - No Accept, or `*/*`, or plain `application/json` → default (latest served, `explicit=false`).
//   - Any `application/vnd.cp+json; version=N` matches → serve N (explicit=true).
//   - Any `application/vnd.cp+json` with no version param → default (explicit=true).
//   - `application/vnd.cp+json; version=N` with N unknown → return `"", true`
//     so the caller can respond 406.
func negotiateVersion(header string) (version string, explicit bool) {
	defaultV := servedVersions[len(servedVersions)-1]
	if strings.TrimSpace(header) == "" {
		return defaultV, false
	}
	requestedButUnknown := false
	for _, part := range strings.Split(header, ",") {
		media, params := parseMediaType(part)
		if media != cpMediaType {
			continue
		}
		v, ok := params["version"]
		if !ok {
			// vnd.cp+json requested, version omitted → default.
			return defaultV, true
		}
		for _, sv := range servedVersions {
			if sv == v {
				return sv, true
			}
		}
		requestedButUnknown = true
	}
	if requestedButUnknown {
		return "", true
	}
	// No vnd.cp+json variant found — fall back to default.
	return defaultV, false
}

func parseMediaType(s string) (media string, params map[string]string) {
	params = map[string]string{}
	segs := strings.Split(strings.TrimSpace(s), ";")
	if len(segs) == 0 {
		return "", params
	}
	media = strings.ToLower(strings.TrimSpace(segs[0]))
	for _, seg := range segs[1:] {
		seg = strings.TrimSpace(seg)
		eq := strings.IndexByte(seg, '=')
		if eq <= 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(seg[:eq]))
		v := strings.Trim(strings.TrimSpace(seg[eq+1:]), `"`)
		params[k] = v
	}
	return media, params
}

// acceptVersionMiddleware negotiates the wire version and stamps the
// response.
func (s *Server) acceptVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, explicit := negotiateVersion(r.Header.Get("Accept"))
		if v == "" {
			// Explicit unknown version → 406.
			w.Header().Set("Vary", "Accept")
			s.jsonError(
				w,
				"unsupported api version — supported: "+strings.Join(servedVersions, ", "),
				http.StatusNotAcceptable,
			)
			return
		}
		w.Header().Set("X-CP-API-Version", v)
		if existing := w.Header().Get("Vary"); existing == "" {
			w.Header().Set("Vary", "Accept")
		} else if !strings.Contains(existing, "Accept") {
			w.Header().Set("Vary", existing+", Accept")
		}
		_ = explicit // reserved for future observability
		next.ServeHTTP(w, r)
	})
}
