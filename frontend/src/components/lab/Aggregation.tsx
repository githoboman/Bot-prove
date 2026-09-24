import React, { useState, useEffect, useCallback } from 'react';
import { PlusCircle, List, CheckCircle, Eye, Loader2, AlertTriangle, GitMerge, FileText, XCircle,
} from 'lucide-react';
import {
  createAggregationBatch,
  addProofToAggregationBatch,
  finalizeAggregationBatch,
  getAggregationBatchById,
  verifyAggregationBatch,
  CreateBatchRequest,
  AddProofToBatchRequest,
  FinalizeBatchRequest,
  
} from '../../lib/api';
import { toast } from '../ui/toast';
import SectionIntro from './SectionIntro';

// Placeholder for a generic Modal component
const Modal: React.FC<{
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  className?: string;
}> = ({ isOpen, onClose, title, children, className }) => {
  if (!isOpen) return null;
  return (
    <div className="fixed inset-0 bg-black bg-opacity-75 flex items-center justify-center z-50 p-4">
      <div className={`bg-[#13131d] border border-[#222235] rounded-lg shadow-xl max-w-2xl w-full max-h-[90vh] overflow-y-auto ${className}`}>
        <div className="flex justify-between items-center p-4 border-b border-[#222235]">
          <h2 className="text-xl font-semibold text-gray-100">{title}</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-100">
            <XCircle size={24} />
          </button>
        </div>
        <div className="p-4 text-gray-200">
          {children}
        </div>
      </div>
    </div>
  );
};

const Aggregation: React.FC = () => {
  // Create Batch State
  const [isCreateBatchModalOpen, setIsCreateBatchModalOpen] = useState(false);
  const [newBatchData, setNewBatchData] = useState<CreateBatchRequest>({ batch_id: `batch-${Date.now().toString(36)}`, max_proofs: 100 });
  const [isCreatingBatch, setIsCreatingBatch] = useState(false);
  const [createdBatchId, setCreatedBatchId] = useState<string | null>(null);

  // Add Proof State
  const [isAddProofModalOpen, setIsAddProofModalOpen] = useState(false);
  const [addProofData, setAddProofData] = useState<AddProofToBatchRequest>({ batch_id: '', proof_hash: '' });
  const [isAddingProof, setIsAddingProof] = useState(false);

  // Finalize Batch State
  const [isFinalizeModalOpen, setIsFinalizeModalOpen] = useState(false);
  const [finalizeBatchId, setFinalizeBatchId] = useState('');

  // Verify Batch State
  const [isVerifyBatchModalOpen, setIsVerifyBatchModalOpen] = useState(false);
  const [verifyBatchId, setVerifyBatchId] = useState('');
  const [verifyBatchResult, setVerifyBatchResult] = useState<any>(null);
  const [isVerifyingBatch, setIsVerifyingBatch] = useState(false);
  const [isFinalizing, setIsFinalizing] = useState(false);

  // View Batch State
  const [searchBatchId, setSearchBatchId] = useState('');
  const [foundBatch, setFoundBatch] = useState<any | null>(null);
  const [isSearchingBatch, setIsSearchingBatch] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);

  const handleCreateBatchChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target;
    setNewBatchData((prev) => ({ ...prev, [name]: value }));
  };

  const handleCreateBatchSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsCreatingBatch(true);
    try {
      const res = await createAggregationBatch(newBatchData);
      if (res.success && res.data) {
        toast.success(`Batch "${newBatchData.batch_id}" created successfully! ID: ${res.data.batch_id}`);
        setCreatedBatchId(res.data.batch_id);
        setIsCreateBatchModalOpen(false);
        setNewBatchData({ batch_id: '', max_proofs: 100 });
        setAddProofData((prev) => ({ ...prev, batch_id: res.data!.batch_id })); // Pre-fill for add proof
        setFinalizeBatchId(res.data!.batch_id); // Pre-fill for finalize
      } else {
        toast.error(res.error || 'Failed to create batch');
      }
    } catch (err) {
      toast.error('An unexpected error occurred during batch creation.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsCreatingBatch(false);
    }
  };

  const handleAddProofChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    // Map HTML name attributes to API field names
    const fieldMap: Record<string, string> = { batchId: 'batch_id', proofId: 'proof_hash' };
    const field = fieldMap[name] || name;
    setAddProofData((prev) => ({ ...prev, [field]: value }));
  };

  const handleAddProofSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsAddingProof(true);
    try {
      const res = await addProofToAggregationBatch(addProofData);
      if (res.success) {
        toast.success(`Proof ${addProofData?.proof_hash?.substring(0, 8)}... added to batch ${addProofData?.batch_id?.substring(0, 8)}...`);
        setIsAddProofModalOpen(false);
        setAddProofData((prev) => ({ ...prev, proof_hash: '' })); // Clear proofId only
        // Optionally refresh batch details if currently viewing
        if (foundBatch?.batch_id === addProofData.batch_id) {
          handleSearchBatch({ preventDefault: () => {} } as React.FormEvent);
        }
      } else {
        toast.error(res.error || 'Failed to add proof to batch');
      }
    } catch (err) {
      toast.error('An unexpected error occurred while adding proof.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsAddingProof(false);
    }
  };

  const handleFinalizeBatchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFinalizeBatchId(e.target.value);
  };

  const handleVerifyBatchSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsVerifyingBatch(true);
    setVerifyBatchResult(null);
    try {
      const res = await verifyAggregationBatch(verifyBatchId);
      if (res.success) {
        setVerifyBatchResult(res.data);
        toast.success(`Batch ${verifyBatchId?.substring(0, 8)}... verification complete!`);
      } else {
        toast.error(res.error || 'Verification failed');
        setVerifyBatchResult({ error: res.error });
      }
    } catch (err) {
      toast.error('Verification error');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsVerifyingBatch(false);
    }
  };

  const handleFinalizeBatchSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsFinalizing(true);
    try {
      const res = await finalizeAggregationBatch({ batch_id: finalizeBatchId });
      if (res.success) {
        toast.success(`Batch ${finalizeBatchId?.substring(0, 8)}... finalized successfully! Merkle Root: ${res.data?.merkle_root?.substring(0, 8)}...`);
        setIsFinalizeModalOpen(false);
        setFinalizeBatchId('');
        // Optionally refresh batch details if currently viewing
        if (foundBatch?.batch_id === finalizeBatchId) {
          handleSearchBatch({ preventDefault: () => {} } as React.FormEvent);
        }
      } else {
        toast.error(res.error || 'Failed to finalize batch');
      }
    } catch (err) {
      toast.error('An unexpected error occurred during batch finalization.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsFinalizing(false);
    }
  };

  const handleSearchBatchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchBatchId(e.target.value);
  };

  const handleSearchBatch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!searchBatchId) {
      setSearchError('Please enter a Batch ID to search.');
      setFoundBatch(null);
      return;
    }
    setIsSearchingBatch(true);
    setSearchError(null);
    setFoundBatch(null);
    try {
      const res = await getAggregationBatchById(searchBatchId);
      if (res.success && res.data) {
        setFoundBatch(res.data);
        toast.success(`Batch ${searchBatchId} found.`);
      } else {
        setSearchError(res.error || `Batch with ID ${searchBatchId} not found.`);
        toast.error(res.error || `Batch with ID ${searchBatchId} not found.`);
      }
    } catch (err) {
      setSearchError('An unexpected error occurred during batch search.');
      toast.error('An unexpected error occurred during batch search.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsSearchingBatch(false);
    }
  };

  // Merkle Tree Visualization (simplified text representation)
  const renderMerkleTree = (proof_hashes: string[], merkle_root?: string) => {
    if (!proof_hashes || proof_hashes.length === 0) {
      return <p className="text-gray-500">No proofs in this batch to visualize.</p>;
    }

    const nodes = proof_hashes.map((id, index) => (
      <div key={index} className="flex items-center text-sm">
        <span className="text-red-500 mr-2">Leaf {index + 1}:</span>
        <span className="font-mono break-all">{id.substring(0, 16)}...</span>
      </div>
    ));

    return (
      <div className="bg-[#0b0b10] p-4 rounded-md border border-[#222235] mt-4">
        <h4 className="text-lg font-medium text-gray-300 mb-3">Merkle Tree Visualization (Simplified)</h4>
        <div className="space-y-2">
          {nodes}
          {proof_hashes.length > 1 && (
            <div className="flex items-center text-sm">
              <span className="text-red-500 mr-2">Intermediate Nodes:</span>
              <span className="text-gray-400">... (hashes of pairs)</span>
            </div>
          )}
          {merkle_root && (
            <div className="flex items-center text-sm">
              <span className="text-red-500 mr-2">Merkle Root:</span>
              <span className="font-mono break-all">{merkle_root}</span>
            </div>
          )}
          {!merkle_root && <p className="text-gray-500 text-sm">Batch not finalized, Merkle Root not available.</p>}
        </div>
      </div>
    );
  };

  return (
    <div className="p-4">
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-2xl font-bold text-gray-100">Proof Aggregation Lab</h2>
        <div className="flex space-x-4">
          <button
            onClick={() => setIsCreateBatchModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200"
          >
            <PlusCircle size={20} />
            Create Batch
          </button>
          <button
            onClick={() => setIsAddProofModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200"
          >
            <List size={20} />
            Add Proof
          </button>
          <button
            onClick={() => setIsFinalizeModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200"
          >
            <CheckCircle size={20} />
            Finalize Batch
          </button>
          <button
            onClick={() => setIsVerifyBatchModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-700 text-white rounded-md transition-colors duration-200"
          >
            <CheckCircle size={20} />
            Verify Batch
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Batch Creation Status */}
        <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-xl font-semibold text-gray-100 mb-4 flex items-center gap-2">
            <GitMerge size={24} className="text-red-500" />
            Batch Operations Overview
          </h3>
          <p className="text-gray-400 mb-4">
            Manage your proof batches: create new ones, add individual proofs, and finalize them into an aggregated proof.
          </p>
          {createdBatchId && (
            <div className="mt-4 p-3 bg-green-900/30 text-green-300 border border-green-700 rounded-md flex items-center gap-2">
              <CheckCircle size={20} />
              Last Created Batch ID: <span className="font-mono break-all">{createdBatchId}</span>
            </div>
          )}
          {!createdBatchId && (
            <div className="mt-4 p-3 bg-gray-900/30 text-gray-400 border border-gray-700 rounded-md flex items-center gap-2">
              <FileText size={20} />
              No batches created yet in this session.
            </div>
          )}
        </div>

        {/* View Batch Details */}
        <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-xl font-semibold text-gray-100 mb-4">View Batch Details</h3>
          <form onSubmit={handleSearchBatch} className="space-y-4">
            <div>
              <label htmlFor="searchBatchId" className="block text-sm font-medium text-gray-300 mb-1">
                Batch ID
              </label>
              <div className="relative">
                <input
                  type="text"
                  id="searchBatchId"
                  value={searchBatchId}
                  onChange={handleSearchBatchChange}
                  className="w-full pl-10 pr-4 py-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 placeholder-gray-500 focus:ring-red-500 focus:border-red-500"
                  placeholder="Enter Batch ID"
                  required
                />
                <Eye size={18} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              </div>
            </div>
            <button
              type="submit"
              disabled={isSearchingBatch}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSearchingBatch ? <Loader2 size={20} className="animate-spin" /> : <Eye size={20} />}
              {isSearchingBatch ? 'Searching...' : 'View Batch'}
            </button>
          </form>

          {searchError && (
            <div className="mt-4 p-3 bg-red-900/30 text-red-300 border border-red-700 rounded-md flex items-center gap-2">
              <AlertTriangle size={20} /> {searchError}
            </div>
          )}

          {foundBatch && (
            <div className="mt-6 p-4 bg-[#0b0b10] border border-[#222235] rounded-md space-y-2 text-sm">
              <h4 className="text-lg font-semibold text-red-400">Batch Details:</h4>
              <p><span className="font-medium text-gray-300">ID:</span> <span className="font-mono break-all">{foundBatch.batch_id}</span></p>
              <p><span className="font-medium text-gray-300">Name:</span> {foundBatch.batch_id}</p>
              {foundBatch?.max_proofs && <p><span className="font-medium text-gray-300">Max Proofs:</span> {foundBatch?.max_proofs}</p>}
              <p><span className="font-medium text-gray-300">Status:</span> <span className={`font-semibold ${foundBatch.status === 'finalized' ? 'text-green-400' : 'text-yellow-400'}`}>{(foundBatch.status || 'unknown').toUpperCase()}</span></p>
              <p><span className="font-medium text-gray-300">Proofs in Batch:</span> {foundBatch.proof_count ?? (foundBatch.proof_hashs?.length || 0)}</p>
              {foundBatch.merkle_root && <p><span className="font-medium text-gray-300">Merkle Root:</span> <span className="font-mono break-all">{foundBatch.merkle_root}</span></p>}
              {foundBatch.finalProof && <p><span className="font-medium text-gray-300">Final Proof:</span> <span className="font-mono break-all">{foundBatch?.finalProof?.substring(0, 60)}...</span></p>}
              {(foundBatch.created_at || foundBatch.createdAt) && <p><span className="font-medium text-gray-300">Created At:</span> {new Date((foundBatch.created_at || foundBatch.createdAt) * 1000).toLocaleString()}</p>}
              {(foundBatch.finalized_at || foundBatch.finalizedAt) ? <p><span className="font-medium text-gray-300">Finalized At:</span> {new Date((foundBatch.finalized_at || foundBatch.finalizedAt) * 1000).toLocaleString()}</p> : null}

              {renderMerkleTree(foundBatch.proof_hashs || [], foundBatch.merkle_root)}
            </div>
          )}
        </div>
      </div>

      {/* Create Batch Modal */}
      <Modal
        isOpen={isCreateBatchModalOpen}
        onClose={() => setIsCreateBatchModalOpen(false)}
        title="Create New Proof Batch"
      >
        <form onSubmit={handleCreateBatchSubmit} className="space-y-4">
          <div>
            <label htmlFor="batch_id" className="block text-sm font-medium text-gray-300 mb-1">
              Batch Name
            </label>
            <input
              type="text"
              id="batch_id"
              name="batch_id"
              value={newBatchData.batch_id}
              onChange={handleCreateBatchChange}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
              required
            />
          </div>
          <div>
            <label htmlFor="merkle_root" className="block text-sm font-medium text-gray-300 mb-1">
              Merkle Root (Optional)
            </label>
            <textarea
              id="merkle_root"
              name="merkle_root"
              rows={3}
              value={newBatchData?.merkle_root ?? ''}
              onChange={handleCreateBatchChange}
              placeholder="Leave empty to set later, or paste a known merkle root"
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
            ></textarea>
          </div>
          <button
            type="submit"
            disabled={isCreatingBatch}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isCreatingBatch ? <Loader2 size={20} className="animate-spin" /> : <PlusCircle size={20} />}
            {isCreatingBatch ? 'Creating...' : 'Create Batch'}
          </button>
        </form>
      </Modal>

      {/* Add Proof to Batch Modal */}
      <Modal
        isOpen={isAddProofModalOpen}
        onClose={() => setIsAddProofModalOpen(false)}
        title="Add Proof to Batch"
      >
        <form onSubmit={handleAddProofSubmit} className="space-y-4">
          <div>
            <label htmlFor="addProofBatchId" className="block text-sm font-medium text-gray-300 mb-1">
              Batch ID
            </label>
            <input
              type="text"
              id="addProofBatchId"
              name="batchId"
              value={addProofData.batch_id}
              onChange={handleAddProofChange}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
              placeholder="Enter existing Batch ID"
              required
            />
          </div>
          <div>
            <label htmlFor="addProofProofId" className="block text-sm font-medium text-gray-300 mb-1">
              Proof ID
            </label>
            <input
              type="text"
              id="addProofProofId"
              name="proofId"
              value={addProofData.proof_hash}
              onChange={handleAddProofChange}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
              placeholder="Enter Proof ID to add"
              required
            />
          </div>
          <button
            type="submit"
            disabled={isAddingProof}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isAddingProof ? <Loader2 size={20} className="animate-spin" /> : <PlusCircle size={20} />}
            {isAddingProof ? 'Adding Proof...' : 'Add Proof'}
          </button>
        </form>
      </Modal>

      {/* Finalize Batch Modal */}
      <Modal
        isOpen={isFinalizeModalOpen}
        onClose={() => setIsFinalizeModalOpen(false)}
        title="Finalize Proof Batch"
      >
        <form onSubmit={handleFinalizeBatchSubmit} className="space-y-4">
          <div>
            <label htmlFor="finalizeBatchId" className="block text-sm font-medium text-gray-300 mb-1">
              Batch ID to Finalize
            </label>
            <input
              type="text"
              id="finalizeBatchId"
              name="batchId"
              value={finalizeBatchId}
              onChange={handleFinalizeBatchChange}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
              placeholder="Enter Batch ID"
              required
            />
          </div>
          <button
            type="submit"
            disabled={isFinalizing}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isFinalizing ? <Loader2 size={20} className="animate-spin" /> : <CheckCircle size={20} />}
            {isFinalizing ? 'Finalizing...' : 'Finalize Batch'}
          </button>
        </form>
      </Modal>

      {/* Verify Batch Modal */}
      <Modal isOpen={isVerifyBatchModalOpen} onClose={() => { setIsVerifyBatchModalOpen(false); setVerifyBatchResult(null); }} title="Verify Aggregation Batch">
        <form onSubmit={handleVerifyBatchSubmit} className="space-y-4">
          <div>
            <label className="text-sm font-medium text-gray-300 block mb-1">Batch ID</label>
            <input
              type="text"
              value={verifyBatchId}
              onChange={(e) => setVerifyBatchId(e.target.value)}
              required
              className="w-full bg-[#0b0b10] border border-[#222235] text-gray-100 px-3 py-2 rounded text-sm font-mono"
              placeholder="Enter batch ID to verify"
            />
          </div>
          <button
            type="submit"
            disabled={isVerifyingBatch || !verifyBatchId}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50"
          >
            {isVerifyingBatch ? <Loader2 size={20} className="animate-spin" /> : <CheckCircle size={20} />}
            {isVerifyingBatch ? 'Verifying...' : 'Verify Batch'}
          </button>
          {verifyBatchResult && (
            <div className="mt-4">
              <pre className={`bg-[#0b0b10] p-3 rounded text-xs overflow-auto max-h-48 font-mono ${verifyBatchResult.error ? 'text-red-300' : 'text-green-300'}`}>
                {JSON.stringify(verifyBatchResult, null, 2)}
              </pre>
            </div>
          )}
        </form>
      </Modal>
    </div>
  );
};

export default Aggregation;
