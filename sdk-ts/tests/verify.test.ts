/**
 * Unit tests for the offline Merkle-proof verifier.
 *
 * Uses Node's built-in `node:test` runner (available since Node 18);
 * the file is also compatible with `vitest` because the assertion API
 * is `node:assert/strict`, which is a superset of `expect`-style checks
 * for this suite's needs.
 *
 * Run: `node --test --experimental-strip-types tests/verify.test.ts`
 * or:  `npx vitest run tests/verify.test.ts`
 */

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";

import {
  blake2b256,
  blake2b256OfString,
  bytesToHex,
  computeMerkleRoot,
  hexToBytes,
  verifyMerkleInclusion,
  verifyOffline,
} from "../verify.ts";
import type { ProofRecord } from "../types.ts";

// ---------------------------------------------------------------------------
// BLAKE2b-256 KATs — cross-check against Node's built-in implementation
// (Node 18+ exposes BLAKE2b via `createHash('blake2b512')`; we truncate).

function nodeBlake2b256(input: Uint8Array): string {
  // Node ships blake2b512 (64-byte output) via OpenSSL. Our implementation
  // targets the standard 32-byte variant with an unkeyed parameter block —
  // NOT the truncation of blake2b512, which has a different IV[0] mix.
  // So we cross-check against a small hard-coded fixture set (KATs).
  void input;
  throw new Error("use hard-coded KATs, not Node's blake2b512");
}
void nodeBlake2b256;

describe("hex helpers", () => {
  it("round-trips arbitrary byte arrays", () => {
    const b = new Uint8Array([0, 1, 15, 16, 254, 255]);
    assert.equal(bytesToHex(b), "00010f10feff");
    assert.deepEqual(hexToBytes("00010f10feff"), b);
  });

  it("accepts 0x prefix", () => {
    assert.deepEqual(hexToBytes("0xab01"), new Uint8Array([0xab, 0x01]));
  });

  it("rejects odd-length hex", () => {
    assert.throws(() => hexToBytes("abc"), /invalid hex length/);
  });

  it("rejects non-hex characters", () => {
    assert.throws(() => hexToBytes("zz"), /invalid hex byte/);
  });
});

describe("blake2b256 known-answer tests", () => {
  // Reference: values obtained from Go's `blake2b.Sum256([]byte(s))`.
  const KATs: Array<[string, string]> = [
    ["", "0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8"],
    ["abc", "bddd813c634239723171ef3fee98579b94964e3bb1cb3e427262c8c068d52319"],
    [
      "The quick brown fox jumps over the lazy dog",
      "01718cec35cd3d796dd00020e0bfecb473ad23457d063b75eff29c0ffa2e58a9",
    ],
  ];
  // Note: the "abc" and "The quick..." digests below are the canonical Go
  // blake2b.Sum256 outputs. If the local implementation is correct they
  // will match exactly. The "" (empty) digest is a well-known KAT for
  // BLAKE2b-256 (see RFC 7693 test vectors + go/x/crypto tests).

  for (const [input, expected] of KATs) {
    it(`hashes ${JSON.stringify(input.slice(0, 20))}`, () => {
      const got = blake2b256OfString(input);
      assert.equal(got, expected, `input="${input}"`);
    });
  }

  it("is deterministic across two calls", () => {
    const a = bytesToHex(blake2b256(new TextEncoder().encode("determinism")));
    const b = bytesToHex(blake2b256(new TextEncoder().encode("determinism")));
    assert.equal(a, b);
  });
});

describe("computeMerkleRoot", () => {
  // A hand-built 4-leaf tree with all leaves = zero-byte hashes for
  // structural checks. Real trees use blake2b digests as leaves.
  const zero = "0".repeat(64);

  it("returns the leaf itself when the path is empty", () => {
    const root = computeMerkleRoot(zero, [], 0);
    assert.equal(root, zero);
  });

  it("hashes leaf||sibling for even index", () => {
    // Leaf at index 0 (left), one sibling.
    const sibling = "11".repeat(32);
    const expected = bytesToHex(
      blake2b256(new Uint8Array([...hexToBytes(zero), ...hexToBytes(sibling)])),
    );
    assert.equal(computeMerkleRoot(zero, [sibling], 0), expected);
  });

  it("hashes sibling||leaf for odd index", () => {
    const sibling = "22".repeat(32);
    const expected = bytesToHex(
      blake2b256(new Uint8Array([...hexToBytes(sibling), ...hexToBytes(zero)])),
    );
    assert.equal(computeMerkleRoot(zero, [sibling], 1), expected);
  });
});

describe("verifyMerkleInclusion", () => {
  // Build a synthetic ProofRecord with a self-consistent Merkle path.
  function buildFixture(): { proof: ProofRecord; leafHash: string } {
    // 4-leaf tree:  h0 h1 h2 h3
    //               \  /  \  /
    //                p01   p23
    //                 \    /
    //                  root
    const leaves = ["aa", "bb", "cc", "dd"].map((s) => blake2b256OfString(s));
    const combine = (l: string, r: string) =>
      bytesToHex(blake2b256(new Uint8Array([...hexToBytes(l), ...hexToBytes(r)])));
    const p01 = combine(leaves[0], leaves[1]);
    const p23 = combine(leaves[2], leaves[3]);
    const root = combine(p01, p23);

    // Prove inclusion of leaf 2 (h2): path = [h3, p01], index = 2.
    const proof: ProofRecord = {
      id: "test-proof",
      agent: "test",
      proof_hash: leaves[2],
      input_hash: leaves[0],
      output_hash: leaves[1],
      model_hash: leaves[3],
      merkle_root: root,
      merkle_path: [leaves[3], p01],
      leaf_index: 2,
      timestamp: 0,
      valid: true,
      revoked: false,
      use_case: "",
      generation_ms: 0,
    };
    return { proof, leafHash: leaves[2] };
  }

  it("accepts a valid inclusion proof", () => {
    const { proof, leafHash } = buildFixture();
    assert.equal(verifyMerkleInclusion(proof, leafHash), true);
  });

  it("rejects a tampered leaf", () => {
    const { proof } = buildFixture();
    const tampered = "ff".repeat(32);
    assert.equal(verifyMerkleInclusion(proof, tampered), false);
  });

  it("rejects a tampered root", () => {
    const { proof, leafHash } = buildFixture();
    const bad = { ...proof, merkle_root: "ee".repeat(32) };
    assert.equal(verifyMerkleInclusion(bad, leafHash), false);
  });

  it("rejects a proof where leaf_index does not match the path direction", () => {
    const { proof, leafHash } = buildFixture();
    const bad = { ...proof, leaf_index: 3 };
    assert.equal(verifyMerkleInclusion(bad, leafHash), false);
  });

  it("rejects a proof with a truncated path", () => {
    const { proof, leafHash } = buildFixture();
    const bad = { ...proof, merkle_path: proof.merkle_path.slice(0, 1) };
    assert.equal(verifyMerkleInclusion(bad, leafHash), false);
  });
});

describe("verifyOffline", () => {
  it("reports overallValid=false when the caller-provided input does not match", () => {
    // Build a proof over "hello"/"world"/"m" then verify against a wrong input.
    const ih = blake2b256OfString("hello");
    const oh = blake2b256OfString("world");
    const mh = blake2b256OfString("m");
    const ph = blake2b256OfString("proof-envelope");

    // 4-leaf tree with our four canonical hashes.
    const combine = (l: string, r: string) =>
      bytesToHex(blake2b256(new Uint8Array([...hexToBytes(l), ...hexToBytes(r)])));
    const p01 = combine(ih, oh);
    const p23 = combine(mh, ph);
    const root = combine(p01, p23);

    // Path for the "input" leaf (index 0): [oh, p23].
    // But verifyOffline verifies *all four* leaves — for the test we build
    // a proof that carries a valid path only for `proof_hash` and treat the
    // other paths as best-effort. The report will surface partial mismatches.
    const proof: ProofRecord = {
      id: "ph-only",
      agent: "test",
      proof_hash: ph,
      input_hash: ih,
      output_hash: oh,
      model_hash: mh,
      merkle_root: root,
      merkle_path: [mh, p01], // valid for ph at index 3
      leaf_index: 3,
      timestamp: 0,
      valid: true,
      revoked: false,
      use_case: "",
      generation_ms: 0,
    };
    const report = verifyOffline(proof, "hello-different", "world", "m");
    assert.equal(report.hashesMatch.input, false, "wrong input should be flagged");
    assert.equal(report.hashesMatch.output, true);
    assert.equal(report.hashesMatch.model, true);
    assert.equal(report.overallValid, false, "wrong input must produce overallValid=false");
    // At least the proof-hash leaf's Merkle inclusion should hold.
    assert.equal(report.merkleMatch.proof, true, "proof leaf inclusion should still verify");
  });
});
