import React, { useState, useCallback, useMemo, useEffect, useRef } from 'react';
import {
  Play,
  Code,
  AlertTriangle,
  Loader2,
  Box,
  FlaskConical,
  ShieldCheck,
  GitMerge,
  PlusCircle,
  CheckCircle,
  XCircle,
  Terminal,
} from 'lucide-react';
import * as api from '../../lib/api';
import { toast } from '../ui/toast';
import SectionIntro from './SectionIntro';
import { useWallet } from '../../lib/WalletProvider';
import { submitProofOnChain, registerAgentOnChain } from '../../lib/liveTx';
import { getCachedManifest } from '../../lib/onchain';
function getContractHash(key: string): string | null {
  return getCachedManifest()?.contracts?.[key]?.contract_hash ?? null;
}

/* ================================================================
   SHARED TYPES
   ================================================================ */

interface EndpointConfig {
  name: string;
  method: 'GET' | 'POST';
  path: string;
  description: string;
  exampleBody?: string;
  apiCall: (data?: any) => Promise<api.ApiResponse<any>>;
  params?: { name: string; type: 'string' | 'number'; optional?: boolean; default?: any }[];
}

/* ================================================================
   API PLAYGROUND ENDPOINTS (unchanged)
   ================================================================ */

const playgroundEndpoints: EndpointConfig[] = [
  { name: 'GET /health', method: 'GET', path: '/health', description: 'Check the health status of the API.', apiCall: api.getHealth },
  { name: 'GET /proofs', method: 'GET', path: '/proofs', description: 'Retrieve a list of proofs with optional filtering and pagination.', apiCall: (params) => api.getProofs(params?.agent, params?.page, params?.limit), params: [{ name: 'agent', type: 'string', optional: true, default: '' }, { name: 'page', type: 'number', optional: true, default: 1 }, { name: 'limit', type: 'number', optional: true, default: 10 }] },
  { name: 'GET /proofs/{id}', method: 'GET', path: '/proofs/{id}', description: 'Retrieve details for a specific proof by ID.', apiCall: (params) => api.getProofById(params.id), params: [{ name: 'id', type: 'string', optional: false, default: 'P-5' }] },
  { name: 'POST /proofs', method: 'POST', path: '/proofs', description: 'Create a Merkle-inclusion proof commitment (input/output/model hashes). Not a ZK proof.', exampleBody: JSON.stringify({ agent: 'agent-alpha', input: 'loan_application_42', output: 'approved_with_conditions', model: 'gpt-4o', use_case: 'kyc-eligibility' }, null, 2), apiCall: api.createProof },
  { name: 'POST /verify', method: 'POST', path: '/verify', description: 'Verify an existing Merkle-inclusion proof.', exampleBody: JSON.stringify({ proof_id: 'P-5' }, null, 2), apiCall: api.verifyProof },
  { name: 'POST /proofs/{id}/revoke', method: 'POST', path: '/proofs/{id}/revoke', description: 'Revoke a specific proof commitment.', apiCall: (params) => api.revokeProof(params.id), params: [{ name: 'id', type: 'string', optional: false, default: 'P-5' }] },
  { name: 'GET /stats', method: 'GET', path: '/stats', description: 'Get overall statistics about the Bot Prove system.', apiCall: api.getStats },
  { name: 'POST /kyc/check', method: 'POST', path: '/kyc/check', description: 'Check KYC status for a user.', exampleBody: JSON.stringify({ user_id: 'alice-agent-01' }, null, 2), apiCall: api.checkKycStatus },
  { name: 'POST /kyc/grant', method: 'POST', path: '/kyc/grant', description: 'Grant KYC access to a user.', exampleBody: JSON.stringify({ user_id: 'alice-agent-01', reason: 'Manual verification after document submission' }, null, 2), apiCall: api.grantKycAccess },
  { name: 'GET /kyc/whitelist/{user}', method: 'GET', path: '/kyc/whitelist/{user}', description: 'View KYC whitelist for a specific user.', apiCall: (params) => api.getKycWhitelist(params.user), params: [{ name: 'user', type: 'string', optional: false, default: 'alice-agent-01' }] },
  { name: 'POST /inference/register-model', method: 'POST', path: '/inference/register-model', description: 'Register a new AI model.', exampleBody: JSON.stringify({ model_id: 'fraud-detection-v1', model_hash: 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2', verifier_contract: getContractHash('proof_registry') ?? '', metadata: { type: 'llm', params: '175B' } }, null, 2), apiCall: api.registerModel },
  { name: 'GET /inference/model/{id}', method: 'GET', path: '/inference/model/{id}', description: 'Get details of a registered AI model.', apiCall: (params) => api.getModelById(params.id), params: [{ name: 'id', type: 'string', optional: false, default: 'fraud-detection-v1' }] },
  { name: 'POST /inference/prove', method: 'POST', path: '/inference/prove', description: 'Record an inference: commits SHA-256 hashes of input/output/model to the Merkle tree. Does not execute the model.', exampleBody: JSON.stringify({ model_id: 'fraud-detection-v1', input: '{"amount": 1000, "risk": "high"}', agent: 'agent-alpha', output: '{"decision": "approved"}', use_case: 'kyc-eligibility' }, null, 2), apiCall: api.inferenceProve },
  { name: 'POST /inference/verify', method: 'POST', path: '/inference/verify', description: 'Verify a previously recorded inference commitment against the Merkle root.', exampleBody: JSON.stringify({ model_id: 'fraud-detection-v1', proof_id: 'P-5', input: '{"amount": 1000, "risk": "high"}' }, null, 2), apiCall: api.inferenceVerify },
  { name: 'POST /aggregation/create-batch', method: 'POST', path: '/aggregation/create-batch', description: 'Create a new batch for proof aggregation.', exampleBody: JSON.stringify({ batch_id: 'batch-demo-50584', max_proofs: 50 }, null, 2), apiCall: api.createAggregationBatch },
  { name: 'POST /aggregation/add-proof', method: 'POST', path: '/aggregation/add-proof', description: 'Add a proof to an existing aggregation batch.', exampleBody: JSON.stringify({ batch_id: 'batch-demo', proof_id: 'P-5' }, null, 2), apiCall: api.addProofToAggregationBatch },
  { name: 'POST /aggregation/finalize', method: 'POST', path: '/aggregation/finalize', description: 'Finalize an aggregation batch.', exampleBody: JSON.stringify({ batch_id: 'batch-demo' }, null, 2), apiCall: api.finalizeAggregationBatch },
  { name: 'GET /aggregation/batch/{id}', method: 'GET', path: '/aggregation/batch/{id}', description: 'Get details of an aggregation batch.', apiCall: (params) => api.getAggregationBatchById(params.id), params: [{ name: 'id', type: 'string', optional: false, default: 'batch-demo' }] },
  { name: 'POST /zk/verify-groth16', method: 'POST', path: '/zk/verify-groth16', description: 'Conceptual Groth16 verification (hash-based simulation). For the real BN254 pairing check, use /zk/groth16-real/verify.', exampleBody: JSON.stringify({ proof: '{"pi_a": ["0x...", "0x..."], "pi_b": [["0x...", "0x..."], ["0x...", "0x..."]], "pi_c": ["0x...", "0x..."]}', public_signals: ['0xsignal1', '0xsignal2'] }, null, 2), apiCall: api.verifyGroth16 },
  { name: 'POST /zk/batch-verify', method: 'POST', path: '/zk/batch-verify', description: 'Batch verify multiple ZK proofs.', exampleBody: JSON.stringify({ proofIds: ['proof-id-1', 'proof-id-2'] }, null, 2), apiCall: api.batchVerifyZK },
  { name: 'POST /zk/challenge', method: 'POST', path: '/zk/challenge', description: 'Create a new ZK challenge.', exampleBody: JSON.stringify({ challengerId: 'challenger-1', proof_id: 'P-5', challengeData: '{"challengeType": "recompute", "params": {"nonce": 123}}' }, null, 2), apiCall: api.challengeZK },
  { name: 'GET /zk/challenge/{id}', method: 'GET', path: '/zk/challenge/{id}', description: 'Get details of a ZK challenge.', apiCall: (params) => api.getZKChallengeById(params.id), params: [{ name: 'id', type: 'string', optional: false, default: 'example-challenge-id' }] },
  { name: 'POST /pq/sign-sphincs', method: 'POST', path: '/pq/sign-sphincs', description: 'Sign using hash-based OTS (Lamport, occupies the SPHINCS+ family slot).', exampleBody: JSON.stringify({ message: 'This is a test message for hash-based OTS signing.' }, null, 2), apiCall: api.signSphincs },
  { name: 'POST /pq/verify-sphincs', method: 'POST', path: '/pq/verify-sphincs', description: 'Verify a hash-based OTS (Lamport) signature.', exampleBody: JSON.stringify({ message: 'This is a test message for hash-based OTS signing.', signature: '0x...', public_key: '0x...' }, null, 2), apiCall: api.verifySphincs },
  { name: 'POST /pq/hybrid-sign', method: 'POST', path: '/pq/hybrid-sign', description: 'Sign using hybrid (classical + PQ) cryptography.', exampleBody: JSON.stringify({ message: 'This is a test message for hybrid signing.' }, null, 2), apiCall: api.hybridSign },
  { name: 'POST /pq/hybrid-verify', method: 'POST', path: '/pq/hybrid-verify', description: 'Verify a hybrid signature.', exampleBody: JSON.stringify({ message: 'This is a test message for hybrid signing.', classical_signature: '0x...', pq_signature: '0x...', classicalPublicKey: '0x...', pqPublicKey: '0x...' }, null, 2), apiCall: api.hybridVerify },
  { name: 'POST /zk/groth16-real/prove', method: 'POST', path: '/zk/groth16-real/prove', description: 'Generate a real BN254 Groth16 proof (MiMC preimage via gnark).', exampleBody: JSON.stringify({ preimage: '42' }, null, 2), apiCall: api.zkGroth16RealProve },
  { name: 'POST /zk/groth16-real/verify', method: 'POST', path: '/zk/groth16-real/verify', description: 'Verify a real BN254 Groth16 proof.', exampleBody: JSON.stringify({ hash: '...', proof_hex: '...' }, null, 2), apiCall: api.zkGroth16RealVerify },
  { name: 'GET /aggregation/verify-batch/{id}', method: 'GET', path: '/aggregation/verify-batch/{id}', description: 'Verify a finalized aggregation batch.', params: [{ name: 'id', type: 'string', optional: false, default: 'batch-demo' }], apiCall: (params: any) => api.verifyAggregationBatch(params.id) },
  { name: 'GET /proofs/{id}/export', method: 'GET', path: '/proofs/{id}/export', description: 'Export a proof as a portable JSON bundle.', params: [{ name: 'id', type: 'string', optional: false, default: 'P-1' }], apiCall: (params: any) => api.exportProof(params.id) },
  { name: 'POST /proofs/batch', method: 'POST', path: '/proofs/batch', description: 'Submit multiple proofs in a single request.', exampleBody: JSON.stringify({ proofs: [{ agent: 'agent-1', model_hash: 'sha256:abc123', input_data: '{"x": 1}', use_case: 'inference' }, { agent: 'agent-1', model_hash: 'sha256:abc123', input_data: '{"x": 2}', use_case: 'inference' }], mode: 'parallel' }, null, 2), apiCall: (body: any) => api.batchProofs(body) },
  { name: 'POST /proof-chain/validate', method: 'POST', path: '/proof-chain/validate', description: 'Validate a proof-chain DAG.', exampleBody: JSON.stringify({ id: 'chain-demo', steps: [{ proof_id: 'step-0', parent_ids: [], model_hash: 'sha256:model-a', input_hash: 'aaa111', output_hash: 'bbb222', step_index: 0 }, { proof_id: 'step-1', parent_ids: ['step-0'], model_hash: 'sha256:model-b', input_hash: 'bbb222', output_hash: 'ccc333', step_index: 1 }, { proof_id: 'step-2', parent_ids: ['step-1'], model_hash: 'sha256:model-c', input_hash: 'ccc333', output_hash: 'ddd444', step_index: 2 }] }, null, 2), apiCall: (body: any) => api.validateProofChain(body) },
];

/* ================================================================
   AGENT PLAYGROUND — pipeline step definitions
   ================================================================ */

const genId = (prefix: string) => `${prefix}-${Math.random().toString(36).substring(2, 10)}`;

interface PipelineStep {
  id: number;
  name: string;
  icon: React.ElementType;
  endpoint: string;
  description: string;
  action: () => Promise<any>;
  status: 'idle' | 'loading' | 'success' | 'error';
  response: any;
  error: string | null;
}

/* ================================================================
   AGENT PLAYGROUND COMPONENT
   ================================================================ */

const AgentPlayground: React.FC = () => {
  const { publicKey, connected: isWalletConnected, clickRef } = useWallet();
  const [isDemoRunning, setIsDemoRunning] = useState(false);
  const [globalError, setGlobalError] = useState<string | null>(null);

  const modelIdRef = useRef('');
  const proofIdRef = useRef('');
  const batchIdRef = useRef('');
  const proofHashRef = useRef('');
  const proofInputHashRef = useRef('');
  const proofOutputHashRef = useRef('');
  const proofModelHashRef = useRef('');

  // Use wallet publicKey as agent when connected, random ID otherwise
  const [agentId] = useState(() => genId('agent'));
  const effectiveAgent = isWalletConnected && publicKey ? publicKey : agentId;
  const inputData = JSON.stringify({ temperature: 0.7, prompt: 'Generate a secure proof for AI decision.' });
  const outputData = JSON.stringify({ decision: 'approved', confidence: 0.95, risk_score: 12 });

  const initialSteps: PipelineStep[] = [
    {
      id: 1, name: 'Register AI Model', icon: Box, endpoint: 'POST /inference/register-model',
      description: 'Register a new AI model on the Bot Prove registry.',
      action: async () => {
        const mid = genId('model');
        const res = await api.registerModel({ model_id: mid, model_hash: genId('hash'), verifier_contract: getContractHash('verifier_gate') ?? '', metadata: { type: 'decision-box', version: '1.0' } });
        if (res.success && res.data) { modelIdRef.current = res.data.model_id || res.data.id || mid; return res.data; }
        throw new Error(res.error || 'Failed to register model');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 2, name: 'Run Inference & Prove', icon: FlaskConical, endpoint: 'POST /inference/prove',
      description: 'Run an AI inference and generate a cryptographic proof.',
      action: async () => {
        if (!modelIdRef.current) throw new Error('Model ID not available. Complete Step 1 first.');
        const res = await api.inferenceProve({ model_id: modelIdRef.current, input: inputData, agent: effectiveAgent, output: outputData, use_case: 'kyc-eligibility' });
        if (res.success && res.data) {
          proofIdRef.current = res.data.id || '';
          proofHashRef.current = res.data.proof_hash || '';
          proofInputHashRef.current = res.data.input_hash || '';
          proofOutputHashRef.current = res.data.output_hash || '';
          proofModelHashRef.current = res.data.model_hash || '';
          return res.data;
        }
        throw new Error(res.error || 'Failed to generate inference proof');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 3, name: 'Verify Proof', icon: ShieldCheck, endpoint: 'POST /inference/verify',
      description: 'Verify the generated proof on the Bot Prove engine.',
      action: async () => {
        if (!proofIdRef.current) throw new Error('Proof ID not available. Complete Step 2 first.');
        const res = await api.inferenceVerify({ proof_id: proofIdRef.current });
        if (res.success && res.data) return res.data;
        throw new Error(res.error || 'Failed to verify proof');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 4, name: 'Create Proof Batch', icon: GitMerge, endpoint: 'POST /aggregation/create-batch',
      description: 'Create a new batch for aggregating multiple proofs.',
      action: async () => {
        const bid = genId('batch');
        const res = await api.createAggregationBatch({ batch_id: bid, max_proofs: 50 });
        if (res.success && res.data) { batchIdRef.current = res.data.batch_id || bid; return res.data; }
        throw new Error(res.error || 'Failed to create batch');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 5, name: 'Add Proof to Batch', icon: PlusCircle, endpoint: 'POST /aggregation/add-proof',
      description: 'Add the generated proof to the aggregation batch.',
      action: async () => {
        if (!batchIdRef.current) throw new Error('Batch ID not available.');
        if (!proofHashRef.current) throw new Error('Proof hash not available.');
        const res = await api.addProofToAggregationBatch({ batch_id: batchIdRef.current, proof_hash: proofHashRef.current });
        if (res.success) return res.data;
        throw new Error(res.error || 'Failed to add proof to batch');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 6, name: 'Finalize & Aggregate', icon: CheckCircle, endpoint: 'POST /aggregation/finalize',
      description: 'Finalize the batch, generating an aggregated Merkle root.',
      action: async () => {
        if (!batchIdRef.current) throw new Error('Batch ID not available.');
        const res = await api.finalizeAggregationBatch({ batch_id: batchIdRef.current });
        if (res.success) return res.data;
        throw new Error(res.error || 'Failed to finalize batch');
      },
      status: 'idle', response: null, error: null,
    },
  ];

  const [steps, setSteps] = useState<PipelineStep[]>(initialSteps);

  const resetDemo = useCallback(() => {
    setSteps(initialSteps);
    setIsDemoRunning(false);
    setGlobalError(null);
    modelIdRef.current = '';
    proofIdRef.current = '';
    batchIdRef.current = '';
    proofHashRef.current = '';
    proofInputHashRef.current = '';
    proofOutputHashRef.current = '';
    proofModelHashRef.current = '';
  }, []);

  const runStep = useCallback(async (index: number) => {
    setSteps((prev) => prev.map((s, i) => (i === index ? { ...s, status: 'loading', error: null, response: null } : s)));
    setGlobalError(null);
    try {
      const response = await steps[index].action();
      setSteps((prev) => prev.map((s, i) => (i === index ? { ...s, status: 'success', response } : s)));
      toast.success(`Step ${index + 1}: ${steps[index].name} completed!`);
      return true;
    } catch (err: any) {
      const msg = err.message || 'An unknown error occurred.';
      setSteps((prev) => prev.map((s, i) => (i === index ? { ...s, status: 'error', error: msg } : s)));
      setGlobalError(`Step ${index + 1} failed: ${msg}`);
      toast.error(`Step ${index + 1}: ${steps[index].name} failed!`);
      return false;
    }
  }, [steps]);

  const runFullDemo = useCallback(async () => {
    resetDemo();
    setIsDemoRunning(true);
    for (let i = 0; i < steps.length; i++) {
      const success = await runStep(i);
      if (!success) { setIsDemoRunning(false); return; }
      await new Promise((r) => setTimeout(r, 800));
    }
    // After pipeline completes, anchor proof on-chain if wallet connected
    if (isWalletConnected && clickRef && publicKey && proofHashRef.current) {
      toast.info('Pipeline done — sign to anchor proof on-chain…');
      try {
        const txResult = await submitProofOnChain(clickRef, {
          proofHash: proofHashRef.current,
          inputHash: proofInputHashRef.current,
          outputHash: proofOutputHashRef.current,
          modelHash: proofModelHashRef.current,
          senderPublicKeyHex: publicKey,
        });
        if (txResult.ok) {
          toast.success(`Pipeline completed & anchored on-chain! Tx: ${txResult.transactionHash.substring(0, 16)}…`);
        } else if ('cancelled' in txResult && txResult.cancelled) {
          toast.success('Pipeline completed (on-chain anchoring skipped).');
        } else {
          toast.success(`Pipeline completed (on-chain failed: ${'error' in txResult ? txResult.error : 'unknown'}).`);
        }
      } catch (err: any) {
        toast.success(`Pipeline completed (on-chain error: ${err.message}).`);
      }
    } else {
      toast.success('Full pipeline completed!');
    }

    setIsDemoRunning(false);
  }, [steps, runStep, resetDemo, isWalletConnected, clickRef, publicKey]);

  const statusBorder = (s: PipelineStep['status']) => {
    switch (s) {
      case 'success': return 'border-green-600 bg-green-900/10';
      case 'error': return 'border-red-600 bg-red-900/10';
      case 'loading': return 'border-blue-500 bg-blue-900/10';
      default: return 'border-[#222235] bg-[#1a1a2a]';
    }
  };

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 mb-6">
        <button onClick={runFullDemo} disabled={isDemoRunning}
          className="flex items-center gap-2 px-5 py-2.5 bg-red-600 hover:bg-red-700 text-white rounded-md text-sm font-medium transition-colors disabled:opacity-50">
          {isDemoRunning ? <Loader2 size={18} className="animate-spin" /> : <Play size={18} />}
          {isDemoRunning ? 'Running…' : 'Run Full Pipeline'}
        </button>
        <button onClick={resetDemo} disabled={isDemoRunning}
          className="flex items-center gap-2 px-5 py-2.5 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-md text-sm font-medium transition-colors disabled:opacity-50">
          <XCircle size={18} /> Reset
        </button>
        <span className="text-gray-500 text-xs ml-auto">6 sequential API calls · model → proof → verify → batch → add → finalize</span>
      </div>

      {globalError && (
        <div className="p-3 bg-red-900/30 text-red-300 border border-red-700 rounded-md mb-4 flex items-center gap-2 text-sm">
          <AlertTriangle size={16} /> {globalError}
        </div>
      )}

      {/* Pipeline rows: 2-column — left: step card, right: response */}
      <div className="space-y-3">
        {steps.map((step) => (
          <div key={step.id}
            className={`grid grid-cols-1 lg:grid-cols-2 gap-0 rounded-lg border overflow-hidden transition-all duration-300 ${statusBorder(step.status)}`}
            style={{ minHeight: '120px' }}
          >
            {/* Left: step info */}
            <div className="flex items-start gap-3 p-4 border-b lg:border-b-0 lg:border-r border-[#222235]">
              <div className="flex items-center justify-center w-8 h-8 rounded-full bg-red-600 text-white text-sm font-bold shrink-0 mt-0.5">
                {step.id}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 mb-1">
                  <step.icon size={16} className="text-red-400 shrink-0" />
                  <h4 className="text-sm font-semibold text-gray-100">{step.name}</h4>
                  {step.status === 'success' && <CheckCircle size={14} className="text-green-500 shrink-0" />}
                  {step.status === 'error' && <XCircle size={14} className="text-red-500 shrink-0" />}
                  {step.status === 'loading' && <Loader2 size={14} className="text-blue-400 animate-spin shrink-0" />}
                </div>
                <p className="text-xs text-gray-400 mb-1.5">{step.description}</p>
                <code className="text-[10px] text-gray-500 font-mono bg-[#0b0b10] px-1.5 py-0.5 rounded">{step.endpoint}</code>
              </div>
            </div>

            {/* Right: API response */}
            <div className="p-4 flex flex-col min-w-0">
              {step.status === 'idle' && (
                <div className="flex-1 flex items-center justify-center">
                  <span className="text-gray-600 text-xs font-mono">// awaiting execution</span>
                </div>
              )}
              {step.status === 'loading' && (
                <div className="flex-1 flex items-center justify-center gap-2 text-blue-400 text-sm">
                  <Loader2 size={16} className="animate-spin" /> Executing…
                </div>
              )}
              {step.status === 'success' && (
                <pre className="bg-[#0b0b10] p-3 rounded font-mono text-[11px] text-green-300 overflow-x-auto overflow-y-auto max-h-36 border border-green-900/30 flex-1">
                  {JSON.stringify(step.response, null, 2)}
                </pre>
              )}
              {step.status === 'error' && (
                <div className="flex-1 flex items-center gap-2 text-red-400 text-sm">
                  <XCircle size={14} /> {step.error}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

/* ================================================================
   API PLAYGROUND COMPONENT
   ================================================================ */

const ApiPlayground: React.FC = () => {
  const [selectedEndpoint, setSelectedEndpoint] = useState<EndpointConfig | null>(null);
  const [requestBody, setRequestBody] = useState<string>('');
  const [pathParams, setPathParams] = useState<Record<string, string | number>>({});
  const [queryParams, setQueryParams] = useState<Record<string, string | number>>({});
  const [response, setResponse] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (selectedEndpoint) {
      setRequestBody(selectedEndpoint.exampleBody || '');
      const initialPathParams: Record<string, string | number> = {};
      const initialQueryParams: Record<string, string | number> = {};
      selectedEndpoint.params?.forEach(p => {
        if (selectedEndpoint.path.includes(`{${p.name}}`)) {
          initialPathParams[p.name] = p.default !== undefined ? p.default : (p.type === 'number' ? 0 : '');
        } else {
          initialQueryParams[p.name] = p.default !== undefined ? p.default : (p.type === 'number' ? 0 : '');
        }
      });
      setPathParams(initialPathParams);
      setQueryParams(initialQueryParams);
      setResponse(null);
      setError(null);
    }
  }, [selectedEndpoint]);

  const handleRunRequest = useCallback(async () => {
    if (!selectedEndpoint) { setError('Please select an endpoint.'); return; }
    setLoading(true); setResponse(null); setError(null);
    try {
      const callParams: Record<string, any> = {};
      for (const k in pathParams) callParams[k] = pathParams[k];
      for (const k in queryParams) if (queryParams[k] !== '' && queryParams[k] !== null) callParams[k] = queryParams[k];

      let bodyData: any;
      if (selectedEndpoint.method === 'POST' && requestBody) {
        try { bodyData = JSON.parse(requestBody); } catch { throw new Error('Invalid JSON in request body.'); }
      }

      let apiCallPromise: Promise<api.ApiResponse<any>>;
      if (selectedEndpoint.name === 'GET /proofs') { apiCallPromise = api.getProofs(callParams.agent, callParams.page, callParams.limit); }
      else if (selectedEndpoint.name === 'GET /proofs/{id}') { apiCallPromise = api.getProofById(callParams.id); }
      else if (selectedEndpoint.name === 'POST /proofs/{id}/revoke') { apiCallPromise = api.revokeProof(callParams.id); }
      else if (selectedEndpoint.name === 'GET /kyc/whitelist/{user}') { apiCallPromise = api.getKycWhitelist(callParams.user); }
      else if (selectedEndpoint.name === 'GET /inference/model/{id}') { apiCallPromise = api.getModelById(callParams.id); }
      else if (selectedEndpoint.name === 'GET /aggregation/batch/{id}') { apiCallPromise = api.getAggregationBatchById(callParams.id); }
      else if (selectedEndpoint.name === 'GET /zk/challenge/{id}') { apiCallPromise = api.getZKChallengeById(callParams.id); }
      else if (selectedEndpoint.name === 'GET /aggregation/verify-batch/{id}') { apiCallPromise = api.verifyAggregationBatch(callParams.id); }
      else if (selectedEndpoint.name === 'GET /proofs/{id}/export') { apiCallPromise = api.exportProof(callParams.id); }
      else if (selectedEndpoint.method === 'POST') { apiCallPromise = selectedEndpoint.apiCall(bodyData); }
      else { apiCallPromise = selectedEndpoint.apiCall(callParams); }

      const res = await apiCallPromise;
      if (res.success) { setResponse(res.data); toast.success('API call successful!'); }
      else { setError(res.error || 'API call failed.'); toast.error(res.error || 'API call failed.'); }
    } catch (err: any) {
      setError(err.message || 'An unexpected error occurred.');
      toast.error(err.message || 'An unexpected error occurred.');
    } finally { setLoading(false); }
  }, [selectedEndpoint, requestBody, pathParams, queryParams]);

  const formattedResponse = useMemo(() => {
    if (response === null) return '';
    try { return JSON.stringify(response, null, 2); } catch { return String(response); }
  }, [response]);

  return (
    <div>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Request Panel */}
        <div className="bg-[#1a1a2a] p-5 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-lg font-semibold text-gray-100 mb-4 flex items-center gap-2">
            <Code size={20} className="text-red-500" /> Request
          </h3>
          <div className="mb-4">
            <label htmlFor="endpoint-select" className="block text-sm font-medium text-gray-300 mb-1">Select Endpoint</label>
            <select id="endpoint-select" onChange={(e) => setSelectedEndpoint(playgroundEndpoints.find(ep => ep.name === e.target.value) || null)}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500 text-sm"
              value={selectedEndpoint?.name || ''}>
              <option value="">-- Select an API Endpoint --</option>
              {playgroundEndpoints.map((ep) => (<option key={ep.name} value={ep.name}>{ep.name}</option>))}
            </select>
          </div>
          {selectedEndpoint && (
            <>
              <p className="text-gray-500 text-xs mb-3">{selectedEndpoint.description}</p>
              {/* Path params */}
              {Object.keys(pathParams).length > 0 && (
                <div className="mb-3">
                  <h4 className="text-sm font-medium text-gray-300 mb-1.5">Path Parameters</h4>
                  {selectedEndpoint.params?.filter(p => selectedEndpoint.path.includes(`{${p.name}}`)).map(param => (
                    <div key={param.name} className="mb-2">
                      <label className="block text-xs font-medium text-gray-400 mb-0.5">{param.name} {param.optional ? '(Opt)' : '(Req)'}</label>
                      <input type={param.type === 'number' ? 'number' : 'text'} value={pathParams[param.name]}
                        onChange={(e) => setPathParams(prev => ({ ...prev, [param.name]: e.target.value }))}
                        className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono text-sm" />
                    </div>
                  ))}
                </div>
              )}
              {/* Query params */}
              {(selectedEndpoint.params?.filter(p => !selectedEndpoint.path.includes(`{${p.name}}`))?.length ?? 0) > 0 && (
                <div className="mb-3">
                  <h4 className="text-sm font-medium text-gray-300 mb-1.5">Query Parameters</h4>
                  {selectedEndpoint.params?.filter(p => !selectedEndpoint.path.includes(`{${p.name}}`)).map(param => (
                    <div key={param.name} className="mb-2">
                      <label className="block text-xs font-medium text-gray-400 mb-0.5">{param.name} {param.optional ? '(Opt)' : '(Req)'}</label>
                      <input type={param.type === 'number' ? 'number' : 'text'} value={queryParams[param.name]}
                        onChange={(e) => setQueryParams(prev => ({ ...prev, [param.name]: e.target.value }))}
                        className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono text-sm" />
                    </div>
                  ))}
                </div>
              )}
              {/* Body */}
              {selectedEndpoint.method === 'POST' && (
                <div className="mb-3">
                  <label className="block text-sm font-medium text-gray-300 mb-1">Request Body (JSON)</label>
                  <textarea rows={8} value={requestBody} onChange={(e) => setRequestBody(e.target.value)}
                    className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono text-sm" />
                </div>
              )}
              <button onClick={handleRunRequest} disabled={loading}
                className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors disabled:opacity-50 text-sm">
                {loading ? <Loader2 size={16} className="animate-spin" /> : <Play size={16} />}
                {loading ? 'Running…' : 'Run Request'}
              </button>
            </>
          )}
        </div>

        {/* Response Panel */}
        <div className="bg-[#1a1a2a] p-5 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-lg font-semibold text-gray-100 mb-4 flex items-center gap-2">
            <Terminal size={20} className="text-red-500" /> Response
          </h3>
          {loading && (
            <div className="text-center p-8 text-blue-400"><Loader2 className="animate-spin mx-auto mb-4" size={32} /> Fetching…</div>
          )}
          {error && (
            <div className="p-4 bg-red-900/30 text-red-300 border border-red-700 rounded-md flex items-center gap-2 text-sm">
              <AlertTriangle size={16} /> {error}
            </div>
          )}
          {!loading && !error && (
            <pre className="bg-[#0b0b10] p-3 rounded-md font-mono text-sm overflow-x-auto border border-[#222235] min-h-[300px]">
              {formattedResponse || 'No response yet. Run a request to see results.'}
            </pre>
          )}
        </div>
      </div>
    </div>
  );
};

/* ================================================================
   MAIN PLAYGROUND — subtabs: API Playground / Agent Playground
   ================================================================ */

type PlaygroundTab = 'api' | 'agent';

const Playground: React.FC = () => {
  const [activeTab, setActiveTab] = useState<PlaygroundTab>('api');

  return (
    <div className="p-4">
      <SectionIntro
        title="Playground"
        description="Two interactive environments for testing Bot Prove. API Playground lets you call any of the 32 endpoints individually. Agent Playground runs the full proof lifecycle pipeline (6 steps) in sequence, showing each API response side-by-side."
        dataSource="All requests hit the live Bot Prove API (botprove-api-ylsh.onrender.com). Every result is real."
        badge="Live API"
        badgeColor="blue"
      />

      {/* Sub-tabs */}
      <div className="flex items-center gap-1 mb-6 border-b border-[#222235]">
        <button
          onClick={() => setActiveTab('api')}
          className={`px-4 py-2.5 text-sm font-medium transition-colors border-b-2 ${
            activeTab === 'api'
              ? 'text-red-500 border-red-500'
              : 'text-gray-400 border-transparent hover:text-gray-200 hover:border-gray-600'
          }`}
        >
          <span className="flex items-center gap-2"><Code size={16} /> API Playground</span>
        </button>
        <button
          onClick={() => setActiveTab('agent')}
          className={`px-4 py-2.5 text-sm font-medium transition-colors border-b-2 ${
            activeTab === 'agent'
              ? 'text-red-500 border-red-500'
              : 'text-gray-400 border-transparent hover:text-gray-200 hover:border-gray-600'
          }`}
        >
          <span className="flex items-center gap-2"><FlaskConical size={16} /> Agent Playground</span>
        </button>
      </div>

      {activeTab === 'api' && <ApiPlayground />}
      {activeTab === 'agent' && <AgentPlayground />}
    </div>
  );
};

export default Playground;
