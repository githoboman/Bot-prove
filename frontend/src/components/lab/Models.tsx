import React, { useState, useEffect, useCallback } from 'react';
import { PlusCircle, Search, FileText, Loader2, AlertTriangle, Eye, XCircle,
} from 'lucide-react';
import {
  registerModel,
  getModelById,
  RegisterModelRequest,
  
  
} from '../../lib/api';
import { toast } from '../ui/toast';
import SectionIntro from './SectionIntro';
import { getCachedManifest, loadManifest } from '../../lib/onchain';

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

// Fallback proof_registry hash (used before /onchain.json resolves). Real
// value is refreshed from the manifest on mount; a redeploy that changes
// the hash only needs the manifest regenerated, not this file.
const PROOF_REGISTRY_FALLBACK = '96e97c4d564fe7374ba4e938355fb89f5be2f448decbe9b7727bd3c978a10708';

function verifierContractFromManifest(): string {
  return getCachedManifest()?.contracts.proof_registry?.contract_hash ?? PROOF_REGISTRY_FALLBACK;
}

const Models: React.FC = () => {
  const [isRegisterModalOpen, setIsRegisterModalOpen] = useState(false);
  const [newModelData, setNewModelData] = useState<RegisterModelRequest>({
    model_id: `gpt-4o-${Date.now().toString(36)}`,
    model_hash: Array.from(crypto.getRandomValues(new Uint8Array(32))).map(b => b.toString(16).padStart(2, '0')).join(''),
    verifier_contract: verifierContractFromManifest(),
    metadata: { type: 'llm', params: '175B' },
  });

  // Refresh verifier_contract from the manifest once it resolves, so the
  // 'Register Model' form starts on the right hash after a redeploy.
  useEffect(() => {
    let alive = true;
    loadManifest().then((m) => {
      if (!alive) return;
      const hash = m.contracts.proof_registry?.contract_hash;
      if (hash) {
        setNewModelData((prev) => (
          prev.verifier_contract === PROOF_REGISTRY_FALLBACK
            ? { ...prev, verifier_contract: hash }
            : prev
        ));
      }
    }).catch(() => { /* stay on fallback */ });
    return () => { alive = false; };
  }, []);
  const [isRegistering, setIsRegistering] = useState(false);

  const [searchModelId, setSearchModelId] = useState('');
  const [foundModel, setFoundModel] = useState<any | null>(null);
  const [isSearchingModel, setIsSearchingModel] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);

  // For "List registered models" - since API doesn't have a direct list, we'll simulate or show registered models after creation
  const [registeredModels, setRegisteredModels] = useState<any[]>([]);

  const handleRegisterModelChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target;
    setNewModelData((prev) => ({ ...prev, [name]: value }));
  };

  const handleRegisterModelSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsRegistering(true);
    setSearchError(null);
    try {
      const res = await registerModel(newModelData);
      if (res.success && res.data) {
        toast.success(`Model "${newModelData.model_id}" registered successfully! ID: ${res.data.model_id}`);
        // Add to local list for display (assuming we can fetch it back)
        const fetchedModelRes = await getModelById(res.data.model_id);
        if (fetchedModelRes.success && fetchedModelRes.data) {
          setRegisteredModels((prev) => [...prev, fetchedModelRes.data!]);
        }
        setIsRegisterModalOpen(false);
        setNewModelData({ model_id: '', model_hash: '', verifier_contract: '', metadata: {} });
      } else {
        toast.error(res.error || 'Failed to register model');
      }
    } catch (err) {
      toast.error('An unexpected error occurred during model registration.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsRegistering(false);
    }
  };

  const handleSearchModel = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!searchModelId) {
      setSearchError('Please enter a Model ID to search.');
      setFoundModel(null);
      return;
    }
    setIsSearchingModel(true);
    setSearchError(null);
    setFoundModel(null);
    try {
      const res = await getModelById(searchModelId);
      if (res.success && res.data) {
        setFoundModel(res.data);
        toast.success(`Box ${searchModelId} found.`);
      } else {
        setSearchError(res.error || `Box with ID ${searchModelId} not found.`);
        toast.error(res.error || `Box with ID ${searchModelId} not found.`);
      }
    } catch (err) {
      setSearchError('An unexpected error occurred during model search.');
      toast.error('An unexpected error occurred during model search.');
      if (import.meta.env.DEV) console.error(err);
    } finally {
      setIsSearchingModel(false);
    }
  };

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text).then(() => {
      toast.success(`${label} copied to clipboard`);
    }).catch(() => {
      toast.error('Failed to copy');
    });
  };

  return (
    <div className="p-4">
      <SectionIntro
        title="AI Box Registry"
        description="Register and look up AI model boxes (LLMs, decision engines) in the Bot Prove model registry. Each box is identified by a cryptographic hash and linked to a verifier smart contract on BOT Chain testnet. This enables tamper-proof model provenance — any proof can be traced back to the exact model version that produced it."
        dataSource="On-chain model registry via the verifier_gate smart contract on BOT Chain testnet."
        badge="On-chain registry"
        badgeColor="green"
      />

      <div className="flex justify-between items-center mb-6">
        <h2 className="text-2xl font-bold text-gray-100">AI Box Registry</h2>
        <button
          onClick={() => setIsRegisterModalOpen(true)}
          className="flex items-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200"
        >
          <PlusCircle size={20} />
          Register New Model
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">

        {/* View Box Details */}
        <div className="bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
          <h3 className="text-xl font-semibold text-gray-100 mb-4">View Box Details</h3>
          <form onSubmit={handleSearchModel} className="space-y-4">
            <div>
              <label htmlFor="searchModelId" className="block text-sm font-medium text-gray-300 mb-1">
                Model ID
              </label>
              <div className="relative">
                <input
                  type="text"
                  id="searchModelId"
                  value={searchModelId}
                  onChange={(e) => setSearchModelId(e.target.value)}
                  className="w-full pl-10 pr-4 py-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 placeholder-gray-500 focus:ring-red-500 focus:border-red-500"
                  placeholder="Enter Model ID"
                  required
                />
                <Search size={18} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              </div>
            </div>
            <button
              type="submit"
              disabled={isSearchingModel}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSearchingModel ? <Loader2 size={20} className="animate-spin" /> : <Eye size={20} />}
              {isSearchingModel ? 'Searching...' : 'View Box'}
            </button>
          </form>

          {searchError && (
            <div className="mt-4 p-3 bg-red-900/30 text-red-300 border border-red-700 rounded-md flex items-center gap-2">
              <AlertTriangle size={20} /> {searchError}
            </div>
          )}

          {foundModel && (
            <div className="mt-6 p-4 bg-[#0b0b10] border border-[#222235] rounded-md space-y-2 text-sm">
              <h4 className="text-lg font-semibold text-red-400">Box Found:</h4>
              <p><span className="font-medium text-gray-300">ID:</span> <span className="font-mono break-all cursor-pointer hover:text-red-300 text-red-400 transition-colors" onClick={() => copyToClipboard(foundModel.model_id, 'Model ID')} title="Click to copy">{foundModel.model_id}</span></p>
              <p><span className="font-medium text-gray-300">Name:</span> <span className="cursor-pointer hover:text-gray-100 transition-colors" onClick={() => copyToClipboard(foundModel.model_id, 'Model name')} title="Click to copy">{foundModel.model_id}</span></p>
              <p><span className="font-medium text-gray-300">Hash:</span> <span className="font-mono break-all cursor-pointer hover:text-gray-100 transition-colors" onClick={() => copyToClipboard(foundModel.model_hash, 'Hash')} title="Click to copy">{foundModel.model_hash}</span></p>
              <p><span className="font-medium text-gray-300">Verifier Contract:</span> <span className="font-mono break-all cursor-pointer hover:text-gray-100 transition-colors" onClick={() => copyToClipboard(foundModel.verifier_contract, 'Contract')} title="Click to copy">{foundModel.verifier_contract}</span></p>
              {foundModel?.description && <p><span className="font-medium text-gray-300">Description:</span> {foundModel?.description}</p>}
              <p><span className="font-medium text-gray-300">Registered At:</span> {new Date(foundModel.registered_at).toLocaleString()}</p>
            </div>
          )}
        </div>
      </div>

      {/* Recently Registered Models */}
      <div className="mt-8 bg-[#1a1a2a] p-6 rounded-lg border border-[#222235] shadow-md">
        <h3 className="text-xl font-semibold text-gray-100 mb-4">Recently Registered Models</h3>
        {registeredModels.length === 0 ? (
          <div className="text-center p-4 text-gray-400">
            <FileText className="mx-auto mb-2" size={32} />
            <p>No models registered yet in this session.</p>
            <p className="text-sm mt-1">Registered models would appear here if a "list all models" API endpoint existed.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-[#222235]">
              <thead className="bg-[#13131d]">
                <tr>
                  <th scope="col" className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                    Model ID
                  </th>
                  <th scope="col" className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                    Name
                  </th>
                  <th scope="col" className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                    Hash
                  </th>
                  <th scope="col" className="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                    Verifier Contract
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#222235]">
                {registeredModels.map((model) => (
                  <tr key={model.model_id} className="hover:bg-[#1a1a2a]/50 transition-colors duration-150">
                    <td
                      className="px-6 py-4 whitespace-nowrap text-sm font-mono text-red-400 cursor-pointer hover:text-red-300 transition-colors"
                      onClick={() => copyToClipboard(model.model_id, 'Model ID')}
                      title={`${model.model_id} — click to copy`}
                    >
                      {model?.model_id?.substring(0, 8)}...{model?.model_id?.substring(model.model_id.length - 8)}
                    </td>
                    <td
                      className="px-6 py-4 whitespace-nowrap text-sm text-gray-300 cursor-pointer hover:text-gray-100 transition-colors"
                      onClick={() => copyToClipboard(model.model_id, 'Model name')}
                      title="Click to copy full name"
                    >
                      {model.model_id}
                    </td>
                    <td
                      className="px-6 py-4 whitespace-nowrap text-sm font-mono text-gray-300 cursor-pointer hover:text-gray-100 transition-colors"
                      onClick={() => copyToClipboard(model.model_hash, 'Model hash')}
                      title={`${model.model_hash} — click to copy`}
                    >
                      {model?.model_hash?.substring(0, 8)}...
                    </td>
                    <td
                      className="px-6 py-4 whitespace-nowrap text-sm font-mono text-gray-300 cursor-pointer hover:text-gray-100 transition-colors"
                      onClick={() => copyToClipboard(model.verifier_contract, 'Contract address')}
                      title={`${model.verifier_contract} — click to copy`}
                    >
                      {model?.verifier_contract?.substring(0, 8)}...
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>


      {/* Register Box Modal */}
      <Modal
        isOpen={isRegisterModalOpen}
        onClose={() => setIsRegisterModalOpen(false)}
        title="Register New AI Box"
      >
        <form onSubmit={handleRegisterModelSubmit} className="space-y-4">
          <div>
            <label htmlFor="model_id" className="block text-sm font-medium text-gray-300 mb-1">
              Box Name
            </label>
            <input
              type="text"
              id="model_id"
              name="model_id"
              value={newModelData.model_id}
              onChange={handleRegisterModelChange}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
              required
            />
          </div>
          <div>
            <label htmlFor="model_hash" className="block text-sm font-medium text-gray-300 mb-1">
              Box Hash (e.g., IPFS CID, cryptographic hash)
            </label>
            <input
              type="text"
              id="model_hash"
              name="model_hash"
              value={newModelData.model_hash}
              onChange={handleRegisterModelChange}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
              required
            />
          </div>
          <div>
            <label htmlFor="verifier_contract" className="block text-sm font-medium text-gray-300 mb-1">
              Verifier Contract Address (on BOT Chain)
            </label>
            <input
              type="text"
              id="verifier_contract"
              name="verifier_contract"
              value={newModelData.verifier_contract}
              onChange={handleRegisterModelChange}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 font-mono focus:ring-red-500 focus:border-red-500"
              required
            />
          </div>
          <div>
            <label htmlFor="description" className="block text-sm font-medium text-gray-300 mb-1">
              Description (Optional)
            </label>
            <textarea
              id="metadata"
              name="metadata"
              rows={3}
              value={JSON.stringify(newModelData.metadata || {})}
              onChange={handleRegisterModelChange}
              className="w-full p-2 bg-[#0b0b10] border border-[#222235] rounded-md text-gray-100 focus:ring-red-500 focus:border-red-500"
            ></textarea>
          </div>
          <button
            type="submit"
            disabled={isRegistering}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isRegistering ? <Loader2 size={20} className="animate-spin" /> : <PlusCircle size={20} />}
            {isRegistering ? 'Registering...' : 'Register Box'}
          </button>
        </form>
      </Modal>
    </div>
  );
};

export default Models;
