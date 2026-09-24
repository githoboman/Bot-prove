/**
 * Public exports for the CasperProver TypeScript SDK.
 *
 * Consumers import from `@casperprover/sdk` (or the local relative path):
 *
 * ```ts
 * import { CasperProverClient, verifyOffline } from "@casperprover/sdk";
 * ```
 *
 * See `README.md` for a quickstart.
 */

export { CasperProverClient, proofStatus } from "./client.ts";
export type { CasperProverClientOptions } from "./client.ts";

export {
  APIError,
  BadRequestError,
  CasperProverError,
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
