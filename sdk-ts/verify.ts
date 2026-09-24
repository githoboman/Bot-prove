/**
 * Client-side Merkle-proof verifier for CasperProver.
 *
 * The CasperProver engine builds a Merkle tree over four canonical leaves —
 * `input_hash`, `output_hash`, `model_hash`, `proof_hash` — using the same
 * BLAKE2b-256 hash algorithm the server uses. This module lets a browser or
 * Node client recompute the root from `(leaf, path, leafIndex)` and compare
 * it to the server's reported `merkle_root` without trusting the server.
 *
 * Runs on both Node (18+) and the browser: it uses `globalThis.crypto.subtle`
 * where available, and falls back to a small pure-TS BLAKE2b-256 implementation
 * for algorithms `subtle` does not expose (BLAKE2b is not in the WebCrypto
 * standard — we ship our own).
 */

import type { ProofRecord } from "./types.ts";

// ---------------------------------------------------------------------------
// Hex helpers

/** Decode a lower/upper-case hex string into a Uint8Array. */
export function hexToBytes(hex: string): Uint8Array {
  const clean = hex.startsWith("0x") ? hex.slice(2) : hex;
  if (clean.length % 2 !== 0) {
    throw new Error(`invalid hex length: ${clean.length}`);
  }
  const out = new Uint8Array(clean.length / 2);
  for (let i = 0; i < out.length; i++) {
    const byte = parseInt(clean.substr(i * 2, 2), 16);
    if (Number.isNaN(byte)) {
      throw new Error(`invalid hex byte at offset ${i * 2}`);
    }
    out[i] = byte;
  }
  return out;
}

/** Encode a Uint8Array as a lower-case hex string, no `0x` prefix. */
export function bytesToHex(bytes: Uint8Array): string {
  let out = "";
  for (let i = 0; i < bytes.length; i++) {
    out += bytes[i].toString(16).padStart(2, "0");
  }
  return out;
}

// ---------------------------------------------------------------------------
// BLAKE2b-256 — matches Go's `golang.org/x/crypto/blake2b` with size=32.
//
// Reference: RFC 7693 §3.2. This is a straightforward port using
// BigInt (u64) arithmetic — fast enough for a handful of hashes per verify,
// portable across every JS runtime without native bindings.

const BLAKE2B_IV: bigint[] = [
  0x6a09e667f3bcc908n, 0xbb67ae8584caa73bn, 0x3c6ef372fe94f82bn, 0xa54ff53a5f1d36f1n,
  0x510e527fade682d1n, 0x9b05688c2b3e6c1fn, 0x1f83d9abfb41bd6bn, 0x5be0cd19137e2179n,
];

const BLAKE2B_SIGMA: number[][] = [
  [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15],
  [14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3],
  [11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4],
  [7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8],
  [9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13],
  [2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9],
  [12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11],
  [13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10],
  [6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5],
  [10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0],
  [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15],
  [14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3],
];

const MASK64 = 0xffffffffffffffffn;

function rotr64(x: bigint, n: number): bigint {
  const nn = BigInt(n);
  return (((x >> nn) | (x << (64n - nn))) & MASK64);
}

function mix(v: bigint[], a: number, b: number, c: number, d: number, x: bigint, y: bigint): void {
  v[a] = (v[a] + v[b] + x) & MASK64;
  v[d] = rotr64(v[d] ^ v[a], 32);
  v[c] = (v[c] + v[d]) & MASK64;
  v[b] = rotr64(v[b] ^ v[c], 24);
  v[a] = (v[a] + v[b] + y) & MASK64;
  v[d] = rotr64(v[d] ^ v[a], 16);
  v[c] = (v[c] + v[d]) & MASK64;
  v[b] = rotr64(v[b] ^ v[c], 63);
}

function readU64LE(bytes: Uint8Array, offset: number): bigint {
  let out = 0n;
  for (let i = 0; i < 8; i++) {
    out |= BigInt(bytes[offset + i]) << BigInt(i * 8);
  }
  return out;
}

function writeU64LE(bytes: Uint8Array, offset: number, value: bigint): void {
  for (let i = 0; i < 8; i++) {
    bytes[offset + i] = Number((value >> BigInt(i * 8)) & 0xffn);
  }
}

function blake2bCompress(h: bigint[], block: Uint8Array, t: bigint, isFinal: boolean): void {
  const v = new Array<bigint>(16);
  for (let i = 0; i < 8; i++) v[i] = h[i];
  for (let i = 0; i < 8; i++) v[i + 8] = BLAKE2B_IV[i];
  v[12] ^= t & MASK64;
  v[13] ^= (t >> 64n) & MASK64;
  if (isFinal) v[14] ^= MASK64;

  const m = new Array<bigint>(16);
  for (let i = 0; i < 16; i++) m[i] = readU64LE(block, i * 8);

  for (let round = 0; round < 12; round++) {
    const s = BLAKE2B_SIGMA[round];
    mix(v, 0, 4, 8, 12, m[s[0]], m[s[1]]);
    mix(v, 1, 5, 9, 13, m[s[2]], m[s[3]]);
    mix(v, 2, 6, 10, 14, m[s[4]], m[s[5]]);
    mix(v, 3, 7, 11, 15, m[s[6]], m[s[7]]);
    mix(v, 0, 5, 10, 15, m[s[8]], m[s[9]]);
    mix(v, 1, 6, 11, 12, m[s[10]], m[s[11]]);
    mix(v, 2, 7, 8, 13, m[s[12]], m[s[13]]);
    mix(v, 3, 4, 9, 14, m[s[14]], m[s[15]]);
  }
  for (let i = 0; i < 8; i++) h[i] ^= v[i] ^ v[i + 8];
}

/**
 * BLAKE2b-256 hash of `input`. Returns a 32-byte Uint8Array.
 * Deterministic; matches Go's `blake2b.Sum256`.
 */
export function blake2b256(input: Uint8Array): Uint8Array {
  const h = BLAKE2B_IV.slice();
  // Parameter block: digest_length=32, key_length=0, fanout=1, depth=1
  h[0] ^= 0x0000000001010020n;

  const block = new Uint8Array(128);
  let t = 0n;
  let offset = 0;

  while (input.length - offset > 128) {
    block.set(input.subarray(offset, offset + 128));
    t += 128n;
    blake2bCompress(h, block, t, false);
    offset += 128;
  }

  const remaining = input.length - offset;
  block.fill(0);
  block.set(input.subarray(offset));
  t += BigInt(remaining);
  blake2bCompress(h, block, t, true);

  const out = new Uint8Array(32);
  for (let i = 0; i < 4; i++) writeU64LE(out, i * 8, h[i]);
  return out;
}

/** Hex-in, hex-out BLAKE2b-256 convenience wrapper. */
export function blake2b256Hex(inputHex: string): string {
  return bytesToHex(blake2b256(hexToBytes(inputHex)));
}

/** Hash of a raw string (UTF-8), matching `hasher.HexHash([]byte(s))`. */
export function blake2b256OfString(s: string): string {
  return bytesToHex(blake2b256(new TextEncoder().encode(s)));
}

// ---------------------------------------------------------------------------
// Merkle-proof verification

/**
 * Recompute the Merkle root from a leaf hash, its sibling path, and the
 * leaf's index. `combine(leaf, sibling)` is BLAKE2b-256(leaf || sibling)
 * when the leaf is on the left (even index at that level), and
 * BLAKE2b-256(sibling || leaf) when it is on the right.
 *
 * All hex arguments are lower or upper case, without `0x`.
 */
export function computeMerkleRoot(leafHashHex: string, pathHex: string[], leafIndex: number): string {
  let current = hexToBytes(leafHashHex);
  let idx = leafIndex;
  for (const siblingHex of pathHex) {
    const sibling = hexToBytes(siblingHex);
    const combined = new Uint8Array(64);
    if (idx % 2 === 0) {
      combined.set(current, 0);
      combined.set(sibling, 32);
    } else {
      combined.set(sibling, 0);
      combined.set(current, 32);
    }
    current = blake2b256(combined);
    idx = Math.floor(idx / 2);
  }
  return bytesToHex(current);
}

/**
 * Verify that `proof.merkle_path` + `proof.leaf_index` reconstruct
 * `proof.merkle_root` when starting from `leafHashHex`.
 *
 * `leafHashHex` is the specific leaf value the caller wants to prove is
 * included — typically `proof.proof_hash` (the canonical envelope hash),
 * but the engine builds the tree over all four hashes, so `input_hash`,
 * `output_hash`, and `model_hash` are equally valid leaves.
 */
export function verifyMerkleInclusion(proof: ProofRecord, leafHashHex: string): boolean {
  const recomputed = computeMerkleRoot(leafHashHex, proof.merkle_path, proof.leaf_index);
  return recomputed.toLowerCase() === proof.merkle_root.toLowerCase();
}

/**
 * Full offline verification of a proof against a caller-supplied
 * `(input, output, model)` triple: recomputes the three hashes, checks
 * they match the ones the server recorded, then verifies each leaf's
 * Merkle inclusion against `merkle_root`.
 *
 * Returns a structured report; does not throw.
 */
export interface OfflineVerifyReport {
  hashesMatch: {
    input: boolean;
    output: boolean;
    model: boolean;
  };
  merkleMatch: {
    input: boolean;
    output: boolean;
    model: boolean;
    proof: boolean;
  };
  overallValid: boolean;
}

export function verifyOffline(
  proof: ProofRecord,
  input: string,
  output: string,
  model: string,
): OfflineVerifyReport {
  const ih = blake2b256OfString(input);
  const oh = blake2b256OfString(output);
  const mh = blake2b256OfString(model);
  const hashesMatch = {
    input: ih === proof.input_hash.toLowerCase(),
    output: oh === proof.output_hash.toLowerCase(),
    model: mh === proof.model_hash.toLowerCase(),
  };
  const merkleMatch = {
    input: verifyMerkleInclusion(proof, proof.input_hash),
    output: verifyMerkleInclusion(proof, proof.output_hash),
    model: verifyMerkleInclusion(proof, proof.model_hash),
    proof: verifyMerkleInclusion(proof, proof.proof_hash),
  };
  const overallValid =
    hashesMatch.input && hashesMatch.output && hashesMatch.model &&
    merkleMatch.input && merkleMatch.output && merkleMatch.model && merkleMatch.proof;
  return { hashesMatch, merkleMatch, overallValid };
}
