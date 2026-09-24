import React, { useState } from 'react';
import { ShieldCheck, Loader2, AlertTriangle, KeyRound, Search, Zap, List, X } from 'lucide-react';
import {
  zkGroth16RealProve,
  zkGroth16RealVerify,
  verifyGroth16,
  batchVerifyZK,
  challengeZK,
  getZKChallengeById,
} from '../../lib/api';
import { toast } from '../ui/toast';
import SectionIntro from './SectionIntro';
import TrustStatus from '../TrustStatus';

const Modal: React.FC<{
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
}> = ({ isOpen, onClose, title, children }) => {
  if (!isOpen) return null;
  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4">
      <div className="bg-[#1a1a2a] rounded-lg border border-[#222235] max-w-lg w-full max-h-[80vh] overflow-y-auto p-6">
        <div className="flex justify-between items-center mb-4">
          <h3 className="text-lg font-semibold text-gray-100">{title}</h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-100"><X size={18} /></button>
        </div>
        {children}
      </div>
    </div>
  );
};

const ZKProofs: React.FC = () => {
  // Groth16 Real Prove
  const [preimage, setPreimage] = useState('42');
  const [proveResult, setProveResult] = useState<any>(null);
  const [proveLoading, setProveLoading] = useState(false);

  // Groth16 Real Verify
  const [verifyHash, setVerifyHash] = useState('');
  const [verifyProofHex, setVerifyProofHex] = useState('');
  const [verifyResult, setVerifyResult] = useState<any>(null);
  const [verifyLoading, setVerifyLoading] = useState(false);

  // Conceptual Groth16
  const [conceptualProof, setConceptualProof] = useState('');
  const [conceptualResult, setConceptualResult] = useState<any>(null);
  const [conceptualLoading, setConceptualLoading] = useState(false);

  // Challenge
  const [challengeProofId, setChallengeProofId] = useState('');
  const [challengeReason, setChallengeReason] = useState('');
  const [challengeResult, setChallengeResult] = useState<any>(null);
  const [challengeLoading, setChallengeLoading] = useState(false);

  // Get Challenge
  const [lookupChallengeId, setLookupChallengeId] = useState('');
  const [lookupResult, setLookupResult] = useState<any>(null);
  const [lookupLoading, setLookupLoading] = useState(false);

  // Modals
  const [showProveModal, setShowProveModal] = useState(false);
  const [showVerifyModal, setShowVerifyModal] = useState(false);

  const handleGroth16Prove = async () => {
    setProveLoading(true);
    setProveResult(null);
    try {
      const res = await zkGroth16RealProve({ preimage });
      if (res.success) {
        setProveResult(res.data);
        toast.success('Groth16 proof generated!');
        // Auto-fill verify fields
        if (res.data?.hash) setVerifyHash(res.data.hash);
        if (res.data?.proof_hex) setVerifyProofHex(res.data.proof_hex);
      } else {
        toast.error(res.error || 'Prove failed');
        setProveResult({ error: res.error });
      }
    } catch (e: any) {
      toast.error(e.message);
      setProveResult({ error: e.message });
    } finally {
      setProveLoading(false);
    }
  };

  const handleGroth16Verify = async () => {
    setVerifyLoading(true);
    setVerifyResult(null);
    try {
      const res = await zkGroth16RealVerify({ hash: verifyHash, proof_hex: verifyProofHex });
      if (res.success) {
        setVerifyResult(res.data);
        toast.success('Proof verified!');
      } else {
        toast.error(res.error || 'Verification failed');
        setVerifyResult({ error: res.error });
      }
    } catch (e: any) {
      toast.error(e.message);
      setVerifyResult({ error: e.message });
    } finally {
      setVerifyLoading(false);
    }
  };

  const handleConceptualVerify = async () => {
    setConceptualLoading(true);
    setConceptualResult(null);
    try {
      const res = await verifyGroth16({
        proof: conceptualProof || 'demo-proof-hash',
        vk_hash: 'demo-vk',
        public_inputs: ['input1'],
      });
      setConceptualResult(res.data);
      if (res.success) toast.success('Conceptual verification complete');
      else toast.error(res.error || 'Failed');
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      setConceptualLoading(false);
    }
  };

  const handleChallenge = async () => {
    setChallengeLoading(true);
    setChallengeResult(null);
    try {
      const res = await challengeZK({ proof_id: challengeProofId, reason: challengeReason });
      setChallengeResult(res.data);
      if (res.success) toast.success('Challenge created');
      else toast.error(res.error || 'Failed');
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      setChallengeLoading(false);
    }
  };

  const handleLookupChallenge = async () => {
    setLookupLoading(true);
    setLookupResult(null);
    try {
      const res = await getZKChallengeById(lookupChallengeId);
      setLookupResult(res.data);
      if (res.success) toast.success('Challenge found');
      else toast.error(res.error || 'Not found');
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      setLookupLoading(false);
    }
  };

  return (
    <div className="p-4 space-y-6">
      <h2 className="text-2xl font-bold text-gray-100 flex items-center gap-3">
        <ShieldCheck size={28} className="text-red-500" />
        ZK Proofs
      </h2>

      <div className="bg-[#12121a] border border-yellow-500/40 rounded-md px-4 py-3 text-xs text-yellow-200/90 flex items-start gap-2">
        <AlertTriangle size={16} className="text-yellow-500 flex-shrink-0 mt-0.5" />
        <span>
          <strong>Boundary:</strong> Real Groth16 prove &amp; verify runs <strong>off-chain</strong> in the Bot Prove engine (gnark BN254). Only the resulting proof handle can be anchored on-chain. On-chain Casper pairing verification is roadmap, not shipped.
        </span>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Real Groth16 Prove */}
        <div className="bg-[#1a1a2a] rounded-lg border border-[#222235] p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-gray-100 flex items-center gap-2">
              <KeyRound size={20} className="text-red-500" />
              Groth16 Real Prove (BN254)
            </h3>
            <TrustStatus kind="real-cryptography" />
          </div>
          <p className="text-gray-400 text-sm mb-4">
            Generate a real BN254 Groth16 proof of knowledge of a MiMC preimage.
            Uses gnark library with trusted setup, R1CS constraints, and real elliptic curve pairing.
          </p>
          <div className="space-y-3">
            <div>
              <label className="text-sm text-gray-300 block mb-1">Preimage (base-10 integer)</label>
              <input
                type="text"
                value={preimage}
                onChange={(e) => setPreimage(e.target.value)}
                className="w-full bg-[#0b0b10] border border-[#222235] text-gray-100 px-3 py-2 rounded text-sm font-mono"
                placeholder="42"
              />
            </div>
            <button
              onClick={handleGroth16Prove}
              disabled={proveLoading}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 disabled:bg-gray-700 text-white rounded transition-colors text-sm font-medium"
            >
              {proveLoading ? <Loader2 size={16} className="animate-spin" /> : <Zap size={16} />}
              Generate Proof
            </button>
          </div>
          {proveResult && (
            <div className="mt-4">
              <pre className="bg-[#0b0b10] p-3 rounded text-xs text-green-300 overflow-auto max-h-48 font-mono">
                {JSON.stringify(proveResult, null, 2)}
              </pre>
            </div>
          )}
        </div>

        {/* Real Groth16 Verify */}
        <div className="bg-[#1a1a2a] rounded-lg border border-[#222235] p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-gray-100 flex items-center gap-2">
              <ShieldCheck size={20} className="text-red-500" />
              Groth16 Real Verify
            </h3>
            <TrustStatus kind="real-cryptography" />
          </div>
          <p className="text-gray-400 text-sm mb-4">
            Verify a Groth16 proof. Fields auto-populate when you generate a proof on the left.
          </p>
          <div className="space-y-3">
            <div>
              <label className="text-sm text-gray-300 block mb-1">Hash (base-10 integer)</label>
              <input
                type="text"
                value={verifyHash}
                onChange={(e) => setVerifyHash(e.target.value)}
                className="w-full bg-[#0b0b10] border border-[#222235] text-gray-100 px-3 py-2 rounded text-sm font-mono"
                placeholder="Auto-filled from proof..."
              />
            </div>
            <div>
              <label className="text-sm text-gray-300 block mb-1">Proof Hex</label>
              <textarea
                value={verifyProofHex}
                onChange={(e) => setVerifyProofHex(e.target.value)}
                className="w-full bg-[#0b0b10] border border-[#222235] text-gray-100 px-3 py-2 rounded text-sm font-mono h-20"
                placeholder="Auto-filled from proof..."
              />
            </div>
            <button
              onClick={handleGroth16Verify}
              disabled={verifyLoading || !verifyHash || !verifyProofHex}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-700 disabled:bg-gray-700 text-white rounded transition-colors text-sm font-medium"
            >
              {verifyLoading ? <Loader2 size={16} className="animate-spin" /> : <ShieldCheck size={16} />}
              Verify Proof
            </button>
          </div>
          {verifyResult && (
            <div className="mt-4">
              <pre className={`bg-[#0b0b10] p-3 rounded text-xs overflow-auto max-h-48 font-mono ${verifyResult.valid ? 'text-green-300' : 'text-red-300'}`}>
                {JSON.stringify(verifyResult, null, 2)}
              </pre>
            </div>
          )}
        </div>

        {/* Conceptual Groth16 */}
        <div className="bg-[#1a1a2a] rounded-lg border border-[#222235] p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-gray-100 flex items-center gap-2">
              <AlertTriangle size={20} className="text-yellow-500" />
              Groth16 Conceptual (Hash-Based)
            </h3>
            <TrustStatus kind="simulation" />
          </div>
          <p className="text-gray-400 text-sm mb-4">
            Legacy hash-based simulation. Honestly labeled — not real pairing verification.
          </p>
          <div className="space-y-3">
            <input
              type="text"
              value={conceptualProof}
              onChange={(e) => setConceptualProof(e.target.value)}
              className="w-full bg-[#0b0b10] border border-[#222235] text-gray-100 px-3 py-2 rounded text-sm font-mono"
              placeholder="Proof hash (optional)"
            />
            <button
              onClick={handleConceptualVerify}
              disabled={conceptualLoading}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-yellow-600 hover:bg-yellow-700 disabled:bg-gray-700 text-white rounded transition-colors text-sm font-medium"
            >
              {conceptualLoading ? <Loader2 size={16} className="animate-spin" /> : <ShieldCheck size={16} />}
              Verify (Conceptual)
            </button>
          </div>
          {conceptualResult && (
            <div className="mt-4">
              <pre className="bg-[#0b0b10] p-3 rounded text-xs text-yellow-300 overflow-auto max-h-48 font-mono">
                {JSON.stringify(conceptualResult, null, 2)}
              </pre>
            </div>
          )}
        </div>

        {/* ZK Challenge */}
        <div className="bg-[#1a1a2a] rounded-lg border border-[#222235] p-6">
          <h3 className="text-lg font-semibold text-gray-100 flex items-center gap-2 mb-4">
            <AlertTriangle size={20} className="text-red-500" />
            ZK Challenge (Dispute)
          </h3>
          <p className="text-gray-400 text-sm mb-4">
            Open a dispute challenge against a proof. The challenge enters a 48-hour dispute window.
          </p>
          <div className="space-y-3">
            <input
              type="text"
              value={challengeProofId}
              onChange={(e) => setChallengeProofId(e.target.value)}
              className="w-full bg-[#0b0b10] border border-[#222235] text-gray-100 px-3 py-2 rounded text-sm font-mono"
              placeholder="Proof ID to challenge"
            />
            <input
              type="text"
              value={challengeReason}
              onChange={(e) => setChallengeReason(e.target.value)}
              className="w-full bg-[#0b0b10] border border-[#222235] text-gray-100 px-3 py-2 rounded text-sm font-mono"
              placeholder="Reason for challenge"
            />
            <button
              onClick={handleChallenge}
              disabled={challengeLoading || !challengeProofId || !challengeReason}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 disabled:bg-gray-700 text-white rounded transition-colors text-sm font-medium"
            >
              {challengeLoading ? <Loader2 size={16} className="animate-spin" /> : <AlertTriangle size={16} />}
              Open Challenge
            </button>
          </div>
          {challengeResult && (
            <div className="mt-4">
              <pre className="bg-[#0b0b10] p-3 rounded text-xs text-red-300 overflow-auto max-h-48 font-mono">
                {JSON.stringify(challengeResult, null, 2)}
              </pre>
            </div>
          )}
          <div className="mt-4 border-t border-[#222235] pt-4">
            <h4 className="text-sm font-medium text-gray-300 mb-2">Look up Challenge</h4>
            <div className="flex gap-2">
              <input
                type="text"
                value={lookupChallengeId}
                onChange={(e) => setLookupChallengeId(e.target.value)}
                className="flex-1 bg-[#0b0b10] border border-[#222235] text-gray-100 px-3 py-2 rounded text-sm font-mono"
                placeholder="Challenge ID"
              />
              <button
                onClick={handleLookupChallenge}
                disabled={lookupLoading || !lookupChallengeId}
                className="flex items-center gap-1 px-3 py-2 bg-[#222235] hover:bg-[#2a2a3a] disabled:bg-gray-800 text-gray-100 rounded text-sm"
              >
                {lookupLoading ? <Loader2 size={14} className="animate-spin" /> : <Search size={14} />}
                Lookup
              </button>
            </div>
            {lookupResult && (
              <pre className="mt-2 bg-[#0b0b10] p-3 rounded text-xs text-gray-300 overflow-auto max-h-32 font-mono">
                {JSON.stringify(lookupResult, null, 2)}
              </pre>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default ZKProofs;
