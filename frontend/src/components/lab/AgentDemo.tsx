import React, { useState, useCallback, useRef } from 'react';
import {
  Play,
  CheckCircle,
  XCircle,
  Loader2,
  Box,
  FlaskConical,
  ShieldCheck,
  GitMerge,
  ArrowRight,
  PlusCircle,
  AlertTriangle,
} from 'lucide-react';
import {
  registerModel,
  inferenceProve,
  inferenceVerify,
  createAggregationBatch,
  addProofToAggregationBatch,
  finalizeAggregationBatch,
} from '../../lib/api';
import { toast } from '../ui/toast';
import SectionIntro from './SectionIntro';
import { getCachedManifest } from '../../lib/onchain';

// Fallback verifier_gate hash (used only if /onchain.json is not yet loaded
// when this demo step runs). See lib/onchain.ts / Gate 1.5 canonical manifest.
const VERIFIER_GATE_FALLBACK = 'a37f9cde9dbdc5bb8b9e92c663bdc59b83b42c89dc75ec73f7f7cde2619f77d3';
function verifierGateHash(): string {
  return getCachedManifest()?.contracts.verifier_gate?.contract_hash ?? VERIFIER_GATE_FALLBACK;
}

const genId = (prefix: string) => `${prefix}-${Math.random().toString(36).substring(2, 10)}`;

interface Step {
  id: number;
  name: string;
  icon: React.ElementType;
  description: string;
  action: () => Promise<any>;
  status: 'idle' | 'loading' | 'success' | 'error';
  response: any;
  error: string | null;
}

const AgentDemo: React.FC = () => {
  const [currentStepIndex, setCurrentStepIndex] = useState(-1);
  const [isDemoRunning, setIsDemoRunning] = useState(false);
  const [globalError, setGlobalError] = useState<string | null>(null);

  const [modelId, setModelIdState] = useState('');
  const [proofId, setProofIdState] = useState('');
  const [batchId, setBatchIdState] = useState('');
  const [proofHash, setProofHashState] = useState('');

  // Refs prevent stale closures — step actions always read the latest value.
  const modelIdRef = useRef('');
  const proofIdRef = useRef('');
  const batchIdRef = useRef('');
  const proofHashRef = useRef('');

  const setModelId = (v: string) => { modelIdRef.current = v; setModelIdState(v); };
  const setProofId = (v: string) => { proofIdRef.current = v; setProofIdState(v); };
  const setBatchId = (v: string) => { batchIdRef.current = v; setBatchIdState(v); };
  const setProofHash = (v: string) => { proofHashRef.current = v; setProofHashState(v); };

  const [agentId] = useState(genId('agent'));
  const inputData = JSON.stringify({ temperature: 0.7, prompt: 'Generate a secure proof for AI decision.' });
  const outputData = JSON.stringify({ decision: 'approved', confidence: 0.95, risk_score: 12 });

  const initialSteps: Step[] = [
    {
      id: 1,
      name: 'Register AI Model',
      icon: Box,
      description: 'Register a new AI model on the Bot Prove registry.',
      action: async () => {
        const mid = genId('model');
        const res = await registerModel({
          model_id: mid,
          model_hash: genId('hash'),
          verifier_contract: verifierGateHash(),
          metadata: { type: 'decision-box', version: '1.0' },
        });
        if (res.success && res.data) {
          const id = res.data.model_id || res.data.id || mid;
          setModelId(id);
          return res.data;
        }
        throw new Error(res.error || 'Failed to register model');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 2,
      name: 'Run Inference & Prove',
      icon: FlaskConical,
      description: 'Run an AI inference and generate a cryptographic proof.',
      action: async () => {
        if (!modelIdRef.current) throw new Error('Model ID not available. Complete Step 1 first.');
        const res = await inferenceProve({
          agent: agentId,
          model_id: modelIdRef.current,
          input: inputData,
          output: outputData,
          use_case: 'kyc-eligibility',
        });
        if (res.success && res.data) {
          const pid = res.data.id || '';
          const ph = res.data.proof_hash || '';
          setProofId(pid);
          setProofHash(ph);
          return res.data;
        }
        throw new Error(res.error || 'Failed to generate inference proof');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 3,
      name: 'Verify Proof',
      icon: ShieldCheck,
      description: 'Verify the generated proof on the Bot Prove engine.',
      action: async () => {
        if (!proofIdRef.current) throw new Error('Proof ID not available. Complete Step 2 first.');
        const res = await inferenceVerify({ proof_id: proofIdRef.current });
        if (res.success && res.data) {
          return res.data;
        }
        throw new Error(res.error || 'Failed to verify proof');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 4,
      name: 'Create Proof Batch',
      icon: GitMerge,
      description: 'Create a new batch for aggregating multiple proofs.',
      action: async () => {
        const bid = genId('batch');
        const res = await createAggregationBatch({
          batch_id: bid,
          max_proofs: 50,
        });
        if (res.success && res.data) {
          const id = res.data.batch_id || bid;
          setBatchId(id);
          return res.data;
        }
        throw new Error(res.error || 'Failed to create batch');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 5,
      name: 'Add Proof to Batch',
      icon: PlusCircle,
      description: 'Add the generated proof to the batch.',
      action: async () => {
        if (!batchIdRef.current) throw new Error('Batch ID not available. Complete Step 4 first.');
        if (!proofHashRef.current) throw new Error('Proof hash not available. Complete Step 2 first.');
        const res = await addProofToAggregationBatch({
          batch_id: batchIdRef.current,
          proof_hash: proofHashRef.current,
        });
        if (res.success) return res.data;
        throw new Error(res.error || 'Failed to add proof to batch');
      },
      status: 'idle', response: null, error: null,
    },
    {
      id: 6,
      name: 'Finalize & Aggregate',
      icon: CheckCircle,
      description: 'Finalize the batch, generating an aggregated proof.',
      action: async () => {
        if (!batchIdRef.current) throw new Error('Batch ID not available. Complete Step 4 first.');
        const res = await finalizeAggregationBatch({ batch_id: batchIdRef.current });
        if (res.success) return res.data;
        throw new Error(res.error || 'Failed to finalize batch');
      },
      status: 'idle', response: null, error: null,
    },
  ];

  const [steps, setSteps] = useState<Step[]>(initialSteps);

  const resetDemo = useCallback(() => {
    setSteps(initialSteps);
    setCurrentStepIndex(-1);
    setIsDemoRunning(false);
    setGlobalError(null);
    setModelId('');
    setProofId('');
    setBatchId('');
    setProofHash('');
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
      setCurrentStepIndex(i);
      const success = await runStep(i);
      if (!success) { setIsDemoRunning(false); return; }
      await new Promise((r) => setTimeout(r, 800));
    }
    setIsDemoRunning(false);
    setCurrentStepIndex(steps.length);
    toast.success('Agent Demo completed!');
  }, [steps, runStep, resetDemo]);

  return (
    <div className="p-4">
      <SectionIntro
        title="Proof Pipeline"
        description="End-to-end demonstration of the Bot Prove proof lifecycle: register an AI model → generate an inference proof → verify it cryptographically → aggregate into a batch → finalize. Every step makes real API calls to the live backend. Watch the full pipeline execute in sequence."
        dataSource="Live API calls to all Bot Prove endpoints. Each step creates real data in the engine."
        badge="Live pipeline"
        badgeColor="blue"
      />
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Proof Pipeline</h2>
      <p className="text-gray-400 mb-6">
        Interactive demo: model registration → inference proof → verification → batch aggregation. All calls hit the live API.
      </p>

      <div className="flex items-center gap-4 mb-8">
        <button onClick={runFullDemo} disabled={isDemoRunning}
          className="flex items-center gap-2 px-6 py-3 bg-red-600 hover:bg-red-700 text-white rounded-md text-lg font-medium transition-colors disabled:opacity-50">
          {isDemoRunning ? <Loader2 size={24} className="animate-spin" /> : <Play size={24} />}
          {isDemoRunning ? 'Running...' : 'Start Demo'}
        </button>
        <button onClick={resetDemo} disabled={isDemoRunning}
          className="flex items-center gap-2 px-6 py-3 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-md text-lg font-medium transition-colors disabled:opacity-50">
          <XCircle size={24} /> Reset
        </button>
      </div>

      {globalError && (
        <div className="p-4 bg-red-900/30 text-red-300 border border-red-700 rounded-md mb-6 flex items-center gap-2">
          <AlertTriangle size={20} />
          <span>{globalError}</span>
        </div>
      )}

      {/* Step cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-5 gap-4">
        {steps.map((step, index) => (
          <React.Fragment key={step.id}>
            <div className={`relative flex flex-col items-center p-5 rounded-lg border transition-all duration-500
              ${step.status === 'success' ? 'bg-green-900/20 border-green-600' : ''}
              ${step.status === 'error' ? 'bg-red-900/20 border-red-600' : ''}
              ${step.status === 'loading' ? 'bg-blue-900/20 border-blue-500 animate-pulse' : ''}
              ${step.status === 'idle' ? 'bg-[#1a1a2a] border-[#222235]' : ''}
              w-full min-h-[220px]`}>
              <div className="flex items-center justify-center w-10 h-10 rounded-full bg-red-600 text-white text-lg font-bold mb-3">
                {step.id}
              </div>
              <step.icon size={28} className="text-red-400 mb-2" />
              <h3 className="text-sm font-semibold text-gray-100 mb-1 text-center">{step.name}</h3>
              <p className="text-xs text-gray-400 text-center">{step.description}</p>

              {step.status === 'loading' && (
                <div className="absolute inset-0 flex items-center justify-center bg-black/70 rounded-lg">
                  <Loader2 size={28} className="animate-spin text-blue-400" />
                </div>
              )}
              {step.status === 'success' && (
                <div className="absolute inset-0 flex items-center justify-center bg-black/60 rounded-lg">
                  <CheckCircle size={28} className="text-green-500" />
                </div>
              )}
              {step.status === 'error' && (
                <div className="absolute inset-0 flex items-center justify-center bg-black/60 rounded-lg">
                  <XCircle size={28} className="text-red-500" />
                </div>
              )}
            </div>
            {/* Arrow removed — grid layout handles flow */}
          </React.Fragment>
        ))}
      </div>

      {/* API Responses */}
      <div className="mt-10 space-y-4">
        <h3 className="text-lg font-bold text-gray-100">API Responses</h3>
        {steps.map((step) => (
          <div key={`resp-${step.id}`} className="bg-[#1a1a2a] p-4 rounded-lg border border-[#222235]">
            <h4 className="text-sm font-semibold text-gray-200 mb-2">
              Step {step.id}: {step.name}
            </h4>
            {step.status === 'loading' && <p className="text-blue-400 text-sm flex items-center gap-2"><Loader2 size={14} className="animate-spin" /> Executing...</p>}
            {step.status === 'success' && (
              <>
                <p className="text-green-400 text-sm flex items-center gap-1 mb-2"><CheckCircle size={14} /> Success</p>
                <pre className="bg-[#0b0b10] p-3 rounded font-mono text-xs overflow-x-auto border border-[#222235] max-h-48 overflow-y-auto">
                  {JSON.stringify(step.response, null, 2)}
                </pre>
              </>
            )}
            {step.status === 'error' && (
              <p className="text-red-400 text-sm flex items-center gap-1"><XCircle size={14} /> {step.error}</p>
            )}
            {step.status === 'idle' && <p className="text-gray-500 text-sm">Awaiting execution...</p>}
          </div>
        ))}
      </div>
    </div>
  );
};

export default AgentDemo;
