
const BASE_URL = '/prover-api';

// --- Utility Types ---
export type ApiResponse<T> = {
  success: boolean;
  data?: T;
  error?: string;
  message?: string;
};

async function fetcher<T>(
  endpoint: string,
  options?: RequestInit
): Promise<ApiResponse<T>> {
  try {
    const response = await fetch(`${BASE_URL}${endpoint}`, {
      headers: {
        'Content-Type': 'application/json',
        ...options?.headers,
      },
      ...options,
    });

    const data = await response.json();

    if (!response.ok) {
      return { success: false, error: data.error || data.message || 'An unknown error occurred', message: data.message };
    }

    return { success: true, data };
  } catch (error) {
    if (import.meta.env.DEV) console.error(`API call to ${endpoint} failed:`, error);
    return { success: false, error: (error as Error).message || 'Network error' };
  }
}

// ============================================================================
// Types — all field names match the Go engine's JSON output (snake_case)
// ============================================================================

// Health
export interface HealthResponse {
  status: string
  version: string
  uptime_s: number
  total_proofs: number
  chain: string
  contracts: Record<string, string>
}

// Proofs
export interface Proof {
  id: string
  agent: string
  proof_hash: string
  input_hash: string
  output_hash: string
  model_hash: string
  merkle_root: string
  merkle_path: string[]
  leaf_index: number
  timestamp: number
  valid: boolean
  revoked: boolean
  use_case: string
  public_key: string
  deploy_hash: string
  generation_ms: number
  mode: string
}

export interface ProofsListResponse {
  proofs: Proof[]
  total: number
  page: number
  limit: number
}

// Submit proof — matches server.go submitProof handler
export interface CreateProofRequest {
  agent: string
  input: string
  output: string
  model: string
  use_case?: string
  mode?: string  // "local" | "anchored"
}

// Verify proof — matches server.go verifyProof handler
export interface VerifyProofRequest {
  proof_id: string
  input?: string
  output?: string
  model?: string
}

export interface VerifyProofResponse {
  proof_id: string
  valid: boolean
  revoked: boolean
  verified?: boolean
  error?: string
  checks?: Record<string, boolean>
}

// Stats
export interface StatsResponse {
  total_proofs: number
  valid_proofs: number
  revoked_proofs: number
  unique_agents: number
  avg_generation_ms: number
  max_merkle_depth: number
  use_cases: Record<string, number>
}

// KYC
export interface KYCCheckRequest {
  proof_id: string
}

export interface KYCGrantRequest {
  user: string
  proof_id: string
}

export interface KYCWhitelistResponse {
  user: string
  whitelisted: boolean
}

// Inference
export interface RegisterModelRequest {
  model_id: string
  model_hash: string
  verifier_contract?: string
  metadata?: Record<string, string>
}

export interface InferenceProveRequest {
  agent: string
  model_id: string
  input: string
  output: string
  use_case?: string
  public_key?: string
}

export interface InferenceVerifyRequest {
  proof_id: string
}

// Aggregation
export interface CreateBatchRequest {
  batch_id: string
  merkle_root?: string
  max_proofs?: number
}

export interface AddProofToBatchRequest {
  batch_id: string
  proof_hash: string
  leaf_index?: number
}

export interface FinalizeBatchRequest {
  batch_id: string
}

// ZK
export interface ZKVerifyGroth16Request {
  proof: string
  public_inputs: string[]
  vk_hash?: string
}

export interface ZKGroth16RealProveRequest {
  preimage: string
}

export interface ZKGroth16RealVerifyRequest {
  hash: string
  proof_hex: string
}

export interface ZKChallengeRequest {
  proof_id: string
  reason: string
}

// PQ
export interface PQSignRequest {
  message: string
}

export interface PQVerifyRequest {
  message: string
  signature: string
  public_key: string
}

export interface PQHybridSignRequest {
  message: string
}

export interface PQHybridVerifyRequest {
  message: string
  signature: string
  classic_public_key: string
  pq_public_key: string
}

// ============================================================================
// API Functions
// ============================================================================

// Main
export const getHealth = () => fetcher<HealthResponse>('/health');
export const getProofs = (agent?: string, page: number = 1, limit: number = 10) =>
  fetcher<ProofsListResponse>(`/proofs?page=${page}&limit=${limit}${agent ? `&agent=${agent}` : ''}`);
export const getProofById = (id: string) => fetcher<Proof>(`/proofs/${id}`);
export const createProof = (data: CreateProofRequest) =>
  fetcher<Proof>('/proofs', { method: 'POST', body: JSON.stringify(data) });
export const verifyProof = (data: VerifyProofRequest) =>
  fetcher<VerifyProofResponse>('/verify', { method: 'POST', body: JSON.stringify(data) });
export const revokeProof = (id: string, publicKey?: string) =>
  fetcher<{ proof_id: string; revoked: boolean }>(`/proofs/${id}/revoke`, {
    method: 'POST',
    body: JSON.stringify({}),
    ...(publicKey ? { headers: { 'Content-Type': 'application/json', 'X-Public-Key': publicKey } } : {}),
  });
export const exportProof = (id: string) =>
  fetcher<any>(`/proofs/${id}/export`, { method: 'GET' });

export const getStats = () => fetcher<StatsResponse>('/stats');

// KYC
export const checkKycStatus = (data: KYCCheckRequest) =>
  fetcher<any>('/kyc/check', { method: 'POST', body: JSON.stringify(data) });
export const grantKycAccess = (data: KYCGrantRequest) =>
  fetcher<any>('/kyc/grant', { method: 'POST', body: JSON.stringify(data) });
export const getKycWhitelist = (user: string) =>
  fetcher<KYCWhitelistResponse>(`/kyc/whitelist/${user}`);

// Inference
export const registerModel = (data: RegisterModelRequest) =>
  fetcher<any>('/inference/register-model', { method: 'POST', body: JSON.stringify(data) });
export const getModelById = (id: string) =>
  fetcher<any>(`/inference/model/${id}`);
export const inferenceProve = (data: InferenceProveRequest) =>
  fetcher<Proof>('/inference/prove', { method: 'POST', body: JSON.stringify(data) });
export const inferenceVerify = (data: InferenceVerifyRequest) =>
  fetcher<any>('/inference/verify', { method: 'POST', body: JSON.stringify(data) });

// Aggregation
export const createAggregationBatch = (data: CreateBatchRequest) =>
  fetcher<any>('/aggregation/create-batch', { method: 'POST', body: JSON.stringify(data) });
export const addProofToAggregationBatch = (data: AddProofToBatchRequest) =>
  fetcher<any>('/aggregation/add-proof', { method: 'POST', body: JSON.stringify(data) });
export const finalizeAggregationBatch = (data: FinalizeBatchRequest) =>
  fetcher<any>('/aggregation/finalize', { method: 'POST', body: JSON.stringify(data) });
export const getAggregationBatchById = (id: string) =>
  fetcher<any>(`/aggregation/batch/${id}`);
export const verifyAggregationBatch = (id: string) =>
  fetcher<any>(`/aggregation/verify-batch/${id}`);

// ZK
export const verifyGroth16 = (data: ZKVerifyGroth16Request) =>
  fetcher<any>('/zk/verify-groth16', { method: 'POST', body: JSON.stringify(data) });
export const batchVerifyZK = (data: { proofs: { proof: string; public_inputs: string[] }[] }) =>
  fetcher<any>('/zk/batch-verify', { method: 'POST', body: JSON.stringify(data) });
export const zkGroth16RealProve = (data: ZKGroth16RealProveRequest) =>
  fetcher<any>('/zk/groth16-real/prove', { method: 'POST', body: JSON.stringify(data) });
export const zkGroth16RealVerify = (data: ZKGroth16RealVerifyRequest) =>
  fetcher<any>('/zk/groth16-real/verify', { method: 'POST', body: JSON.stringify(data) });
export const challengeZK = (data: ZKChallengeRequest) =>
  fetcher<any>('/zk/challenge', { method: 'POST', body: JSON.stringify(data) });
export const getZKChallengeById = (id: string) =>
  fetcher<any>(`/zk/challenge/${id}`);

// PQ
export const signSphincs = (data: PQSignRequest) =>
  fetcher<any>('/pq/sign-sphincs', { method: 'POST', body: JSON.stringify(data) });
export const verifySphincs = (data: PQVerifyRequest) =>
  fetcher<any>('/pq/verify-sphincs', { method: 'POST', body: JSON.stringify(data) });
export const hybridSign = (data: PQHybridSignRequest) =>
  fetcher<any>('/pq/hybrid-sign', { method: 'POST', body: JSON.stringify(data) });
export const hybridVerify = (data: PQHybridVerifyRequest) =>
  fetcher<any>('/pq/hybrid-verify', { method: 'POST', body: JSON.stringify(data) });

// Batch proofs
export interface BatchProofsRequest {
  proofs: { agent: string; model_hash: string; input_data: string; use_case?: string }[];
  mode?: string;
}
export const batchProofs = (data: BatchProofsRequest) =>
  fetcher<any>('/proofs/batch', { method: 'POST', body: JSON.stringify(data) });

// Phase 2: Proof Chain
export interface ProofChainStep {
  proof_id: string;
  parent_ids: string[];
  model_hash: string;
  input_hash: string;
  output_hash: string;
  step_index: number;
}
export interface ValidateProofChainRequest {
  id?: string;
  steps: ProofChainStep[];
}
export const validateProofChain = (data: ValidateProofChainRequest) =>
  fetcher<any>('/proof-chain/validate', { method: 'POST', body: JSON.stringify(data) });
