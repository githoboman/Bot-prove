package sdk

// Response payloads from the CasperProver API are intentionally returned as
// map[string]any by Client methods (see client.go) rather than fixed structs:
// several endpoints (proofs, aggregation batches, inference results) return
// server-side maps whose optional fields vary by state (e.g. a batch only has
// aggregate_proof_hash once finalized). Decoding into map[string]any avoids
// silently dropping fields or fighting Go's JSON decoder over optional keys.
//
// If you need a stable typed response for a specific endpoint, decode the
// map yourself at the call site, or open an issue - see docs/openapi.yaml
// for the authoritative response shapes.
