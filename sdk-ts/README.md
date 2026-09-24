# @casperprover/sdk

**TypeScript SDK for [CasperProver](../)** — HTTP client for the proof-generation API plus an *offline* Merkle-inclusion verifier that runs the same BLAKE2b-256 the server uses.

The SDK is deliberately **zero-build**: the source ships as raw `.ts` files, and every runtime we target (Node 22+, Node 24 with native TS, Bun, Deno, Vite, esbuild) either imports `.ts` directly or hands it to a bundler untouched. There is no `dist/`, no compile step, no publish-and-hope pipeline. Consume it exactly like the AE402 TS SDK.

## Install

Right now this is consumed in-tree from the CasperProver monorepo. Once the package is published:

```bash
npm install @casperprover/sdk
# or
pnpm add @casperprover/sdk
```

Node 18+ is required (built-in `fetch`). If you're on Node 22.6+ you also get native TypeScript loading via `--experimental-strip-types`; on Node 24+ it's on by default.

## Quickstart

```ts
import { CasperProverClient, verifyOffline } from "@casperprover/sdk";

const cp = new CasperProverClient({
  baseUrl: "https://cp.example.com",
  apiKey: process.env.CP_API_KEY,      // optional; sent as X-API-Key
  publicKey: process.env.CP_PUBLIC_KEY, // optional; sent as X-Public-Key
});

// 1. Generate a proof server-side.
const proof = await cp.generateProof({
  agent: "agent-42",
  input: "prompt bytes",
  output: "model output bytes",
  model: "gpt-4o-2026-05-13",
  use_case: "content-moderation",
  mode: "local",
});

// 2. Ask the server whether it thinks the proof is still valid.
const verdict = await cp.verifyProof({
  proof_id: proof.id,
  input: "prompt bytes",
  output: "model output bytes",
  model: "gpt-4o-2026-05-13",
});
console.log(verdict.verified, verdict.checks);

// 3. Independently reconstruct the Merkle root — no trust in the server.
const report = verifyOffline(proof, "prompt bytes", "model output bytes", "gpt-4o-2026-05-13");
if (!report.overallValid) {
  throw new Error("proof does not reconstruct on the client");
}
```

## API surface

| Method | HTTP route | Purpose |
|--------|------------|---------|
| `health()` | `GET /health` | Liveness probe |
| `generateProof(req)` | `POST /proofs` | Submit input/output/model, receive a proof record |
| `batchGenerate({ proofs, mode })` | `POST /proofs/batch` | Up to 50 proofs in one call |
| `verifyProof(req)` | `POST /verify` | Ask the server to verify a proof (optionally with echo) |
| `batchVerify(ids[])` | `POST /verify` (sequenced) | Convenience — verifies each ID and returns the array |
| `getProof(id)` | `GET /proofs/{id}` | Fetch a proof record by ID |
| `getProofStatus(id)` | `GET /proofs/{id}` | Convenience — returns `"valid" \| "revoked" \| "invalid"` |
| `revokeProof(id)` | `POST /proofs/{id}/revoke` | Mark a proof revoked |
| `listProofs(query?)` | `GET /proofs` | Paginated listing with `agent`, `public_key`, `mode` filters |

Offline verifier (no network):

| Function | Purpose |
|----------|---------|
| `blake2b256(bytes)` | 256-bit BLAKE2b, matches Go's `x/crypto/blake2b.Sum256` |
| `blake2b256OfString(s)` | UTF-8 encode + hash |
| `computeMerkleRoot(leafHex, pathHex, leafIndex)` | Reconstruct root from a leaf + siblings |
| `verifyMerkleInclusion(proof, leafHex)` | Verify a specific leaf against `proof.merkle_root` |
| `verifyOffline(proof, input, output, model)` | Full offline report — hash checks + Merkle checks for all four leaves |

## Errors

Every HTTP failure is mapped to a typed exception so callers can `catch` the ones that matter:

- `BadRequestError` (400) · `UnauthorizedError` (401) · `ForbiddenError` (403) · `NotFoundError` (404)
- `RateLimitError` (429) — carries `retryAfterSec` when the server sent `Retry-After`
- `ServerError` (5xx) · `APIError` (other 4xx)
- `NetworkError` — connection refused, DNS failure, `AbortError` on timeout
- All extend `CasperProverError`, all extend `Error`

The client's `error.body` field carries the parsed response body when available, so callers can pull out server-specific fields without reparsing.

## Development

```bash
cd sdk-ts
npm install                  # only `typescript` + `@types/node`; no runtime deps
npm run test                 # node --test on all suites; ~40 tests, ~150ms
npm run typecheck            # tsc --noEmit
```

Tests use Node's built-in `node:test` runner and a hand-rolled fake `fetch` — no MSW, no jest, no vitest. Everything runs anywhere Node 22+ runs.

## Parity with other clients

| Endpoint | Go client | TS client (this SDK) |
|----------|-----------|----------------------|
| `/health` | ✅ | ✅ |
| `POST /proofs` | ✅ | ✅ |
| `POST /proofs/batch` | ✅ | ✅ |
| `POST /verify` | ✅ | ✅ |
| `GET /proofs/{id}` | ✅ | ✅ |
| `POST /proofs/{id}/revoke` | ✅ | ✅ |
| `GET /proofs` | ✅ | ✅ |
| Offline Merkle verifier | — (server-side only) | ✅ (BLAKE2b + path reconstruction) |
| Typed errors | error strings | class hierarchy |
| Retry-After parsing | — | ✅ |

Admin/KYC/aggregation/ZK/PQ endpoints are intentionally *not* exposed by the SDK — those are operator surfaces, not client-facing.

## Design notes

- **Zero build.** Deliberate. Adds one less thing that can drift.
- **No transitive deps.** `fetch` is native; BLAKE2b is bundled (~120 LOC). The only devDep is `typescript` for `--noEmit` checks.
- **Injectable `fetch`.** Tests use a fake; consumers can wrap in a retry/backoff decorator without patching globals.
- **Structured errors.** Callers should never have to `if (e.message.includes("404"))`.
- **Match Go on the wire.** JSON field names mirror `engine/internal/prover/types.go` so a Go response can be handed to the TS type without renaming.

## License

MIT
