# CasperProver TypeScript SDK

> **Status:** `v0.1.0`. Full-feature client with `prove`, `verify`, `batch`,
> `anchor`, and a shared receipt validator. Feature parity with the Go and
> Python SDKs.

## Install (once published)

```sh
npm install @casperprover/sdk
```

Until the first release, add it as a path dependency or import directly
from the checkout.

## Quickstart

```ts
import { Client, verifyReceiptBytes } from "@casperprover/sdk";

const c = new Client({
  baseUrl: "https://casperprover-api-ylsh.onrender.com",
  apiKey: "pk_...",
});

const proof = await c.prove(
  { agent: "a", model: "gpt-toy-v1", input: "hello", output: "42" },
  { idempotencyKey: "run-1" },
);
console.log(proof.id, proof.vk_hash);

const check = await c.verify(proof.id);
console.log(check.valid);
```

Every write primitive accepts `{ idempotencyKey }` — safe retries against
the server-side dedup cache (24h TTL).

Legacy unversioned routes: `new Client({ apiVersion: "" })`.

## Receipt validator

`verifyReceiptBytes(payload)` and `verifyReceipt(obj)` re-derive
`input_hash`, `output_hash`, and `model_hash` locally (SHA-256 of the UTF-8
plaintext) and throw `ReceiptValidationError` on any mismatch.

## Test

```sh
cd sdk/typescript
node --test --experimental-strip-types src/client.test.ts
```

## Build (once TypeScript is installed)

```sh
npm install --save-dev typescript@5
npx tsc -p tsconfig.json
```
