package sdk

// Request-scoped options and API-versioning helpers for the CasperProver SDK.
//
// These layer on top of the low-level Client transport and are consumed by
// the high-level primitives in primitives.go. They are intentionally small,
// stdlib-only, and add no dependencies.

// APIVersion controls the URL prefix used by high-level primitives.
// Empty ("") targets the unversioned legacy routes; "v1" targets /v1/... .
type APIVersion string

const (
	// APIVersionUnversioned targets the legacy unversioned routes.
	// Servers on >=e862a71 mark responses with Deprecation/Sunset/Link headers.
	APIVersionUnversioned APIVersion = ""
	// APIVersionV1 targets the /v1/... routes.
	APIVersionV1 APIVersion = "v1"
)

// WithAPIVersion pins the client to a specific API version. Defaults to v1.
func WithAPIVersion(v APIVersion) ClientOption {
	return func(c *Client) { c.apiVersion = v }
}

// prefix returns the URL prefix for the currently configured API version
// (e.g. "" or "/v1"). Safe to concatenate with a leading-slash path.
func (c *Client) prefix() string {
	if c.apiVersion == "" || c.apiVersion == APIVersionUnversioned {
		return ""
	}
	return "/" + string(c.apiVersion)
}

// RequestOption tweaks a single request without changing client-wide state.
type RequestOption func(*requestConfig)

type requestConfig struct {
	// idempotencyKey, when set, is sent as the X-Idempotency-Key header. The
	// server-side middleware (see engine/internal/api middleware) will
	// deduplicate retries with the same key + body for 24h.
	idempotencyKey string
	// extraHeaders is an optional bag of per-request headers.
	extraHeaders map[string]string
}

// WithIdempotencyKey attaches X-Idempotency-Key to a single request. Retry the
// same call with the same key + body to get a bit-identical replay; different
// body under the same key returns 409.
func WithIdempotencyKey(key string) RequestOption {
	return func(rc *requestConfig) { rc.idempotencyKey = key }
}

// WithHeader attaches an arbitrary header to a single request. Useful for
// X-Public-Key on revoke-style routes, or bespoke tenant headers.
func WithHeader(name, value string) RequestOption {
	return func(rc *requestConfig) {
		if rc.extraHeaders == nil {
			rc.extraHeaders = map[string]string{}
		}
		rc.extraHeaders[name] = value
	}
}

func buildRequestConfig(opts []RequestOption) requestConfig {
	rc := requestConfig{}
	for _, opt := range opts {
		opt(&rc)
	}
	return rc
}
