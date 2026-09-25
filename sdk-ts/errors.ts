/**
 * Typed exceptions thrown by the BotProve TypeScript SDK.
 *
 * All errors extend `BotProveError`, which extends the built-in `Error`.
 * The concrete subclass is chosen by HTTP status: 400 → BadRequestError,
 * 401 → UnauthorizedError, 403 → ForbiddenError, 404 → NotFoundError,
 * 429 → RateLimitError, 5xx → ServerError, everything else → APIError.
 * A network / abort failure becomes `NetworkError` (no status).
 */

/** Base class for all SDK errors. */
export class BotProveError extends Error {
  /** HTTP status code, when the error originated from an HTTP response. */
  readonly status?: number;
  /** Parsed response body, when available. */
  readonly body?: unknown;

  constructor(message: string, status?: number, body?: unknown) {
    super(message);
    this.name = "BotProveError";
    this.status = status;
    this.body = body;
    // Restore prototype chain when transpiled to ES5.
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/** 400 — the request was malformed or failed server-side validation. */
export class BadRequestError extends BotProveError {
  constructor(message: string, status = 400, body?: unknown) {
    super(message, status, body);
    this.name = "BadRequestError";
  }
}

/** 401 — API key missing or invalid. */
export class UnauthorizedError extends BotProveError {
  constructor(message: string, status = 401, body?: unknown) {
    super(message, status, body);
    this.name = "UnauthorizedError";
  }
}

/** 403 — the caller lacks the required scope. */
export class ForbiddenError extends BotProveError {
  constructor(message: string, status = 403, body?: unknown) {
    super(message, status, body);
    this.name = "ForbiddenError";
  }
}

/** 404 — the requested proof/resource does not exist. */
export class NotFoundError extends BotProveError {
  constructor(message: string, status = 404, body?: unknown) {
    super(message, status, body);
    this.name = "NotFoundError";
  }
}

/** 429 — the client is rate-limited. */
export class RateLimitError extends BotProveError {
  /** Optional Retry-After hint in seconds, if the server sent one. */
  readonly retryAfterSec?: number;

  constructor(message: string, status = 429, body?: unknown, retryAfterSec?: number) {
    super(message, status, body);
    this.name = "RateLimitError";
    this.retryAfterSec = retryAfterSec;
  }
}

/** 5xx — server-side failure. */
export class ServerError extends BotProveError {
  constructor(message: string, status: number, body?: unknown) {
    super(message, status, body);
    this.name = "ServerError";
  }
}

/** Unclassified HTTP error. */
export class APIError extends BotProveError {
  constructor(message: string, status: number, body?: unknown) {
    super(message, status, body);
    this.name = "APIError";
  }
}

/** Network / abort failure — no HTTP status. */
export class NetworkError extends BotProveError {
  constructor(message: string, cause?: unknown) {
    super(message);
    this.name = "NetworkError";
    if (cause !== undefined) {
      (this as { cause?: unknown }).cause = cause;
    }
  }
}

/**
 * Pick the right error subclass for an HTTP status code.
 * `body` is a best-effort parse of the response payload (may be undefined).
 */
export function errorForStatus(status: number, message: string, body?: unknown): BotProveError {
  if (status === 400) return new BadRequestError(message, status, body);
  if (status === 401) return new UnauthorizedError(message, status, body);
  if (status === 403) return new ForbiddenError(message, status, body);
  if (status === 404) return new NotFoundError(message, status, body);
  if (status === 429) return new RateLimitError(message, status, body);
  if (status >= 500 && status < 600) return new ServerError(message, status, body);
  return new APIError(message, status, body);
}
