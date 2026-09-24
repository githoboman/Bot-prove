/**
 * Type-shape smoke tests for the CasperProver SDK.
 *
 * These are structural checks — the compile-time contract is enforced by
 * `tsc --noEmit`; at runtime we just make sure the exports exist and the
 * public function set is stable across releases.
 */

import { describe, it } from "node:test";
import assert from "node:assert/strict";

import * as sdk from "../index.ts";

describe("SDK public surface", () => {
  it("exports the client class + status helper", () => {
    assert.equal(typeof sdk.CasperProverClient, "function");
    assert.equal(typeof sdk.proofStatus, "function");
  });

  it("exports every declared error class", () => {
    for (const name of [
      "CasperProverError",
      "APIError",
      "BadRequestError",
      "UnauthorizedError",
      "ForbiddenError",
      "NotFoundError",
      "RateLimitError",
      "ServerError",
      "NetworkError",
    ]) {
      assert.equal(typeof (sdk as unknown as Record<string, unknown>)[name], "function", `missing export: ${name}`);
    }
  });

  it("exports the offline verifier surface", () => {
    for (const name of [
      "blake2b256",
      "blake2b256Hex",
      "blake2b256OfString",
      "computeMerkleRoot",
      "verifyMerkleInclusion",
      "verifyOffline",
      "hexToBytes",
      "bytesToHex",
      "errorForStatus",
    ]) {
      assert.equal(typeof (sdk as unknown as Record<string, unknown>)[name], "function", `missing: ${name}`);
    }
  });

  it("proofStatus derives 'valid'/'revoked'/'invalid' correctly", () => {
    const base = {
      id: "id",
      agent: "a",
      proof_hash: "",
      input_hash: "",
      output_hash: "",
      model_hash: "",
      merkle_root: "",
      merkle_path: [],
      leaf_index: 0,
      timestamp: 0,
      use_case: "",
      generation_ms: 0,
    };
    assert.equal(sdk.proofStatus({ ...base, valid: true, revoked: false }), "valid");
    assert.equal(sdk.proofStatus({ ...base, valid: false, revoked: true }), "revoked");
    assert.equal(sdk.proofStatus({ ...base, valid: false, revoked: false }), "invalid");
    // revoked takes precedence over valid=true (safety-first)
    assert.equal(sdk.proofStatus({ ...base, valid: true, revoked: true }), "revoked");
  });
});
