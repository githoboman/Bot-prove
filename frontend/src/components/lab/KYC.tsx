import React, { useState } from 'react';
import { UserCheck, UserPlus, List, Loader2, AlertTriangle, Shield } from 'lucide-react';
import {
  checkKycStatus,
  grantKycAccess,
  getKycWhitelist,
  KYCGrantRequest,
} from '../../lib/api';
import { toast } from '../ui/toast';
import SectionIntro from './SectionIntro';

const KYC: React.FC = () => {
  // Check KYC Status State
  const [checkProofId, setCheckProofId] = useState('P-1');
  const [kycStatusResult, setKycStatusResult] = useState<any>(null);
  const [isCheckingKyc, setIsCheckingKyc] = useState(false);
  const [checkKycError, setCheckKycError] = useState<string | null>(null);

  // Grant KYC Access State
  const [grantUserId, setGrantUserId] = useState('alice-agent-01');
  const [grantProofId, setGrantProofId] = useState('P-1');
  const [isGrantingKyc, setIsGrantingKyc] = useState(false);
  const [grantKycError, setGrantKycError] = useState<string | null>(null);
  const [grantResult, setGrantResult] = useState<any>(null);

  // View Whitelist State
  const [whitelistUser, setWhitelistUser] = useState('alice-agent-01');
  const [whitelistResult, setWhitelistResult] = useState<any>(null);
  const [isViewingWhitelist, setIsViewingWhitelist] = useState(false);
  const [whitelistError, setWhitelistError] = useState<string | null>(null);

  const handleCheckKycStatus = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsCheckingKyc(true);
    setKycStatusResult(null);
    setCheckKycError(null);
    try {
      const res = await checkKycStatus({ proof_id: checkProofId });
      if (res.success && res.data) {
        setKycStatusResult(res.data);
        toast.success(`KYC check for proof ${checkProofId} complete.`);
      } else {
        setCheckKycError(res.error || 'Failed to check KYC status.');
        toast.error(res.error || 'Failed to check KYC status.');
      }
    } catch (err) {
      setCheckKycError('An unexpected error occurred.');
      toast.error('An unexpected error occurred.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsCheckingKyc(false);
    }
  };

  const handleGrantKycAccess = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsGrantingKyc(true);
    setGrantKycError(null);
    setGrantResult(null);
    try {
      const request: KYCGrantRequest = { user: grantUserId, proof_id: grantProofId };
      const res = await grantKycAccess(request);
      if (res.success && res.data) {
        setGrantResult(res.data);
        toast.success(`KYC access granted to ${grantUserId}!`);
      } else {
        setGrantKycError(res.error || 'Failed to grant KYC access.');
        toast.error(res.error || 'Failed to grant KYC access.');
      }
    } catch (err) {
      setGrantKycError('An unexpected error occurred.');
      toast.error('An unexpected error occurred.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsGrantingKyc(false);
    }
  };

  const handleViewWhitelist = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsViewingWhitelist(true);
    setWhitelistResult(null);
    setWhitelistError(null);
    try {
      const res = await getKycWhitelist(whitelistUser || 'all');
      if (res.success && res.data) {
        setWhitelistResult(res.data);
        toast.success('KYC whitelist fetched.');
      } else {
        setWhitelistError(res.error || 'Failed to fetch KYC whitelist.');
        toast.error(res.error || 'Failed to fetch KYC whitelist.');
      }
    } catch (err) {
      setWhitelistError('An unexpected error occurred.');
      toast.error('An unexpected error occurred.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsViewingWhitelist(false);
    }
  };

  return (
    <div className="p-4">
      <SectionIntro
        title="KYC Operations"
        description="Privacy-preserving Know Your Customer verification using zero-knowledge proofs. Check if a cryptographic proof passes KYC eligibility, grant access to verified users, and view KYC whitelists — all without exposing the underlying personal data."
        dataSource="Live KYC verification API calls. Proof verification uses the Bot Prove engine's Merkle tree."
        badge="Live verification"
        badgeColor="green"
      />
      <h2 className="text-2xl font-bold text-gray-100 mb-2">KYC Operations Lab</h2>
      <p className="text-gray-400 mb-6">
        Manage and verify Know Your Customer (KYC) statuses for users, ensuring privacy-preserving compliance on BOT Chain.
      </p>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Check KYC Status */}
        <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-xl font-semibold text-gray-100 mb-4 flex items-center gap-2">
            <UserCheck size={24} className="text-red-500" />
            Check KYC Status
          </h3>
          <form onSubmit={handleCheckKycStatus} className="space-y-4">
            <div>
              <label htmlFor="checkProofId" className="block text-sm font-medium text-gray-300 mb-1">
                Proof ID
              </label>
              <input
                type="text"
                id="checkProofId"
                value={checkProofId}
                onChange={(e) => setCheckProofId(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
                placeholder="e.g. P-1"
                required
              />
              <p className="text-xs text-gray-500 mt-1">The proof ID to verify for KYC eligibility (e.g. P-1, P-2)</p>
            </div>
            <button
              type="submit"
              disabled={isCheckingKyc}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isCheckingKyc ? <Loader2 size={20} className="animate-spin" /> : <UserCheck size={20} />}
              {isCheckingKyc ? 'Checking...' : 'Check Status'}
            </button>
          </form>

          {checkKycError && (
            <div className="mt-4 p-3 bg-red-900/30 text-red-300 border border-red-700 rounded-md flex items-center gap-2">
              <AlertTriangle size={20} /> {checkKycError}
            </div>
          )}

          {kycStatusResult && (
            <div className="mt-6 p-4 bg-[#0b0b10] border border-[#222235] rounded-md space-y-2 text-sm">
              <h4 className="text-lg font-semibold text-red-400">KYC Verification Result</h4>
              <p><span className="font-medium text-gray-300">Proof ID:</span> <span className="font-mono">{kycStatusResult.proof_id}</span></p>
              <p><span className="font-medium text-gray-300">Verified:</span> <span className={`font-bold ${kycStatusResult.verified ? 'text-green-400' : 'text-red-400'}`}>{kycStatusResult.verified ? 'YES ✓' : 'NO ✗'}</span></p>
              <p className="text-gray-500 mt-2">
                <Shield size={16} className="inline-block mr-1" />
                This checks whether the cryptographic proof is valid for KYC eligibility.
              </p>
            </div>
          )}
        </div>

        {/* Grant KYC Access */}
        <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-xl font-semibold text-gray-100 mb-4 flex items-center gap-2">
            <UserPlus size={24} className="text-red-500" />
            Grant KYC Access
          </h3>
          <form onSubmit={handleGrantKycAccess} className="space-y-4">
            <div>
              <label htmlFor="grantUserId" className="block text-sm font-medium text-gray-300 mb-1">
                User ID
              </label>
              <input
                type="text"
                id="grantUserId"
                value={grantUserId}
                onChange={(e) => setGrantUserId(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
                placeholder="e.g. alice-agent-01"
                required
              />
            </div>
            <div>
              <label htmlFor="grantProofId" className="block text-sm font-medium text-gray-300 mb-1">
                Proof ID
              </label>
              <input
                type="text"
                id="grantProofId"
                value={grantProofId}
                onChange={(e) => setGrantProofId(e.target.value)}
                className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
                placeholder="e.g. P-1"
                required
              />
              <p className="text-xs text-gray-500 mt-1">A valid proof ID that has been verified</p>
            </div>
            <button
              type="submit"
              disabled={isGrantingKyc}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isGrantingKyc ? <Loader2 size={20} className="animate-spin" /> : <UserPlus size={20} />}
              {isGrantingKyc ? 'Granting...' : 'Grant Access'}
            </button>
          </form>

          {grantKycError && (
            <div className="mt-4 p-3 bg-red-900/30 text-red-300 border border-red-700 rounded-md flex items-center gap-2">
              <AlertTriangle size={20} /> {grantKycError}
            </div>
          )}

          {grantResult && (
            <div className="mt-4 p-3 bg-green-900/30 text-green-300 border border-green-700 rounded-md text-sm">
              <p>✓ Access granted for <strong>{grantResult.user}</strong></p>
              <p className="text-xs mt-1 font-mono">Proof: {grantResult.proof_id}</p>
            </div>
          )}
        </div>
      </div>

      {/* View Whitelist */}
      <div className="mt-8 bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
        <h3 className="text-xl font-semibold text-gray-100 mb-4 flex items-center gap-2">
          <List size={24} className="text-red-500" />
          View KYC Whitelist
        </h3>
        <form onSubmit={handleViewWhitelist} className="space-y-4">
          <div>
            <label htmlFor="whitelistUser" className="block text-sm font-medium text-gray-300 mb-1">
              User ID
            </label>
            <input
              type="text"
              id="whitelistUser"
              value={whitelistUser}
              onChange={(e) => setWhitelistUser(e.target.value)}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
              placeholder="Enter User ID to check"
            />
          </div>
          <button
            type="submit"
            disabled={isViewingWhitelist}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isViewingWhitelist ? <Loader2 size={20} className="animate-spin" /> : <List size={20} />}
            {isViewingWhitelist ? 'Loading...' : 'View Whitelist'}
          </button>
        </form>

        {whitelistError && (
          <div className="mt-4 p-3 bg-red-900/30 text-red-300 border border-red-700 rounded-md flex items-center gap-2">
            <AlertTriangle size={20} /> {whitelistError}
          </div>
        )}

        {whitelistResult && (
          <div className="mt-6 p-4 bg-[#0b0b10] border border-[#222235] rounded-md space-y-2 text-sm">
            <h4 className="text-lg font-semibold text-red-400">Whitelist Status</h4>
            <p>
              <span className="font-medium text-gray-300">User:</span>{' '}
              <span className="font-mono">{whitelistResult.user}</span>
            </p>
            <p>
              <span className="font-medium text-gray-300">Whitelisted:</span>{' '}
              <span className={`font-bold ${whitelistResult.whitelisted ? 'text-green-400' : 'text-red-400'}`}>
                {whitelistResult.whitelisted ? 'YES ✓' : 'NO ✗'}
              </span>
            </p>
            <p className="text-gray-500 mt-2">
              <Shield size={16} className="inline-block mr-1" />
              Only whitelist status is revealed. No personal data is exposed.
            </p>
          </div>
        )}
      </div>
    </div>
  );
};

export default KYC;
