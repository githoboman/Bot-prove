/**
 * Unit tests for the CasperProver HTTP client.
 *
 * We inject a fake `fetch` via `CasperProverClient({ fetch })` and record
 * the request the client issues. No live server, no MSW — this keeps the
 * SDK zero-dep and the tests reproducible in every JS runtime.
 *
 * Run: `node --test --experimental-strip-types tests/client.test.ts`
 */

import { describe, it } from "node:test";
import assert from "node:assert/strict";

import { CasperProverClient } from "../client.ts";
import {
  BadRequestError,
  ForbiddenError,
  NotFoundError,
  RateLimitError,
  ServerError,
  UnauthorizedError,
} from "../errors.ts";
import type { ProofRecord } from "../types.ts";

interface CapturedRequest {
  url: string;
  method: string;
  headers: Record<string, string>;
  body?: string;
}

/**
 * Build a fake `fetch` that returns queued responses and records every
 * incoming request. Responses can be JSON, plain-text, or (rare) empty.
 */
function makeFakeFetch(
  responses: Array<{ status: number; body?: unknown; headers?: Record<string, string> }>,
): { fetchImpl: typeof fetch; requests: CapturedRequest[] } {
  const requests: CapturedRequest[] = [];
  const queue = [...responses];
  const fetchImpl = (async (input: unknown, init?: RequestInit) => {
    const url = typeof input === "string" ? input : String(input);
    const headers: Record<string, string> = {};
    if (init?.headers) {
      const h = init.headers as Record<string, string>;
      for (const k of Object.keys(h)) headers[k] = h[k];
    }
    requests.push({
      url,
      method: init?.method ?? "GET",
      headers,
      body: typeof init?.body === "string" ? init.body : undefined,
    });
    const next = queue.shift();
    if (!next) throw new Error(`fake fetch: no queued response for ${init?.method ?? "GET"} ${url}`);

    let text: string;
    let contentType = "application/json";
    if (typeof next.body === "string") {
      text = next.body;
      contentType = "text/plain";
    } else if (next.body === undefined) {
      text = "";
    } else {
      text = JSON.stringify(next.body);
    }
    const respHeaders = new Headers({ "content-type": contentType, ...(next.headers ?? {}) });
    return new Response(text, { status: next.status, headers: respHeaders });
  }) as unknown as typeof fetch;
  return { fetchImpl, requests };
}

const fakeProof: ProofRecord = {
  id: "proof-1",
  agent: "agent-a",
  proof_hash: "aa".repeat(32),
  input_hash: "bb".repeat(32),
  output_hash: "cc".repeat(32),
  model_hash: "dd".repeat(32),
  merkle_root: "ee".repeat(32),
  merkle_path: [],
  leaf_index: 0,
  timestamp: 1_700_000_000,
  valid: true,
  revoked: false,
  use_case: "unit-test",
  generation_ms: 12,
  mode: "local",
};

describe("CasperProverClient — routing and headers", () => {
  it("hits /health with GET", async () => {
    const { fetchImpl, requests } = makeFakeFetch([{ status: 200, body: { status: "ok" } }]);
    const c = new CasperProverClient({ baseUrl: "http://cp:8080/", fetch: fetchImpl });
    const res = await c.health();
    assert.equal(res.status, "ok");
    assert.equal(requests[0].url, "http://cp:8080/health");
    assert.equal(requests[0].method, "GET");
    assert.equal(requests[0].headers["Accept"], "application/json");
    assert.equal(requests[0].headers["X-API-Key"], undefined);
  });

  it("sends X-API-Key and X-Public-Key when configured", async () => {
    const { fetchImpl, requests } = makeFakeFetch([{ status: 200, body: { status: "ok" } }]);
    const c = new CasperProverClient({
      baseUrl: "http://cp:8080",
      apiKey: "test-key",
      publicKey: "deadbeef",
      fetch: fetchImpl,
    });
    await c.health();
    assert.equal(requests[0].headers["X-API-Key"], "test-key");
    assert.equal(requests[0].headers["X-Public-Key"], "deadbeef");
  });

  it("POSTs generateProof with JSON body", async () => {
    const { fetchImpl, requests } = makeFakeFetch([{ status: 200, body: fakeProof }]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    const p = await c.generateProof({
      agent: "a",
      input: "i",
      output: "o",
      model: "m",
      use_case: "u",
      mode: "local",
    });
    assert.equal(p.id, "proof-1");
    const req = requests[0];
    assert.equal(req.method, "POST");
    assert.match(req.url, /\/proofs$/);
    assert.equal(req.headers["Content-Type"], "application/json");
    const body = JSON.parse(req.body ?? "{}");
    assert.deepEqual(body, { agent: "a", input: "i", output: "o", model: "m", use_case: "u", mode: "local" });
  });

  it("rejects generateProof without required fields client-side", async () => {
    const c = new CasperProverClient();
    await assert.rejects(
      () => c.generateProof({ agent: "", input: "i", output: "o", model: "m" }),
      /agent, input, output, model are required/,
    );
  });

  it("URL-encodes proof IDs on GET /proofs/{id}", async () => {
    const { fetchImpl, requests } = makeFakeFetch([{ status: 200, body: fakeProof }]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    await c.getProof("weird id/with slash");
    assert.match(requests[0].url, /\/proofs\/weird%20id%2Fwith%20slash$/);
  });

  it("derives ProofStatus via getProofStatus", async () => {
    const { fetchImpl } = makeFakeFetch([
      { status: 200, body: { ...fakeProof, valid: true, revoked: false } },
      { status: 200, body: { ...fakeProof, valid: false, revoked: true } },
      { status: 200, body: { ...fakeProof, valid: false, revoked: false } },
    ]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    assert.equal(await c.getProofStatus("a"), "valid");
    assert.equal(await c.getProofStatus("b"), "revoked");
    assert.equal(await c.getProofStatus("c"), "invalid");
  });

  it("builds listProofs query string from filters", async () => {
    const { fetchImpl, requests } = makeFakeFetch([
      { status: 200, body: { proofs: [], total: 0, page: 1, limit: 20 } },
    ]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    await c.listProofs({ agent: "a-1", mode: "anchored", page: 2, limit: 10 });
    assert.match(requests[0].url, /agent=a-1/);
    assert.match(requests[0].url, /mode=anchored/);
    assert.match(requests[0].url, /page=2/);
    assert.match(requests[0].url, /limit=10/);
  });

  it("batchVerify sequences verifyProof calls", async () => {
    const { fetchImpl, requests } = makeFakeFetch([
      { status: 200, body: { proof_id: "p1", valid: true, revoked: false } },
      { status: 200, body: { proof_id: "p2", valid: true, revoked: false } },
    ]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    const out = await c.batchVerify(["p1", "p2"]);
    assert.equal(out.length, 2);
    assert.equal(requests.length, 2);
    assert.match(requests[0].url, /\/verify$/);
  });

  it("batchGenerate enforces server-side batch size limit client-side", async () => {
    const c = new CasperProverClient();
    const many = Array.from({ length: 51 }, () => ({ agent: "a", input: "i", output: "o", model: "m" }));
    await assert.rejects(() => c.batchGenerate({ proofs: many }), /larger than 50/);
    await assert.rejects(() => c.batchGenerate({ proofs: [] }), /at least one proof entry/);
  });
});

describe("CasperProverClient — error mapping", () => {
  it("maps 400 to BadRequestError with server message", async () => {
    const { fetchImpl } = makeFakeFetch([{ status: 400, body: { error: "bad input" } }]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    await assert.rejects(() => c.getProof("x"), (e) => {
      assert.ok(e instanceof BadRequestError, "expected BadRequestError");
      assert.equal((e as BadRequestError).message, "bad input");
      assert.equal((e as BadRequestError).status, 400);
      return true;
    });
  });

  it("maps 401/403/404 to their subclasses", async () => {
    const { fetchImpl } = makeFakeFetch([
      { status: 401, body: { error: "no key" } },
      { status: 403, body: { error: "wrong scope" } },
      { status: 404, body: { error: "no proof" } },
    ]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    await assert.rejects(() => c.getProof("a"), UnauthorizedError);
    await assert.rejects(() => c.getProof("b"), ForbiddenError);
    await assert.rejects(() => c.getProof("c"), NotFoundError);
  });

  it("maps 429 to RateLimitError and picks up Retry-After", async () => {
    const { fetchImpl } = makeFakeFetch([
      { status: 429, body: { error: "slow down" }, headers: { "Retry-After": "12" } },
    ]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    await assert.rejects(() => c.getProof("a"), (e) => {
      assert.ok(e instanceof RateLimitError, "expected RateLimitError");
      assert.equal((e as RateLimitError).retryAfterSec, 12);
      return true;
    });
  });

  it("maps 5xx to ServerError", async () => {
    const { fetchImpl } = makeFakeFetch([{ status: 503, body: { error: "backend down" } }]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    await assert.rejects(() => c.getProof("a"), ServerError);
  });

  it("falls back to plain-text error body when server returned text/plain", async () => {
    const { fetchImpl } = makeFakeFetch([{ status: 400, body: "invalid JSON body" }]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    await assert.rejects(() => c.getProof("a"), (e) => {
      assert.ok(e instanceof BadRequestError);
      assert.equal((e as BadRequestError).message, "invalid JSON body");
      return true;
    });
  });
});

describe("CasperProverClient — constructor", () => {
  it("strips trailing slashes from baseUrl", async () => {
    const { fetchImpl, requests } = makeFakeFetch([{ status: 200, body: { status: "ok" } }]);
    const c = new CasperProverClient({ baseUrl: "http://cp:8080///", fetch: fetchImpl });
    await c.health();
    assert.equal(requests[0].url, "http://cp:8080/health");
  });

  it("uses default baseUrl when omitted", async () => {
    const { fetchImpl, requests } = makeFakeFetch([{ status: 200, body: { status: "ok" } }]);
    const c = new CasperProverClient({ fetch: fetchImpl });
    await c.health();
    assert.equal(requests[0].url, "http://localhost:8080/health");
  });

  it("throws when no fetch is available and none is injected", () => {
    // Deno / very old Node scenario.
    const originalFetch = globalThis.fetch;
    try {
      // @ts-expect-error — deliberately clearing.
      globalThis.fetch = undefined;
      assert.throws(() => new CasperProverClient(), /no fetch\(\) available/);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});
