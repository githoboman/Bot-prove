/**
 * TypeScript client for the CasperProver HTTP API.
 *
 * Zero-build: consumed directly as `.ts` files via `tsx` / `vitest` / Node 24+'s
 * native TypeScript loader. See `sdk-ts/README.md`.
 *
 * The client mirrors the Go client under `engine/pkg/client/` and the routes
 * declared in `engine/internal/api/server.go`. Only the *public* surface is
 * exposed — admin/KYC/aggregation endpoints are not part of the SDK contract.
 */

import { errorForStatus, NetworkError, RateLimitError } from "./errors.ts";
import type {
  BatchProofsRequest,
  GenerateProofRequest,
  HealthResponse,
  ListProofsQuery,
  ListProofsResponse,
  ProofRecord,
  ProofStatus,
  VerifyProofRequest,
  VerifyProofResponse,
} from "./types.ts";

/** Options for constructing a client. */
export interface CasperProverClientOptions {
  /** Base URL of the CasperProver API, no trailing slash needed (default `http://localhost:8080`). */
  baseUrl?: string;
  /** Optional API key. When set, sent as `X-API-Key: <key>`. */
  apiKey?: string;
  /** Optional ed25519 public key (hex) for `X-Public-Key`. */
  publicKey?: string;
  /** Per-request timeout in milliseconds (default 30_000). */
  timeoutMs?: number;
  /** Custom fetch implementation (mainly for tests). Defaults to `globalThis.fetch`. */
  fetch?: typeof fetch;
}

/** Convenience: derive a coarse status from a `ProofRecord`. */
export function proofStatus(p: ProofRecord): ProofStatus {
  if (p.revoked) return "revoked";
  if (p.valid) return "valid";
  return "invalid";
}

export class CasperProverClient {
  private readonly baseUrl: string;
  private readonly apiKey?: string;
  private readonly publicKey?: string;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof fetch;

  constructor(options: CasperProverClientOptions = {}) {
    this.baseUrl = (options.baseUrl ?? "http://localhost:8080").replace(/\/+$/, "");
    this.apiKey = options.apiKey;
    this.publicKey = options.publicKey;
    this.timeoutMs = options.timeoutMs ?? 30_000;
    const f = options.fetch ?? globalThis.fetch;
    if (typeof f !== "function") {
      throw new Error(
        "no fetch() available — pass a custom fetch via options.fetch or run on Node 18+ / a modern browser",
      );
    }
    this.fetchImpl = f;
  }

  // -------------------------------------------------------------------------
  // Internal transport

  private buildHeaders(extra?: Record<string, string>): Record<string, string> {
    const h: Record<string, string> = {
      "Accept": "application/json",
      ...(extra ?? {}),
    };
    if (this.apiKey) h["X-API-Key"] = this.apiKey;
    if (this.publicKey) h["X-Public-Key"] = this.publicKey;
    return h;
  }

  private async request<T>(
    method: "GET" | "POST" | "DELETE",
    path: string,
    body?: unknown,
  ): Promise<T> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);

    const headers = this.buildHeaders(
      body === undefined ? undefined : { "Content-Type": "application/json" },
    );

    let resp: Response;
    try {
      resp = await this.fetchImpl(`${this.baseUrl}${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
      });
    } catch (err) {
      throw new NetworkError(
        err instanceof Error ? `network error: ${err.message}` : "network error",
        err,
      );
    } finally {
      clearTimeout(timer);
    }

    // Best-effort body parse: some routes return JSON, some plain text on error.
    const text = await resp.text();
    let parsed: unknown = undefined;
    if (text.length > 0) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = text;
      }
    }

    if (!resp.ok) {
      let message = `HTTP ${resp.status}`;
      if (parsed && typeof parsed === "object") {
        const rec = parsed as Record<string, unknown>;
        if (typeof rec.error === "string") message = rec.error;
        else if (typeof rec.message === "string") message = rec.message;
      } else if (typeof parsed === "string" && parsed.length > 0) {
        message = parsed;
      }

      const err = errorForStatus(resp.status, message, parsed);
      if (err instanceof RateLimitError) {
        const hdr = resp.headers.get("Retry-After");
        if (hdr !== null) {
          const secs = Number.parseInt(hdr, 10);
          if (!Number.isNaN(secs)) {
            (err as { retryAfterSec?: number }).retryAfterSec = secs;
          }
        }
      }
      throw err;
    }

    return parsed as T;
  }

  // -------------------------------------------------------------------------
  // Public API

  /** `GET /health` — liveness probe. */
  async health(): Promise<HealthResponse> {
    return this.request<HealthResponse>("GET", "/health");
  }

  /** `POST /proofs` — generate a proof and return the record. */
  async generateProof(req: GenerateProofRequest): Promise<ProofRecord> {
    if (!req.agent || !req.input || !req.output || !req.model) {
      throw new Error("generateProof: agent, input, output, model are required");
    }
    return this.request<ProofRecord>("POST", "/proofs", req);
  }

  /** `POST /proofs/batch` — generate up to 50 proofs in a single call. */
  async batchGenerate(req: BatchProofsRequest): Promise<{ proofs: ProofRecord[]; count: number }> {
    if (!Array.isArray(req.proofs) || req.proofs.length === 0) {
      throw new Error("batchGenerate: at least one proof entry is required");
    }
    if (req.proofs.length > 50) {
      throw new Error("batchGenerate: server rejects batches larger than 50");
    }
    return this.request<{ proofs: ProofRecord[]; count: number }>("POST", "/proofs/batch", req);
  }

  /** `POST /verify` — verify a proof by ID, optionally with the original triple. */
  async verifyProof(req: VerifyProofRequest): Promise<VerifyProofResponse> {
    if (!req.proof_id) {
      throw new Error("verifyProof: proof_id is required");
    }
    return this.request<VerifyProofResponse>("POST", "/verify", req);
  }

  /**
   * `POST /verify` convenience helper: same call as `verifyProof`, but named
   * to match the docs' notion of "batch verify" (i.e. verify each proof in
   * sequence). If you need atomic multi-proof verification, sequence the
   * awaits yourself and inspect each response.
   */
  async batchVerify(proofIds: string[]): Promise<VerifyProofResponse[]> {
    if (!Array.isArray(proofIds) || proofIds.length === 0) {
      throw new Error("batchVerify: proofIds must be a non-empty array");
    }
    const out: VerifyProofResponse[] = [];
    for (const proof_id of proofIds) {
      out.push(await this.verifyProof({ proof_id }));
    }
    return out;
  }

  /** `GET /proofs/{id}` — fetch a proof record. Throws `NotFoundError` if absent. */
  async getProof(proofId: string): Promise<ProofRecord> {
    if (!proofId) throw new Error("getProof: proofId is required");
    return this.request<ProofRecord>("GET", `/proofs/${encodeURIComponent(proofId)}`);
  }

  /** Convenience — derive `ProofStatus` from a fresh `getProof`. */
  async getProofStatus(proofId: string): Promise<ProofStatus> {
    return proofStatus(await this.getProof(proofId));
  }

  /** `POST /proofs/{id}/revoke` — revoke a proof (idempotent on the server side). */
  async revokeProof(proofId: string): Promise<ProofRecord> {
    if (!proofId) throw new Error("revokeProof: proofId is required");
    return this.request<ProofRecord>("POST", `/proofs/${encodeURIComponent(proofId)}/revoke`);
  }

  /** `GET /proofs` — paginated listing with optional filters. */
  async listProofs(query: ListProofsQuery = {}): Promise<ListProofsResponse> {
    const params = new URLSearchParams();
    if (query.agent) params.set("agent", query.agent);
    if (query.public_key) params.set("public_key", query.public_key);
    if (query.mode) params.set("mode", query.mode);
    if (typeof query.page === "number") params.set("page", String(query.page));
    if (typeof query.limit === "number") params.set("limit", String(query.limit));
    const suffix = params.toString();
    return this.request<ListProofsResponse>("GET", `/proofs${suffix ? `?${suffix}` : ""}`);
  }
}
