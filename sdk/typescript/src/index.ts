/**
 * CasperProver TypeScript SDK.
 *
 * @example
 * ```ts
 * import { Client, ProveRequest, verifyReceipt } from "@casperprover/sdk";
 *
 * const c = new Client({ baseUrl: "http://localhost:9090", apiKey: "pk_..." });
 * const proof = await c.prove({ agent: "a", model: "m", input: "hi", output: "ok" });
 * const check = await c.verify(proof.id);
 * ```
 *
 * Feature parity with the Go SDK (`sdk/primitives.go`) and the Python SDK
 * (`sdk/python/casperprover`). All hash primitives are bit-identical.
 */

export {
  Client,
  ApiError,
  DEFAULT_BASE_URL,
  DEFAULT_TIMEOUT_MS,
} from "./client.js";
export type {
  ClientOptions,
  RequestOptions,
  ProveRequest,
  ProveResponse,
  VerifyResponse,
  BatchItem,
  BatchResponse,
  AnchorResponse,
} from "./client.js";
export {
  hashField,
  verifyReceipt,
  verifyReceiptBytes,
  ReceiptValidationError,
} from "./receipt.js";
export type { ProofReceipt } from "./receipt.js";
