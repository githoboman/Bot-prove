/**
 * Client-side transaction building for wallet-signed on-chain operations.
 *
 * Rewritten for BOT Chain (EVM) compatibility using window.ethereum directly.
 */
import { getCachedManifest } from './onchain';

export const BOT_CHAIN_NAME = 'bot-test';

export let PROOF_REGISTRY_HASH = '0x601794aFcE3443668f0280ACA1c2e522739478f7'; // Default to Botchain Mainnet deploy

export function getProofRegistryHash(): string {
  // Use botchain_prover if available, fallback to the hardcoded mainnet hash
  return getCachedManifest()?.contracts?.botchain_prover?.contract_address || 
         getCachedManifest()?.contracts?.proof_registry?.contract_hash || 
         PROOF_REGISTRY_HASH;
}

export type LiveTxResult =
  | { ok: true; transactionHash: string }
  | { ok: false; cancelled: true }
  | { ok: false; cancelled: false; error: string };

// Utility to encode strings into hex for raw EVM calldata if needed (simplified)
function stringToHex(str: string) {
  return Array.from(str).map(c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('');
}

// Helper to pad hex string to 32 bytes (64 chars)
function pad32(hex: string) {
  return hex.replace('0x', '').padStart(64, '0');
}

// Minimal ABI Encoder for: function storeProof(bytes32 proofHash, string memory model)
function encodeStoreProof(proofHash: string, modelStr: string): string {
  const selector = '9fb43444'; // keccak256("storeProof(bytes32,string)")
  const p1 = pad32(proofHash);
  const p2Offset = pad32(Number(64).toString(16)); // Offset to dynamic string is 64 bytes (0x40)
  
  // Encode string length and data
  const hexStr = stringToHex(modelStr);
  const strLen = pad32((hexStr.length / 2).toString(16));
  // Pad string data to multiple of 32 bytes (64 hex chars)
  const padLen = Math.ceil(hexStr.length / 64) * 64;
  const strData = hexStr.padEnd(padLen, '0');
  
  return '0x' + selector + p1 + p2Offset + strLen + strData;
}

/**
 * Submit a proof on-chain via the connected EVM wallet.
 */
export async function submitProofOnChain(
  _clickRef: any, // Ignored in EVM
  opts: {
    proofHash: string;
    inputHash: string;
    outputHash: string;
    modelHash: string;
    senderPublicKeyHex: string;
  }
): Promise<LiveTxResult> {
  if (!window.ethereum) return { ok: false, cancelled: false, error: "No wallet detected" };

  try {
    const txParams = {
      to: getProofRegistryHash(),
      from: opts.senderPublicKeyHex,
      value: '0x0',
      data: encodeStoreProof(opts.proofHash, opts.modelHash)
    };

    const txHash = await window.ethereum.request({
      method: 'eth_sendTransaction',
      params: [txParams],
    });

    return { ok: true, transactionHash: txHash };
  } catch (err: any) {
    if (err.code === 4001) return { ok: false, cancelled: true };
    return { ok: false, cancelled: false, error: err?.message || String(err) };
  }
}

/**
 * Register an agent on-chain via the connected EVM wallet.
 */
export async function registerAgentOnChain(
  _clickRef: any,
  opts: {
    agentId: string;
    modelHash: string;
    senderPublicKeyHex: string;
  }
): Promise<LiveTxResult> {
  if (!window.ethereum) return { ok: false, cancelled: false, error: "No wallet detected" };

  try {
    const txParams = {
      to: getProofRegistryHash(),
      from: opts.senderPublicKeyHex,
      value: '0x0',
      data: '0x' + stringToHex(`register_agent:${opts.agentId}:${opts.modelHash}`)
    };

    const txHash = await window.ethereum.request({
      method: 'eth_sendTransaction',
      params: [txParams],
    });

    return { ok: true, transactionHash: txHash };
  } catch (err: any) {
    if (err.code === 4001) return { ok: false, cancelled: true };
    return { ok: false, cancelled: false, error: err?.message || String(err) };
  }
}

/**
 * Revoke a proof on-chain via the connected EVM wallet.
 */
export async function revokeProofOnChain(
  _clickRef: any,
  opts: {
    proofId: string;
    senderPublicKeyHex: string;
  }
): Promise<LiveTxResult> {
  if (!window.ethereum) return { ok: false, cancelled: false, error: "No wallet detected" };

  try {
    const txParams = {
      to: getProofRegistryHash(),
      from: opts.senderPublicKeyHex,
      value: '0x0',
      data: '0x' + stringToHex(`revoke_proof:${opts.proofId}`)
    };

    const txHash = await window.ethereum.request({
      method: 'eth_sendTransaction',
      params: [txParams],
    });

    return { ok: true, transactionHash: txHash };
  } catch (err: any) {
    if (err.code === 4001) return { ok: false, cancelled: true };
    return { ok: false, cancelled: false, error: err?.message || String(err) };
  }
}
