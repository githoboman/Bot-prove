# CasperProver — Secret Handling Policy

*Backlog 10.4.* How CasperProver classifies, stores, rotates, and
audits every secret it touches. The rules apply to the deployed
service, the SDK examples, and any developer running the codebase
locally.

## Classification

| Class            | Examples                                              | Storage                                                        | Rotation |
|------------------|-------------------------------------------------------|----------------------------------------------------------------|----------|
| **T1 — chain key** | Owner secret keys for deployed Casper contracts       | Offline hardware wallet or an env-only PEM never committed     | 90 d     |
| **T2 — API key**   | `API_KEY` env var (server)                            | Env only; never checked in; injected via secret manager        | 30 d     |
| **T3 — model key**  | Third-party LLM provider keys (OpenAI, Anthropic, …) | Server env only; NEVER passed to clients                       | 90 d     |
| **T4 — signed url** | Short-lived S3/GCS artefact URLs                      | Ephemeral memory                                               | ≤ 15 min |
| **T5 — telemetry**  | Session IDs, request IDs, trace IDs                   | Logs — low sensitivity                                        | —        |

## Where secrets MUST NOT appear

1. **Git history.** Every push runs gitleaks in CI (see `.github/workflows/secret-scan.yml`). A hit blocks merge.
2. **Client bundles.** Vite build is scanned for anything matching `sk-`, `ghp_`, `SECRET`, `PRIVATE KEY`. See `docs/BUILD_HARDENING.md` (planned).
3. **Log lines.** `engine/internal/decision.redactMetadata` scrubs metadata keys matching `api_key|authorization|cookie|password|secret|token|pii|email|phone|ssn`.
4. **Response bodies.** Even error paths never echo request headers back.
5. **Screenshots or demo videos.** Every demo capture uses fixture keys committed under `testdata/fixtures/keys.example` (dummy 32-byte zeros).

## Storage

- **Local dev.** `.env` (gitignored), never `.env.example`. `.env.example` carries key NAMES only.
- **CI.** GitHub Actions secrets, scoped per environment. No `secrets.*` referenced outside a job whose runner is the intended env.
- **Production.** Managed secret store (planned: AWS Secrets Manager or GCP Secret Manager). The API server reads from env at boot only — no re-read on request path.

## Rotation

- Automated for T4 (signed URLs) by construction.
- Manual for T1–T3 per the table. A rotation entry lands in
  `docs/rotation-log.md` (planned) with `old_hash → new_hash` and
  the operator's initials.
- Emergency rotation: `docs/RUNBOOK_ROTATE.md` (planned).

## Recovery

- **Owner key lost:** the timelocked governance contract's
  `recover_owner` entry-point lets a multi-sig quorum re-establish
  ownership after the timelock window. Design in `docs/OWNER_LIFECYCLE.md`.
- **API key leaked:** revoke via env update + roll the deployment.
  Every issued proof carries `api_version` + `key_id` so old-key
  requests can be denied server-side.
- **Chain key compromise:** contracts anchored by the compromised
  key are frozen via `emergency_pause` (backlog 1.10, 1.11).

## Detection

- **gitleaks** — pre-push hook + CI job on every push.
- **trufflehog** — nightly scan of the full history via
  `.github/workflows/deep-secret-scan.yml` (planned).
- **CycloneDX SBOM diff** — alerts on new dependencies that bundle
  known-secret-leaking transitive packages.
- **Log sink filter** — any log line matching the redaction regex
  above is dropped at the stdlib slog handler before emission.

## Audit trail

Every access to a T1–T3 secret must produce an entry in the
decision-log sink (`internal/decision`) with:

- `metadata["secret_class"] = "T1|T2|T3"`
- `metadata["accessor"]` (agent id, not the key value)
- **NO** field carrying the value itself.

A record with `secret_class` set and a populated `trace_preview` is
a bug and MUST fail CI — tracked as invariant I-SEC-1 in
`docs/CONTRACT_INVARIANTS.md`.
