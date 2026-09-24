# Provenance Lineage — DecisionReceipt / W3C-VC / Agent-Receipt / OTel

Ref: `docs/roadmap/PROVENANCE.md` (30-day plan §D).
Shipped in commit *(this PR)*, Pack AR / backlog items **5.1**, **5.2**, **5.3**.

## What ships

The `engine/internal/receipts/` package is the honest-scope implementation of
the provenance-lineage layer. It signs each decision with the active PQ
signing key (default `hybrid_ed25519_mldsa65`) and emits the same signed
receipt in three interoperable shapes:

- **Internal (`DecisionReceipt`)** — the canonical JSON produced by
  `Service.Emit`. Source of truth for the signature calculation.
- **W3C Verifiable Credentials 2.0** — `receipts.ToW3CVC()`. Maps onto
  a VC whose `credentialSubject.id` is the decision id, whose
  `issuer` is the engine's DID, and whose `proof.proofValue` carries
  the ML-DSA+Ed25519 signature. Includes `cp:canonical_hash` under
  `credentialSubject` so a downstream can rehash without importing
  the Go package.
- **Agent Receipt (agentreceipts.org draft v0.3)** — `receipts.ToAgentReceipt()`.
  A superset of the facet outputs, an evidence pointer at the prover
  merkle root, and a `cp_extra` bag for engine-private fields.

The three shapes are **lossless in one direction only**: internal →
W3C-VC and internal → agent-receipt. Round-tripping back to internal
is not supported. Rationale: receipts are produced, not consumed, by
CasperProver — omitting the reverse path keeps the surface small and
makes the canonical hash the only thing that ever has to be
recomputed by a verifier.

## Canonical hash and signature

Every receipt has a canonical, sort-normalised, length-prefixed byte
form (`receipts.CanonicalHash`). The signature covers that hash,
NOT the JSON serialisation. Facets are sorted by `kind`
lexicographically; provider receipts are sorted by `receipt_hash`
lexicographically; timestamps are formatted as
`2006-01-02T15:04:05.000000000Z` in UTC. A downstream verifier that
re-serialises the JSON in a different key order still produces the
same hash.

The Proof field is deliberately excluded from the hash: including a
self-reference would be circular.

## HTTP surface

All endpoints are opt-in via `CP_RECEIPTS_ENABLE=1`. When disabled
they return 503 with a well-formed JSON error, never fall through to
a silent no-op.

- `POST /v1/receipts/emit` (`receipts:write`)
  Body: a fully-formed decision commit (submitter, spec_id,
  payload_hex, nonce, aggregate, facets), plus optional evidence
  root, model id, upstream provider receipts, and HITL resolution.
  Returns 201 with the signed `DecisionReceipt`.

- `GET /v1/receipts/{id}` (`receipts:read`)
  Returns the canonical `DecisionReceipt` JSON.

- `GET /v1/receipts/{id}/lineage?max_depth=N` (`receipts:read`)
  Returns `{ root, ancestors, depth }`. `max_depth` defaults to 8,
  hard-capped at 32. Missing upstream receipts are skipped (not
  errored) so a partial store still returns a partial graph.

- `GET /v1/receipts/{id}/w3c-vc` (`receipts:read`)
  Returns the W3C VC 2.0 shape.

- `GET /v1/receipts/{id}/agent-receipt` (`receipts:read`)
  Returns the agentreceipts.org 0.3 shape.

## OpenTelemetry bridging

The `receipts.OtelSink` interface is deliberately narrow: `Record(ctx,
receipt) error`. Default is `NoopSink()`. A JSONL implementation
(`NewJSONLSink(path)`) is shipped for the demo path — one JSON object
per line with the OTel-compatible attribute names:

```
cp.receipt_id      cp.subject          cp.hitl
cp.receipt_hash    cp.spec_id          cp.hitl_action
cp.issued_at       cp.verdict          cp.hitl_ticket
cp.issuer          cp.confidence       cp.hitl_reviewer
cp.evidence_root   cp.model_id         cp.facet_count
cp.vetoed_by       cp.provider_count
```

A production deployment plugs an OTel-native implementation in via
the same interface — a Span emitted per receipt, attributes set from
the map above, links to upstream span ids constructed from the
lineage graph.

To wire the JSONL sink now:
```
CP_RECEIPTS_ENABLE=1
CP_RECEIPTS_JSONL=/var/log/cp-receipts.jsonl
CP_RECEIPTS_ISSUER_DID=did:cp:engine-prod-01   # optional; default derives from active key id
```

Vector / Fluent Bit / OpenTelemetry Collector can then tail the file
and forward to any OTel receiver. See the "OTel Collector wiring"
appendix for a reference `otel-collector-config.yaml`.

## Threat model

What the receipt IS:
- A cryptographic binding of {aggregate, facets, evidence_root,
  model_id, HITL resolution} under the engine's active signing key.
  A downstream that trusts the engine's public key can verify the
  binding independently.
- A pointer at upstream provider receipts by canonical hash. A
  downstream that walks the lineage graph can independently
  reconstruct which providers were consulted.
- Interoperable with W3C VC 2.0 verifiers and AR 0.3 clients.

What the receipt IS NOT:
- A proof that the engine ran the aggregation honestly. That's the
  job of the on-chain Groth16 proof (see `docs/ZK_VERIFICATION.md`
  and the verifier-gate contract).
- A proof that the upstream providers evaluated honestly. That
  requires each provider to sign its own facet outputs. Roadmap.
- A revocation mechanism. Receipts are append-only; a receipt
  emitted by a compromised key stays valid to a naive verifier
  until the key is rotated (`/v1/pq/keys/rotate`). Downstream
  verifiers SHOULD consult the verifier-gate contract for the
  active-key epoch before accepting a receipt.
- A retention policy. Receipts live in the process store by default;
  a deployment with a compliance requirement plugs a Postgres-backed
  `receipts.Store` in via the interface. See `docs/roadmap/LEGAL.md`
  for the retention design.

## Environment

| Env                          | Default            | Purpose                                                        |
|------------------------------|--------------------|----------------------------------------------------------------|
| `CP_RECEIPTS_ENABLE`         | unset (disabled)   | Master switch — `1` enables the routes                         |
| `CP_RECEIPTS_ISSUER_DID`     | derived from key   | Override issuer DID; default is `did:cp:<active-key-id>`       |
| `CP_RECEIPTS_JSONL`          | unset (noop sink)  | Path to JSONL audit file; parent dirs created eagerly          |
| `CP_RECEIPTS_STORE`          | `memory`           | Reserved — future Postgres-backed store selector               |

## Out of scope for this change

- Postgres-backed `Store` implementation. Interface is ready;
  writing the driver is a separate ticket (backlog 5.4).
- Provider-side signatures. Each provider in the pool would need a
  keystore of its own; that's a design commitment that spans the
  A2A layer (backlog 3.5 follow-up).
- Long-term retention policy. See `docs/roadmap/LEGAL.md`.
- Bridging to non-W3C schemas (CACAO / EIP-4361, AR ≥ 1.0). Roadmap.

## References

- Design plan: `docs/roadmap/PROVENANCE.md`
- W3C VC 2.0: <https://www.w3.org/TR/vc-data-model-2.0/>
- Agent Receipts draft: <https://agentreceipts.org/spec/v0.3>
- OpenTelemetry semantic conventions: <https://opentelemetry.io/docs/specs/semconv/>
