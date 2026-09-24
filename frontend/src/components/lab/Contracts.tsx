import React, { useEffect, useState } from 'react';
import SectionIntro from './SectionIntro';
import TrustStatus from '../TrustStatus';
import { Link as LinkIcon, FileText, Code, Shield, Swords, Box, Layers, Brain, ExternalLink, Landmark, KeyRound } from 'lucide-react';
import { loadManifest, OnChainManifest } from '../../lib/onchain';

// Presentation-only metadata (icon, purpose, lines) keyed by manifest name.
// On-chain data (address, deployed?, deployDate) is loaded from
// /onchain.json (generated from deploy-out/onchain.json). A redeploy no
// longer requires editing this file — only regenerate the manifest.
interface PresentationMeta {
  key: string;          // key in manifest.contracts / manifest.undeployed_contracts
  name: string;         // display name (may differ, e.g. dashes vs underscores)
  purpose: string;
  icon: React.ElementType;
  lines: number;
}

const PRESENTATION: PresentationMeta[] = [
  { key: 'proof_registry', name: 'proof-registry', purpose: 'Immutable on-chain store for all proof metadata — hashes, Merkle roots, timestamps, and verification status.', icon: FileText, lines: 251 },
  { key: 'verifier_gate', name: 'verifier-gate', purpose: 'Gateway contract for Merkle inclusion verification — checks proof existence and validity via cross-contract calls.', icon: Shield, lines: 143 },
  { key: 'defi_mock', name: 'defi-mock', purpose: 'KYC-gated DeFi vault — demonstrates proof-based access control for financial operations via cross-contract verification.', icon: Code, lines: 202 },
  { key: 'stake_slashing', name: 'stake-slashing', purpose: 'Economic penalty contract — 20% CSPR slash on revoked proofs with permissionless bounty for reporters. Cross-contract call to proof-registry.', icon: Swords, lines: 273 },
  { key: 'proof_of_inference', name: 'proof-of-inference', purpose: 'Full inference proof contract — records model hash, input/output commitments, and verification result on-chain for each AI decision.', icon: Brain, lines: 498 },
  { key: 'model_registry', name: 'model-registry', purpose: 'On-chain model versioning registry — tracks model hashes, ownership, and version history for provenance auditing.', icon: Box, lines: 372 },
  { key: 'proof_aggregation', name: 'proof-aggregation', purpose: 'Batch aggregation contract — stores Merkle roots of aggregated proof batches for gas-efficient on-chain verification.', icon: Layers, lines: 179 },
  { key: 'governance', name: 'governance', purpose: '48-hour timelock with 2-of-3 guardian recovery — governs upgrades and admin actions across the other contracts.', icon: Landmark, lines: 581 },
  { key: 'zk_verifier', name: 'zk-verifier', purpose: 'On-chain vk registry + verdict recorder for Groth16 proofs — records what an off-chain verifier decided (proof_hash, vk_hash, verdict), not a pairing verifier itself.', icon: KeyRound, lines: 536 },
];

interface ContractRow extends PresentationMeta {
  address: string | null;
  deployed: boolean;
  deployDate?: string;
}

const EXPLORER_BASE_URL_FALLBACK = 'https://testnet.cspr.live/contract/';
const GITHUB_BASE_URL = 'https://github.com/anna-stolbovskaja/Bot Prove/tree/main/contracts/';

function joinContract(base: string): string {
  if (!base) return EXPLORER_BASE_URL_FALLBACK;
  return base.endsWith('/') ? `${base}contract/` : `${base}/contract/`;
}

const Contracts: React.FC = () => {
  const [manifest, setManifest] = useState<OnChainManifest | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadManifest()
      .then(setManifest)
      .catch((e) => setError(e.message ?? String(e)));
  }, []);

  if (error) {
    return (
      <div className="p-4">
        <div className="bg-red-950/40 border border-red-500/40 rounded-md px-4 py-3 text-sm text-red-200">
          Failed to load on-chain manifest ({error}). Contract data cannot be
          displayed without a live <code>/onchain.json</code>.
        </div>
      </div>
    );
  }

  if (!manifest) {
    return <div className="p-4 text-gray-500 text-sm">Loading on-chain manifest…</div>;
  }

  const contracts: ContractRow[] = PRESENTATION.map((p) => {
    const dep = manifest.contracts[p.key];
    if (dep) {
      return {
        ...p,
        address: dep.contract_hash,
        deployed: true,
        deployDate: dep.deployed_at ? dep.deployed_at.slice(0, 10) : undefined,
      };
    }
    return { ...p, address: null, deployed: false };
  });

  const deployed = contracts.filter((c) => c.deployed);
  const written = contracts.filter((c) => !c.deployed);
  const explorerBase = joinContract(manifest.explorer);

  return (
    <div className="p-4">
      <SectionIntro
        title="Smart Contracts"
        description={`${contracts.length} Rust/Wasm smart contracts built for Bot Prove: ${deployed.length} deployed on ${manifest.network} and ${written.length} written but not yet deployed. Each contract is verified on-chain with real deploy hashes — click to view on Casper Explorer.`}
        dataSource={`Live from /onchain.json (generated from deploy-out/onchain.json). Network: ${manifest.network}.`}
        badge="On-chain verified"
        badgeColor="green"
      />
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Bot Prove Contracts</h2>
      <p className="text-gray-400 mb-6">
        {deployed.length} deployed on {manifest.network} · {written.length} written and ready for mainnet · {contracts.reduce((s, c) => s + c.lines, 0).toLocaleString()} lines of Rust
      </p>

      {/* Deployed contracts */}
      <h3 className="text-lg font-semibold text-green-400 mb-4 flex items-center gap-2">
        <span className="w-2 h-2 rounded-full bg-green-400" /> Deployed on Testnet
      </h3>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
        {deployed.map((c) => (
          <div key={c.name} className="bg-[#1a1a2a] p-5 rounded-lg border border-[#222235] shadow-md">
            <div className="flex items-center gap-3 mb-3">
              {React.createElement(c.icon, { size: 24, className: 'text-red-500' })}
              <h3 className="text-lg font-semibold text-gray-100">{c.name}</h3>
              <TrustStatus kind="on-chain" className="ml-auto" />
              {c.deployDate && (
                <span className="text-xs text-green-400/70 border border-green-500/20 px-2 py-0.5 rounded">
                  {c.deployDate}
                </span>
              )}
            </div>
            <p className="text-gray-400 text-sm mb-3">{c.purpose}</p>
            <div className="text-xs text-gray-500 font-mono mb-3 break-all">
              {c.address?.slice(0, 16)}...{c.address?.slice(-8)}
            </div>
            <div className="flex items-center gap-3">
              <a
                href={`${explorerBase}${c.address}`}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white rounded-md text-xs font-medium transition-colors"
              >
                <LinkIcon size={14} /> CasperLive
              </a>
              <a
                href={`${GITHUB_BASE_URL}${c.name}`}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-gray-700 hover:border-gray-500 text-gray-300 rounded-md text-xs font-medium transition-colors"
              >
                <ExternalLink size={14} /> Source ({c.lines} lines)
              </a>
            </div>
          </div>
        ))}
      </div>

      {/* Written contracts */}
      <h3 className="text-lg font-semibold text-yellow-400 mb-4 flex items-center gap-2">
        <span className="w-2 h-2 rounded-full bg-yellow-400" /> Written — Ready for Mainnet
      </h3>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-8">
        {written.map((c) => (
          <div key={c.name} className="bg-[#1a1a2a] p-5 rounded-lg border border-[#222235]/60 shadow-md">
            <div className="flex items-center gap-3 mb-3">
              {React.createElement(c.icon, { size: 24, className: 'text-yellow-500' })}
              <h3 className="text-lg font-semibold text-gray-100">{c.name}</h3>
              <TrustStatus kind="built-not-deployed" className="ml-auto" />
            </div>
            <p className="text-gray-400 text-sm mb-3">{c.purpose}</p>
            <a
              href={`${GITHUB_BASE_URL}${c.name}`}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-gray-700 hover:border-gray-500 text-gray-300 rounded-md text-xs font-medium transition-colors"
            >
              <ExternalLink size={14} /> Source ({c.lines} lines)
            </a>
          </div>
        ))}
      </div>

      {/* Additional info */}
      <div className="bg-[#1a1a2a] p-5 rounded-lg border border-[#222235] shadow-md">
        <h3 className="text-lg font-semibold text-gray-100 mb-3">Architecture Notes</h3>
        <ul className="text-gray-400 text-sm space-y-2">
          <li>• All contracts are written in Rust for Casper 2.x (CEP-18 compatible)</li>
          <li>• <strong>stake-slashing</strong> uses cross-contract calls to read proof state from <strong>proof-registry</strong></li>
          <li>• <strong>defi-mock</strong> demonstrates KYC gating via cross-contract verification against stored proofs</li>
          <li>• <strong>stake-slashing-session</strong> — helper session contract for initiating stake/slash operations</li>
          <li>• Integration tests cover all entry points (<code>contracts/tests/</code>)</li>
          <li>• 248+ testnet transactions across contract deploys and entry-point calls</li>
        </ul>
      </div>
    </div>
  );
};

export default Contracts;
