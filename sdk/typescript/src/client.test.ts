import { describe, it } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import type { AddressInfo } from "node:net";

import { Client, ApiError } from "./client.ts";
import { hashField, verifyReceiptBytes, ReceiptValidationError } from "./receipt.ts";

type Recorded = {
  method: string;
  path: string;
  headers: Record<string, string>;
  body: string;
};

async function withServer(
  reply: string,
  fn: (baseUrl: string, rec: Recorded) => Promise<void>,
): Promise<void> {
  const rec: Recorded = { method: "", path: "", headers: {}, body: "" };
  const server = http.createServer((req, res) => {
    let chunks: Buffer[] = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      rec.method = req.method ?? "";
      rec.path = req.url ?? "";
      rec.headers = Object.fromEntries(
        Object.entries(req.headers).map(([k, v]) => [
          k.toLowerCase(),
          Array.isArray(v) ? v.join(",") : String(v ?? ""),
        ]),
      );
      rec.body = Buffer.concat(chunks).toString("utf8");
      res.setHeader("Content-Type", "application/json");
      res.statusCode = 200;
      res.end(reply);
    });
  });
  await new Promise<void>((r) => server.listen(0, "127.0.0.1", () => r()));
  const { port } = server.address() as AddressInfo;
  try {
    await fn(`http://127.0.0.1:${port}`, rec);
  } finally {
    await new Promise<void>((r) => server.close(() => r()));
  }
}

describe("Client.prove", () => {
  it("posts to /v1/proofs", async () => {
    await withServer('{"id":"pf-1","proof_hash":"deadbeef"}', async (url, rec) => {
      const c = new Client({ baseUrl: url });
      const got = await c.prove({ agent: "a", model: "m", input: "in", output: "out" });
      assert.equal(got.id, "pf-1");
      assert.equal(rec.path, "/v1/proofs");
      assert.equal(rec.method, "POST");
    });
  });

  it("sends idempotency key", async () => {
    await withServer('{"id":"pf-1"}', async (url, rec) => {
      const c = new Client({ baseUrl: url });
      await c.prove({ agent: "a" }, { idempotencyKey: "key-42" });
      assert.equal(rec.headers["x-idempotency-key"], "key-42");
    });
  });
});

describe("Client.verify", () => {
  it("posts proof_id", async () => {
    await withServer('{"valid":true,"proof_id":"pf-9"}', async (url, rec) => {
      const c = new Client({ baseUrl: url });
      const got = await c.verify("pf-9");
      assert.equal(got.valid, true);
      assert.equal(got.proof_id, "pf-9");
      assert.equal(rec.path, "/v1/verify");
      assert.match(rec.body, /"proof_id":"pf-9"/);
    });
  });
});

describe("Client.batch", () => {
  it("sends all items with mode", async () => {
    await withServer('{"verified":["a","b"],"total":2}', async (url, rec) => {
      const c = new Client({ baseUrl: url });
      const got = await c.batch([{ proof_id: "a" }, { proof_id: "b" }], "strict");
      assert.equal(got.total, 2);
      assert.equal(got.verified?.length, 2);
      assert.equal(rec.path, "/v1/batch/verify-zk");
      const body = JSON.parse(rec.body);
      assert.equal(body.mode, "strict");
    });
  });
});

describe("Client.anchor", () => {
  it("hits /v1/proofs/{id}/anchor with idempotency", async () => {
    await withServer(
      '{"proof_id":"pf-x","tx_hash":"aa11","strict_mode":true}',
      async (url, rec) => {
        const c = new Client({ baseUrl: url });
        const got = await c.anchor("pf-x", { idempotencyKey: "anchor-key" });
        assert.equal(got.tx_hash, "aa11");
        assert.equal(got.strict_mode, true);
        assert.equal(rec.path, "/v1/proofs/pf-x/anchor");
        assert.equal(rec.headers["x-idempotency-key"], "anchor-key");
      },
    );
  });
});

describe("Client legacy version", () => {
  it("apiVersion empty keeps /proofs", async () => {
    await withServer('{"id":"pf"}', async (url, rec) => {
      const c = new Client({ baseUrl: url, apiVersion: "" });
      await c.prove({ agent: "a" });
      assert.equal(rec.path, "/proofs");
    });
  });
});

describe("Client error handling", () => {
  it("throws ApiError on non-2xx", async () => {
    // Custom server that always 500s.
    const server = http.createServer((_, res) => {
      res.statusCode = 500;
      res.end("boom");
    });
    await new Promise<void>((r) => server.listen(0, "127.0.0.1", () => r()));
    try {
      const { port } = server.address() as AddressInfo;
      const c = new Client({ baseUrl: `http://127.0.0.1:${port}` });
      await assert.rejects(() => c.prove({ agent: "a" }), (e: unknown) => {
        assert.ok(e instanceof ApiError);
        assert.equal((e as ApiError).status, 500);
        return true;
      });
    } finally {
      await new Promise<void>((r) => server.close(() => r()));
    }
  });
});

describe("receipt validator", () => {
  it("matches Go SHA-256 for 'hello'", () => {
    assert.equal(
      hashField("hello"),
      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
    );
  });

  it("accepts a well-formed receipt", () => {
    const payload = JSON.stringify({
      id: "pf-1",
      input: "hello",
      output: "world",
      model: "gpt-toy-v1",
      input_hash: hashField("hello"),
      output_hash: hashField("world"),
      model_hash: hashField("gpt-toy-v1"),
    });
    const r = verifyReceiptBytes(payload);
    assert.equal(r.id, "pf-1");
  });

  it("rejects a tampered receipt", () => {
    const payload = JSON.stringify({
      id: "pf-1",
      input: "hello",
      input_hash: hashField("goodbye"),
    });
    assert.throws(
      () => verifyReceiptBytes(payload),
      (e: unknown) => {
        assert.ok(e instanceof ReceiptValidationError);
        assert.equal((e as ReceiptValidationError).field, "input_hash");
        return true;
      },
    );
  });
});
