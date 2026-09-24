// Package sdk provides a Go client for the CasperProver REST API and an
// MCP (Model Context Protocol) stdio server exposing a subset of it as
// LLM-callable tools.
//
// Client methods map 1:1 to routes in engine/internal/api/server.go; see
// docs/openapi.yaml for the authoritative endpoint list. Not every route has
// a Client method yet - PRs welcome.
package sdk

// Version is the SDK's own version, independent of the API server version.
const Version = "0.4.0"
