/**
 * CasperProver TypeScript SDK - core client.
 *
 * Uses global `fetch` (Node >= 20.6 and every modern browser). No runtime
 * dependencies. Feature parity with sdk/primitives.go.
 */

export const DEFAULT_BASE_URL = "http://localhost:9090";
export const DEFAULT_TIMEOUT_MS = 30_000;

export interface ClientOptions {
  /** API base URL. Defaults to http://localhost:9090. */
  baseUrl?: string;
  /** Value sent as X-API-Key. Optional. */
  apiKey?: string;
  /** URL prefix; "v1" by default. Pass "" for legacy routes. */
  apiVersion?: string;
  /** Per-request timeout in milliseconds. */
  timeoutMs?: number;
  /** Optional custom fetch implementation (for tests). */
  fetch?: typeof fetch;
}

export interface RequestOptions {
  /** X-Idempotency-Key header value. */
  idempotencyKey?: string;
  /** Extra headers to attach to this one request. */
  headers?: Record<string, string>;
}

// -- request/response shapes ------------------------------------------------

export interface ProveRequest {
  agent?: string;
  input?: string;
  output?: string;
  model?: string;
  use_case?: string;
  mode?: string;
}

export interface ProveResponse {
  id: string;
  proof_hash?: string;
  vk_hash?: string;
  input_hash?: string;
  output_hash?: string;
  model_hash?: string;
  verdict?: string;
  created_at?: string;
  /** Full server response for fields the SDK has not surfaced yet. */
  raw?: Record<string, unknown>;
}

export interface VerifyResponse {
  valid: boolean;
  proof_id?: string;
  reason?: string;
  raw?: Record<string, unknown>;
}

export interface BatchItem {
  proof_id?: string;
  model_id?: string;
  input?: string;
  output?: string;
  [extra: string]: unknown;
}

export interface BatchResponse {
  verified?: string[];
  failed?: string[];
  total?: number;
  mode?: string;
  raw?: Record<string, unknown>;
}

export interface AnchorResponse {
  proof_id?: string;
  tx_hash?: string;
  block_hash?: string;
  anchored_at?: string;
  strict_mode?: boolean;
  deployer_key_id?: string;
  raw?: Record<string, unknown>;
}

/** Thrown when the CasperProver API returns a non-2xx status. */
export class ApiError extends Error {
  public readonly status: number;
  public readonly body: string;
  constructor(status: number, body: string) {
    super(`api error (status ${status}): ${body}`);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

export class Client {
  private readonly baseUrl: string;
  private readonly apiKey?: string;
  private readonly apiVersion: string;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof fetch;

  constructor(opts: ClientOptions = {}) {
    this.baseUrl = (opts.baseUrl ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
    this.apiKey = opts.apiKey;
    this.apiVersion = (opts.apiVersion ?? "v1").replace(/^\/+|\/+$/g, "");
    this.timeoutMs = opts.timeoutMs ?? DEFAULT_TIMEOUT_MS;
    this.fetchImpl = opts.fetch ?? fetch;
  }

  private prefix(): string {
    return this.apiVersion ? `/${this.apiVersion}` : "";
  }

  private async request<T>(
    method: string,
    path: string,
    body: unknown,
    ro: RequestOptions = {},
  ): Promise<T> {
    const headers: Record<string, string> = {};
    let bodyStr: string | undefined;
    if (body !== undefined && body !== null) {
      bodyStr = JSON.stringify(body);
      headers["Content-Type"] = "application/json";
    }
    if (this.apiKey) headers["X-API-Key"] = this.apiKey;
    if (ro.idempotencyKey) headers["X-Idempotency-Key"] = ro.idempotencyKey;
    if (ro.headers) Object.assign(headers, ro.headers);

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      const resp = await this.fetchImpl(this.baseUrl + path, {
        method,
        headers,
        body: bodyStr,
        signal: controller.signal,
      });
      const text = await resp.text();
      if (!resp.ok) throw new ApiError(resp.status, text);
      if (!text) return {} as T;
      return JSON.parse(text) as T;
    } finally {
      clearTimeout(timer);
    }
  }

  // -- primitives ----------------------------------------------------------

  async health(): Promise<Record<string, unknown>> {
    return this.request("GET", "/health", null);
  }

  async prove(req: ProveRequest, ro?: RequestOptions): Promise<ProveResponse> {
    // Prune undefined / empty-string fields to match Go's omitempty semantics.
    const body: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(req)) {
      if (v !== undefined && v !== "") body[k] = v;
    }
    const raw = await this.request<Record<string, unknown>>(
      "POST",
      `${this.prefix()}/proofs`,
      body,
      ro,
    );
    return { ...(raw as unknown as ProveResponse), raw };
  }

  async verify(proofId: string, ro?: RequestOptions): Promise<VerifyResponse> {
    const raw = await this.request<Record<string, unknown>>(
      "POST",
      `${this.prefix()}/verify`,
      { proof_id: proofId },
      ro,
    );
    return { ...(raw as unknown as VerifyResponse), raw };
  }

  async batch(
    proofs: BatchItem[],
    mode: string = "",
    ro?: RequestOptions,
  ): Promise<BatchResponse> {
    const body: Record<string, unknown> = {
      proofs: proofs.map((p) => {
        const out: Record<string, unknown> = {};
        for (const [k, v] of Object.entries(p)) {
          if (v !== undefined && v !== "") out[k] = v;
        }
        return out;
      }),
    };
    if (mode) body.mode = mode;
    const raw = await this.request<Record<string, unknown>>(
      "POST",
      `${this.prefix()}/batch/verify-zk`,
      body,
      ro,
    );
    return { ...(raw as BatchResponse), raw };
  }

  async anchor(
    proofId: string,
    ro?: RequestOptions,
  ): Promise<AnchorResponse> {
    const raw = await this.request<Record<string, unknown>>(
      "POST",
      `${this.prefix()}/proofs/${encodeURIComponent(proofId)}/anchor`,
      null,
      ro,
    );
    return { ...(raw as AnchorResponse), raw };
  }
}
