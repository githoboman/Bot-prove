/**
 * Client-side proof-receipt validator.
 *
 * Bit-for-bit compatible with the Go and Python implementations. Uses the
 * global WebCrypto API (Node >= 20, all modern browsers).
 */

import { createHash } from "node:crypto";

export interface ProofReceipt {
  id: string;
  agent?: string;
  model?: string;
  input?: string;
  output?: string;
  use_case?: string;
  proof_hash?: string;
  vk_hash?: string;
  input_hash?: string;
  output_hash?: string;
  model_hash?: string;
  verdict?: string;
  created_at?: string;
  /** Full parsed JSON payload. */
  raw?: Record<string, unknown>;
}

export class ReceiptValidationError extends Error {
  public readonly field: string;
  public readonly expected: string;
  public readonly actual: string;
  constructor(field: string, expected: string, actual: string) {
    super(`receipt field "${field}" mismatch: expected ${expected}, got ${actual}`);
    this.name = "ReceiptValidationError";
    this.field = field;
    this.expected = expected;
    this.actual = actual;
  }
}

/**
 * Return the canonical lowercase-hex SHA-256 of a UTF-8 string. Matches the
 * Go implementation's `HashField` and the Python `hash_field` byte for byte.
 */
export function hashField(value: string): string {
  return createHash("sha256").update(value, "utf8").digest("hex");
}

function normalize(h: string): string {
  const lower = h.toLowerCase();
  return lower.startsWith("0x") ? lower.slice(2) : lower;
}

function hexEqual(a: string, b: string): boolean {
  const na = normalize(a);
  const nb = normalize(b);
  if (na.length !== nb.length) return false;
  for (const s of [na, nb]) {
    for (let i = 0; i < s.length; i++) {
      const c = s.charCodeAt(i);
      const isDigit = c >= 48 && c <= 57;
      const isHex = c >= 97 && c <= 102;
      if (!isDigit && !isHex) return false;
    }
  }
  return na === nb;
}

export function verifyReceipt(data: Record<string, unknown>): ProofReceipt {
  const receipt: ProofReceipt = {
    id: String(data.id ?? ""),
    agent: data.agent ? String(data.agent) : undefined,
    model: data.model ? String(data.model) : undefined,
    input: data.input !== undefined ? String(data.input) : undefined,
    output: data.output !== undefined ? String(data.output) : undefined,
    use_case: data.use_case ? String(data.use_case) : undefined,
    proof_hash: data.proof_hash ? String(data.proof_hash) : undefined,
    vk_hash: data.vk_hash ? String(data.vk_hash) : undefined,
    input_hash: data.input_hash ? String(data.input_hash) : undefined,
    output_hash: data.output_hash ? String(data.output_hash) : undefined,
    model_hash: data.model_hash ? String(data.model_hash) : undefined,
    verdict: data.verdict ? String(data.verdict) : undefined,
    created_at: data.created_at ? String(data.created_at) : undefined,
    raw: data,
  };
  if (!receipt.id) throw new Error("receipt missing id");

  const checks: [string, string | undefined, string | undefined][] = [
    ["input_hash", receipt.input, receipt.input_hash],
    ["model_hash", receipt.model, receipt.model_hash],
    ["output_hash", receipt.output, receipt.output_hash],
  ].sort((a, b) => (a[0] as string).localeCompare(b[0] as string)) as [
    string,
    string | undefined,
    string | undefined,
  ][];

  for (const [name, plain, supplied] of checks) {
    if (!supplied) continue;
    const expected = hashField(plain ?? "");
    if (!hexEqual(expected, supplied)) {
      throw new ReceiptValidationError(name, expected, supplied);
    }
  }
  return receipt;
}

export function verifyReceiptBytes(
  payload: string | Uint8Array,
): ProofReceipt {
  const text =
    typeof payload === "string"
      ? payload
      : new TextDecoder("utf-8").decode(payload);
  const data = JSON.parse(text);
  if (typeof data !== "object" || data === null || Array.isArray(data)) {
    throw new Error("receipt must be a JSON object");
  }
  return verifyReceipt(data as Record<string, unknown>);
}
