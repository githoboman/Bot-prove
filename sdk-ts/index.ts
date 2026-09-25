/**
 * Public exports for the BotProve TypeScript SDK.
 *
 * Consumers import from `@botprove/sdk` (or the local relative path):
 *
 * ```ts
 * import { BotProveClient, verifyOffline } from "@botprove/sdk";
 * ```
 *
 * See `README.md` for a quickstart.
 */

export { BotProveClient, proofStatus } from "./client.ts";
export type { BotProveClientOptions } from "./client.ts";

export {
  APIError,
  BadRequestError,
  BotProveError,
  ForbiddenError,
  NetworkError,
  NotFoundError,
  RateLimitError,
  ServerError,
  UnauthorizedError,
  errorForStatus,
} from "./errors.ts";

export {
  blake2b256,
  blake2b256Hex,
  blake2b256OfString,
  bytesToHex,
  computeMerkleRoot,
  hexToBytes,
  verifyMerkleInclusion,
  verifyOffline,
} from "./verify.ts";
export type { OfflineVerifyReport } from "./verify.ts";

export type {
  APIErrorBody,
  BatchProofsRequest,
  ConsensusResult,
  GenerateProofRequest,
  HealthResponse,
  ListProofsQuery,
  ListProofsResponse,
  ProofRecord,
  ProofStatus,
  VerifierAttestation,
  VerifyProofRequest,
  VerifyProofResponse,
} from "./types.ts";
