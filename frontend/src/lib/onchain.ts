/**
 * onchain.ts — typed loader for the canonical on-chain deployment manifest.
 *
 * The manifest is served as a static asset at /onchain.json. It is generated
 * from deploy-out/onchain.json via scripts/gen-manifest.mjs; DO NOT edit
 * frontend/public/onchain.json by hand — regenerate instead.
 *
 * Contract addresses that appear in TS/TSX source code should be loaded
 * through here instead of being hardcoded, so that a redeploy only requires
 * updating deploy-out/onchain.json + rerunning the generator.
 */

export interface ContractEntry {
  contract_hash: string;
  contract_package_hash?: string;
  deploy_hash: string;
  version?: number;
  deployed_at?: string;
  source: string;
  entry_points?: string[];
  notes?: string;
}

export interface UndeployedContractEntry {
  source: string;
  status: 'compiled' | 'wip' | 'planned';
  reason?: string;
}

export interface OnChainManifest {
  network: 'casper-test' | 'casper-mainnet';
  chain_name: string;
  project: string;
  deployer: string;
  explorer: string;
  cspr_cloud?: string;
  contracts: Record<string, ContractEntry>;
  undeployed_contracts?: Record<string, UndeployedContractEntry>;
  verification: {
    api_health: string;
    frontend: string;
    verify_script?: string;
  };
}

let cached: OnChainManifest | null = null;
let inflight: Promise<OnChainManifest> | null = null;

/**
 * Load /onchain.json. Cached forever within a page load (contract hashes
 * do not change between deploys — a redeploy always ships a new build).
 */
export async function loadManifest(): Promise<OnChainManifest> {
  if (cached) return cached;
  if (inflight) return inflight;
  inflight = fetch('/onchain.json', { cache: 'force-cache' })
    .then((r) => {
      if (!r.ok) throw new Error(`onchain manifest fetch failed: HTTP ${r.status}`);
      return r.json() as Promise<OnChainManifest>;
    })
    .then((m) => {
      cached = m;
      inflight = null;
      return m;
    })
    .catch((e) => {
      inflight = null;
      throw e;
    });
  return inflight;
}

/**
 * Convenience getter for a specific contract's on-chain hash.
 * Returns null for a contract that exists in the manifest but is not deployed.
 */
export async function getContractHash(
  key: 'proof_registry' | 'verifier_gate' | 'defi_mock' | 'stake_slashing' | string,
): Promise<string | null> {
  const m = await loadManifest();
  const c = m.contracts[key];
  return c ? c.contract_hash : null;
}

/**
 * Synchronous cached snapshot for components that were rendered after a
 * successful loadManifest() upstream. Returns null before the first fetch
 * resolves — callers should fall back to loadManifest() in that case.
 */
export function getCachedManifest(): OnChainManifest | null {
  return cached;
}

/**
 * Testing/SSR seam: inject a preloaded manifest so components can render
 * synchronously without a network round-trip.
 */
export function primeManifest(m: OnChainManifest): void {
  cached = m;
}
