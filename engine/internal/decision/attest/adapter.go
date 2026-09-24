package attest

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// HTTPProviderAdapter is a real, deterministic Provider that speaks a
// small JSON-over-HTTP protocol to any external evaluator. The wire
// format is documented in `docs/DECISION_A2A_HITL.md` and is the same
// contract the fixture provider implements against its map.
//
// The adapter is deterministic in the sense that it forwards the exact
// canonical Decision.ID() to the evaluator: two calls with the same
// Decision produce the same request, and any deterministic evaluator on
// the other end will produce the same response.  The adapter itself
// performs no interpretation of payload semantics.
//
// If unconfigured (Endpoint == ""), Evaluate falls back to the provided
// Fallback provider — this is the "deterministic fixture fallback" path
// described in Pack AQ 3.1: the demo, the reproducer script and CI can
// run without any external HTTP dependency, but a production deployment
// only has to set CP_DECISION_PROVIDER_URL to swap in a real evaluator.

// HTTPProviderConfig configures the adapter. Zero values are legal — a
// zero config produces an adapter that always falls back to Fallback.
type HTTPProviderConfig struct {
	// Name overrides the adapter's identity in receipts. Defaults to
	// "http-adapter".
	Name string
	// Endpoint is the URL of the remote evaluator. Empty ⇒ fallback.
	Endpoint string
	// Token is optional bearer auth added as `Authorization: Bearer …`.
	Token string
	// Timeout is a per-request timeout. Defaults to 5s. Zero ⇒ default.
	Timeout time.Duration
	// Client optionally overrides the underlying *http.Client.
	Client *http.Client
	// Fallback is invoked when the adapter is unconfigured OR the
	// remote returns a transport error. If nil, a fresh
	// NewFixtureProvider() is used.
	Fallback Provider
	// AllowedFacets restricts which facets we return from the remote
	// response. Anything outside the set is dropped. Empty ⇒ all facets.
	AllowedFacets []FacetKind
}

// HTTPProviderAdapter is the constructed Provider.
type HTTPProviderAdapter struct {
	cfg     HTTPProviderConfig
	client  *http.Client
	allowed map[FacetKind]struct{}
}

// NewHTTPProviderAdapter builds an adapter from cfg. It never returns an
// error — misconfiguration is deferred to Evaluate, where it manifests
// as a fallback path so the pool stays operational.
func NewHTTPProviderAdapter(cfg HTTPProviderConfig) *HTTPProviderAdapter {
	if cfg.Name == "" {
		cfg.Name = "http-adapter"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	if cfg.Fallback == nil {
		cfg.Fallback = NewNamedFixtureProvider(cfg.Name + "-fallback")
	}
	allowed := make(map[FacetKind]struct{}, len(cfg.AllowedFacets))
	for _, k := range cfg.AllowedFacets {
		allowed[k] = struct{}{}
	}
	return &HTTPProviderAdapter{cfg: cfg, client: client, allowed: allowed}
}

// NewHTTPProviderAdapterFromEnv is the deployment-friendly constructor.
// It reads CP_DECISION_PROVIDER_URL, CP_DECISION_PROVIDER_NAME and
// CP_DECISION_PROVIDER_TOKEN. If URL is empty, the returned adapter is
// unconfigured (fixture fallback only).
func NewHTTPProviderAdapterFromEnv() *HTTPProviderAdapter {
	return NewHTTPProviderAdapter(HTTPProviderConfig{
		Name:     strings.TrimSpace(os.Getenv("CP_DECISION_PROVIDER_NAME")),
		Endpoint: strings.TrimSpace(os.Getenv("CP_DECISION_PROVIDER_URL")),
		Token:    strings.TrimSpace(os.Getenv("CP_DECISION_PROVIDER_TOKEN")),
	})
}

// Name implements Provider.
func (a *HTTPProviderAdapter) Name() string { return a.cfg.Name }

// Configured reports whether the adapter has a real endpoint. Useful for
// tests and diagnostics.
func (a *HTTPProviderAdapter) Configured() bool { return a.cfg.Endpoint != "" }

// evaluateRequest is the JSON body sent to the remote evaluator.
type evaluateRequest struct {
	DecisionID  string `json:"decision_id"`
	Submitter   string `json:"submitter"`
	SpecID      string `json:"spec_id"`
	Nonce       uint64 `json:"nonce"`
	PayloadHex  string `json:"payload_hex"`
	SubmittedAt string `json:"submitted_at"`
}

// evaluateResponseFacet mirrors FacetVerdict on the wire, with the
// verdict encoded as a small enum string ("APPROVE"|"ABSTAIN"|"REJECT").
type evaluateResponseFacet struct {
	Kind       string  `json:"kind"`
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// evaluateResponse is the JSON body the remote evaluator returns.
type evaluateResponse struct {
	// Verdicts is the ordered list of per-facet verdicts.
	Verdicts []evaluateResponseFacet `json:"verdicts"`
}

// Evaluate implements Provider. If the adapter is unconfigured, or the
// remote call fails at the transport layer (network error, non-2xx
// status, malformed JSON), we fall back to the configured Fallback
// provider. This deliberately fail-open behaviour is safe because the
// Byzantine-robust aggregator still applies at the pool level; a lone
// fallback vote cannot flip a critical facet against an honest quorum.
func (a *HTTPProviderAdapter) Evaluate(ctx context.Context, d Decision) ([]FacetVerdict, error) {
	if !a.Configured() {
		return a.cfg.Fallback.Evaluate(ctx, d)
	}

	body, err := json.Marshal(evaluateRequest{
		DecisionID:  d.ID(),
		Submitter:   d.Submitter,
		SpecID:      d.SpecID,
		Nonce:       d.Nonce,
		PayloadHex:  hex.EncodeToString(d.Payload),
		SubmittedAt: d.SubmittedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return a.cfg.Fallback.Evaluate(ctx, d)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return a.cfg.Fallback.Evaluate(ctx, d)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if a.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.Token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return a.cfg.Fallback.Evaluate(ctx, d)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain a small amount of body for diagnostic completeness,
		// then fall back.
		_, _ = io.CopyN(io.Discard, resp.Body, 1024)
		return a.cfg.Fallback.Evaluate(ctx, d)
	}

	var parsed evaluateResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return a.cfg.Fallback.Evaluate(ctx, d)
	}

	out := make([]FacetVerdict, 0, len(parsed.Verdicts))
	for _, f := range parsed.Verdicts {
		kind := FacetKind(f.Kind)
		if len(a.allowed) > 0 {
			if _, ok := a.allowed[kind]; !ok {
				continue
			}
		}
		v, err := parseVerdictString(f.Verdict)
		if err != nil {
			// Silently skip unparseable verdicts; Judge will fill
			// missing kinds as ABSTAIN.
			continue
		}
		conf := f.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}
		out = append(out, FacetVerdict{
			Kind:       kind,
			Verdict:    v,
			Confidence: conf,
			Reason:     f.Reason,
		})
	}
	return out, nil
}

// parseVerdictString converts the wire string form back into a Verdict.
func parseVerdictString(s string) (Verdict, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "APPROVE":
		return VerdictApprove, nil
	case "ABSTAIN":
		return VerdictAbstain, nil
	case "REJECT":
		return VerdictReject, nil
	default:
		return VerdictUnknown, fmt.Errorf("decision: unknown verdict string %q", s)
	}
}

// verdictWireString is the inverse; exported for the HITL package.
func verdictWireString(v Verdict) string {
	switch v {
	case VerdictApprove:
		return "APPROVE"
	case VerdictAbstain:
		return "ABSTAIN"
	case VerdictReject:
		return "REJECT"
	default:
		return "UNKNOWN"
	}
}

// Compile-time assertion.
var _ Provider = (*HTTPProviderAdapter)(nil)

// ErrRemoteUnconfigured is returned from RemoteRequired when the adapter
// has no endpoint. It is only used by tests and diagnostics; production
// code always tolerates unconfigured adapters via the fallback path.
var ErrRemoteUnconfigured = errors.New("decision: HTTP provider adapter is unconfigured")

// RemoteRequired returns ErrRemoteUnconfigured if the adapter is
// operating in fallback-only mode. Diagnostic aid for tests that need
// to guarantee the real path was exercised.
func (a *HTTPProviderAdapter) RemoteRequired() error {
	if !a.Configured() {
		return ErrRemoteUnconfigured
	}
	return nil
}
