import React, { useState } from 'react';
import { KeyRound, ShieldCheck, Loader2, AlertTriangle, Fingerprint, GitFork } from 'lucide-react';
import {
  signSphincs,
  verifySphincs,
  hybridSign,
  hybridVerify,
  PQSignRequest,
  PQVerifyRequest,
  PQHybridSignRequest,
  PQHybridVerifyRequest,
} from '../../lib/api';
import { toast } from '../ui/toast';
import SectionIntro from './SectionIntro';

const PQCrypto: React.FC = () => {
  // SPHINCS+ State
  const [sphincsMessage, setSphincsMessage] = useState('Hello, post-quantum world!');
  const [sphincsSignature, setSphincsSignature] = useState('');
  const [sphincsPublicKey, setSphincsPublicKey] = useState('');
  const [isSigningSphincs, setIsSigningSphincs] = useState(false);
  const [isVerifyingSphincs, setIsVerifyingSphincs] = useState(false);
  const [sphincsVerifyResult, setSphincsVerifyResult] = useState<boolean | null>(null);

  // Hybrid State
  const [hybridMessage, setHybridMessage] = useState('Verify my AI agent decision');
  const [hybridClassicalSignature, setHybridClassicalSignature] = useState('');
  const [hybridPQSignature, setHybridPQSignature] = useState('');
  const [hybridClassicalPublicKey, setHybridClassicalPublicKey] = useState('');
  const [hybridPQPublicKey, setHybridPQPublicKey] = useState('');
  const [isSigningHybrid, setIsSigningHybrid] = useState(false);
  const [isVerifyingHybrid, setIsVerifyingHybrid] = useState(false);
  const [hybridVerifyResult, setHybridVerifyResult] = useState<boolean | null>(null);

  const handleSphincsSign = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSigningSphincs(true);
    setSphincsSignature('');
    setSphincsPublicKey('');
    try {
      const request: PQSignRequest = { message: sphincsMessage };
      const res = await signSphincs(request);
      if (res.success && res.data) {
        setSphincsSignature(res.data.signature);
        setSphincsPublicKey(res.data.publicKey);
        toast.success('SPHINCS+ message signed successfully!');
      } else {
        toast.error(res.error || 'Failed to sign message with SPHINCS+');
      }
    } catch (err) {
      toast.error('An unexpected error occurred during SPHINCS+ signing.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsSigningSphincs(false);
    }
  };

  const handleSphincsVerify = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsVerifyingSphincs(true);
    setSphincsVerifyResult(null);
    try {
      const request: PQVerifyRequest = {
        message: sphincsMessage,
        signature: sphincsSignature,
        public_key: sphincsPublicKey,
      };
      const res = await verifySphincs(request);
      if (res.success && res.data) {
        setSphincsVerifyResult(res.data.valid);
        toast.success(`SPHINCS+ verification result: ${res.data.valid ? 'Valid' : 'Invalid'}`);
      } else {
        toast.error(res.error || 'Failed to verify SPHINCS+ signature');
      }
    } catch (err) {
      toast.error('An unexpected error occurred during SPHINCS+ verification.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsVerifyingSphincs(false);
    }
  };

  const handleHybridSign = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSigningHybrid(true);
    setHybridClassicalSignature('');
    setHybridPQSignature('');
    setHybridClassicalPublicKey('');
    setHybridPQPublicKey('');
    try {
      const request: PQHybridSignRequest = { message: hybridMessage };
      const res = await hybridSign(request);
      if (res.success && res.data) {
        setHybridClassicalSignature(res.data.signature);
        setHybridPQSignature(res.data.signature);
        setHybridClassicalPublicKey(res.data.classic_public_key);
        setHybridPQPublicKey(res.data.pq_public_key);
        toast.success('Hybrid message signed successfully!');
      } else {
        toast.error(res.error || 'Failed to sign message with Hybrid crypto');
      }
    } catch (err) {
      toast.error('An unexpected error occurred during Hybrid signing.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsSigningHybrid(false);
    }
  };

  const handleHybridVerify = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsVerifyingHybrid(true);
    setHybridVerifyResult(null);
    try {
      const request: PQHybridVerifyRequest = {
        message: hybridMessage,
        signature: hybridClassicalSignature,
        
        classic_public_key: hybridClassicalPublicKey,
        pq_public_key: hybridPQPublicKey,
      };
      const res = await hybridVerify(request);
      if (res.success && res.data) {
        setHybridVerifyResult(res.data.valid);
        toast.success(`Hybrid verification result: ${res.data.valid ? 'Valid' : 'Invalid'}`);
      } else {
        toast.error(res.error || 'Failed to verify Hybrid signature');
      }
    } catch (err) {
      toast.error('An unexpected error occurred during Hybrid verification.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsVerifyingHybrid(false);
    }
  };

  const renderResult = (isValid: boolean | null) => {
    if (isValid === null) return null;
    return (
      <div className={`mt-4 p-3 rounded-md flex items-center gap-2 ${isValid ? 'bg-green-900/30 text-green-300 border border-green-700' : 'bg-red-900/30 text-red-300 border border-red-700'}`}>
        {isValid ? <ShieldCheck size={20} /> : <AlertTriangle size={20} />}
        Verification Result: <span className="font-bold">{isValid ? 'VALID' : 'INVALID'}</span>
      </div>
    );
  };

  return (
    <div className="p-4">
      <SectionIntro
        title="Post-Quantum Cryptography"
        description="Test real post-quantum digital signature schemes: ML-DSA-65 (FIPS 204, formerly Dilithium) and Lamport One-Time Signatures. ML-DSA-65 provides quantum-resistant signing for production use. Lamport OTS is honestly labeled as educational — a hash-based scheme that demonstrates PQ principles."
        dataSource="Real cryptographic operations computed on the Bot Prove backend. ML-DSA-65 uses the official FIPS 204 implementation."
        badge="Real PQ signatures"
        badgeColor="green"
      />
      <h2 className="text-2xl font-bold text-gray-100 mb-6">Post-Quantum Cryptography Lab</h2>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
        {/* SPHINCS+ Operations */}
        <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-xl font-semibold text-gray-100 mb-4 flex items-center gap-2">
            <Fingerprint size={24} className="text-red-500" />
            Hash-based OTS (Lamport)
          </h3>
          <p className="text-gray-400 mb-4">
            Lamport one-time signatures (Lamport 1979) — the classic hash-based OTS construction that
            SPHINCS+'s WOTS+ builds on. Real, self-consistent crypto; one-time use per key pair.
            Occupies the &quot;SPHINCS+ family&quot; slot until a maintained Go SLH-DSA implementation ships.
          </p>

          <form onSubmit={handleSphincsSign} className="space-y-4 mb-6">
            <div>
              <label htmlFor="sphincsMessage" className="block text-sm font-medium text-gray-300 mb-1">
                Message to Sign
              </label>
              <textarea
                id="sphincsMessage"
                rows={3}
                value={sphincsMessage}
                onChange={(e) => setSphincsMessage(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
                required
              ></textarea>
            </div>
            <button
              type="submit"
              disabled={isSigningSphincs}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSigningSphincs ? <Loader2 size={20} className="animate-spin" /> : <KeyRound size={20} />}
              {isSigningSphincs ? 'Signing...' : 'Sign Message (SPHINCS+)'}
            </button>
          </form>

          {sphincsSignature && (
            <div className="bg-[#0b0b10] p-4 rounded-md border border-[#222235] mb-6 space-y-2 text-sm">
              <h4 className="text-lg font-medium text-gray-300">Generated Signature:</h4>
              <p><span className="font-medium text-gray-300">Public Key:</span> <span className="font-mono break-all">{sphincsPublicKey}</span></p>
              <p><span className="font-medium text-gray-300">Signature:</span> <span className="font-mono break-all">{sphincsSignature}</span></p>
            </div>
          )}

          <form onSubmit={handleSphincsVerify} className="space-y-4">
            <div>
              <label htmlFor="sphincsVerifySignature" className="block text-sm font-medium text-gray-300 mb-1">
                Signature to Verify
              </label>
              <textarea
                id="sphincsVerifySignature"
                rows={3}
                value={sphincsSignature}
                onChange={(e) => setSphincsSignature(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
                required
              ></textarea>
            </div>
            <div>
              <label htmlFor="sphincsVerifyPublicKey" className="block text-sm font-medium text-gray-300 mb-1">
                Public Key
              </label>
              <input
                type="text"
                id="sphincsVerifyPublicKey"
                value={sphincsPublicKey}
                onChange={(e) => setSphincsPublicKey(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
                required
              />
            </div>
            <button
              type="submit"
              disabled={isVerifyingSphincs}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isVerifyingSphincs ? <Loader2 size={20} className="animate-spin" /> : <ShieldCheck size={20} />}
              {isVerifyingSphincs ? 'Verifying...' : 'Verify Signature (SPHINCS+)'}
            </button>
          </form>
          {renderResult(sphincsVerifyResult)}
        </div>

        {/* Hybrid Operations */}
        <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-xl font-semibold text-gray-100 mb-4 flex items-center gap-2">
            <GitFork size={24} className="text-red-500" />
            Hybrid Signature Scheme
          </h3>
          <p className="text-gray-400 mb-4">
            Combines classical (e.g., ECDSA) and post-quantum (e.g., SPHINCS+) signatures for transitional security.
          </p>

          <form onSubmit={handleHybridSign} className="space-y-4 mb-6">
            <div>
              <label htmlFor="hybridMessage" className="block text-sm font-medium text-gray-300 mb-1">
                Message to Sign
              </label>
              <textarea
                id="hybridMessage"
                rows={3}
                value={hybridMessage}
                onChange={(e) => setHybridMessage(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
                required
              ></textarea>
            </div>
            <button
              type="submit"
              disabled={isSigningHybrid}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSigningHybrid ? <Loader2 size={20} className="animate-spin" /> : <KeyRound size={20} />}
              {isSigningHybrid ? 'Signing...' : 'Sign Message (Hybrid)'}
            </button>
          </form>

          {hybridClassicalSignature && (
            <div className="bg-[#0b0b10] p-4 rounded-md border border-[#222235] mb-6 space-y-2 text-sm">
              <h4 className="text-lg font-medium text-gray-300">Generated Hybrid Signature:</h4>
              <p><span className="font-medium text-gray-300">Classical Public Key:</span> <span className="font-mono break-all">{hybridClassicalPublicKey}</span></p>
              <p><span className="font-medium text-gray-300">PQ Public Key:</span> <span className="font-mono break-all">{hybridPQPublicKey}</span></p>
              <p><span className="font-medium text-gray-300">Classical Signature:</span> <span className="font-mono break-all">{hybridClassicalSignature}</span></p>
              <p><span className="font-medium text-gray-300">PQ Signature:</span> <span className="font-mono break-all">{hybridPQSignature}</span></p>
            </div>
          )}

          <form onSubmit={handleHybridVerify} className="space-y-4">
            <div>
              <label htmlFor="hybridVerifyClassicalSignature" className="block text-sm font-medium text-gray-300 mb-1">
                Classical Signature
              </label>
              <textarea
                id="hybridVerifyClassicalSignature"
                rows={2}
                value={hybridClassicalSignature}
                onChange={(e) => setHybridClassicalSignature(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
                required
              ></textarea>
            </div>
            <div>
              <label htmlFor="hybridVerifyPQSignature" className="block text-sm font-medium text-gray-300 mb-1">
                PQ Signature
              </label>
              <textarea
                id="hybridVerifyPQSignature"
                rows={2}
                value={hybridPQSignature}
                onChange={(e) => setHybridPQSignature(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
                required
              ></textarea>
            </div>
            <div>
              <label htmlFor="hybridVerifyClassicalPublicKey" className="block text-sm font-medium text-gray-300 mb-1">
                Classical Public Key
              </label>
              <input
                type="text"
                id="hybridVerifyClassicalPublicKey"
                value={hybridClassicalPublicKey}
                onChange={(e) => setHybridClassicalPublicKey(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
                required
              />
            </div>
            <div>
              <label htmlFor="hybridVerifyPQPublicKey" className="block text-sm font-medium text-gray-300 mb-1">
                PQ Public Key
              </label>
              <input
                type="text"
                id="hybridVerifyPQPublicKey"
                value={hybridPQPublicKey}
                onChange={(e) => setHybridPQPublicKey(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
                required
              />
            </div>
            <button
              type="submit"
              disabled={isVerifyingHybrid}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isVerifyingHybrid ? <Loader2 size={20} className="animate-spin" /> : <ShieldCheck size={20} />}
              {isVerifyingHybrid ? 'Verifying...' : 'Verify Signature (Hybrid)'}
            </button>
          </form>
          {renderResult(hybridVerifyResult)}
        </div>
      </div>

      {/* Comparison Section */}
      <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md mt-8">
        <h3 className="text-xl font-semibold text-gray-100 mb-4">Classical vs. Post-Quantum Security</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6 text-gray-300">
          <div>
            <h4 className="text-lg font-medium text-red-400 mb-2">Classical Cryptography (e.g., RSA, ECDSA)</h4>
            <ul className="list-disc list-inside space-y-1 text-gray-400">
              <li>Relies on mathematical problems hard for classical computers (e.g., factoring large numbers, discrete logarithms).</li>
              <li>Efficient and widely adopted today.</li>
              <li>Vulnerable to attacks by sufficiently powerful quantum computers (e.g., Shor's algorithm, Grover's algorithm).</li>
              <li>Examples: Digital signatures for most blockchain transactions, TLS/SSL.</li>
            </ul>
          </div>
          <div>
            <h4 className="text-lg font-medium text-red-400 mb-2">Post-Quantum Cryptography (PQC)</h4>
            <ul className="list-disc list-inside space-y-1 text-gray-400">
              <li>Designed to be secure against both classical and quantum computers.</li>
              <li>Often based on different mathematical problems (e.g., lattice-based, hash-based, code-based).</li>
              <li>Generally larger key/signature sizes and slower performance compared to classical schemes.</li>
              <li>Examples: SPHINCS+, CRYSTALS-Dilithium, Falcon.</li>
            </ul>
          </div>
        </div>
        <p className="text-gray-400 mt-6 text-sm">
          Hybrid schemes combine both classical and PQC algorithms to provide a transitional solution, ensuring security against both current and future quantum threats.
        </p>
      </div>
    </div>
  );
};

export default PQCrypto;
