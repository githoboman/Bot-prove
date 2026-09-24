/**
 * CasperProver server DTOs, mirrored from `engine/internal/prover/types.go`
 * and `engine/internal/api/server.go`. Field names match the on-wire JSON.
 */

/** A proof record as returned by the CasperProver API. */
export interface ProofRecord {
  /** Unique proof identifier (server-generated). */
  id: string;
  /** Logical agent name that produced the inference (e.g. "agent-42"). */
  agent: string;
  /** Hash of the canonical proof envelope. */
  proof_hash: string;
  /** Hash of the input payload. */
  input_hash: string;
  /** Hash of the output payload. */
  output_hash: string;
  /** Hash of the model identifier. */
  model_hash: string;
  /** Merkle root over the {ih, oh, mh, ph} leaves. */
  merkle_root: string;
  /** Sibling hashes along the path from leaf to root. */
  merkle_path: string[];
  /** Position of this leaf within the tree (0-indexed). */
  leaf_index: number;
  /** UNIX timestamp (seconds) when the proof was generated. */
  timestamp: number;
  /** True if the server considered the proof valid at generation. */
  valid: boolean;
  /** True if the proof has been explicitly revoked. */
  revoked: boolean;
  /** Optional business-domain tag. */
  use_case: string;
  /** Optional ed25519 public key of the submitter, hex-encoded. */
  public_key?: string;
  /** Deploy hash on Casper (populated only for `anchored` mode). */
  deploy_hash?: string;
  /** Milliseconds the server took to generate the proof. */
  generation_ms: number;
  /** "local" (in-memory) or "anchored" (submitted to Casper). */
  mode?: "local" | "anchored" | string;
}

/** Submission payload for `POST /proofs`. */
export interface GenerateProofRequest {
  agent: string;
  input: string;
  output: string;
  model: string;
  use_case?: string;
  /** Defaults to "local" server-side when omitted. */
  mode?: "local" | "anchored";
}

/** Batch submission for `POST /proofs/batch`. */
export interface BatchProofsRequest {
  proofs: Array<Omit<GenerateProofRequest, "mode">>;
  mode?: "local" | "anchored";
}

/** Response envelope for `POST /verify`. */
export interface VerifyProofResponse {
  proof_id: string;
  valid: boolean;
  revoked: boolean;
  /** Set only when input+output+model were echoed back for full verify. */
  verified?: boolean;
  /** Set on verified=false. */
  error?: string;
  /** Set when full verification was requested. */
  checks?: {
    input_hash_match: boolean;
    output_hash_match: boolean;
    model_hash_match: boolean;
    merkle_path_valid?: boolean;
  };
}

/** Payload for `POST /verify`. */
export interface VerifyProofRequest {
  proof_id: string;
  /** When provided together with `output`+`model`, the server also recomputes the hashes. */
  input?: string;
  output?: string;
  model?: string;
}

/** Response for `GET /proofs`. */
export interface ListProofsResponse {
  proofs: ProofRecord[];
  total: number;
  page: number;
  limit: number;
}

/** Query filters for `GET /proofs`. */
export interface ListProofsQuery {
  agent?: string;
  public_key?: string;
  mode?: "local" | "anchored";
  page?: number;
  /** 1..100, defaults to 20 server-side. */
  limit?: number;
}

/** Health-check envelope from `GET /health`. */
export interface HealthResponse {
  status: string;
  [key: string]: unknown;
}

/** Attestation returned when a verifier votes on a proof (external verifiers). */
export interface VerifierAttestation {
  verifier_id: string;
  proof_id: string;
  vote: "ACCEPT" | "REJECT" | "ABSTAIN";
  timestamp: number;
  signature?: string;
  reason?: string;
}

/** Consensus outcome across multiple attestations for the same proof. */
export interface ConsensusResult {
  proof_id: string;
  accepts: number;
  rejects: number;
  abstains: number;
  quorum: number;
  outcome: "ACCEPTED" | "REJECTED" | "ABSTAIN" | "PENDING";
  attestations: VerifierAttestation[];
}

/** Server-side proof status values. */
export type ProofStatus = "valid" | "revoked" | "invalid";

/** Body returned by the API on error (best-effort — some routes return plain text). */
export interface APIErrorBody {
  error?: string;
  message?: string;
  [key: string]: unknown;
}
