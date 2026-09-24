/**
 * Client-side transaction building for wallet-signed on-chain operations.
 *
 * When a Casper wallet is connected, write operations (submit_proof,
 * register_agent) can be signed and submitted directly by the user's wallet
 * instead of the backend's server key. Standard CSPR.click flow:
 * build a ContractCallBuilder tx → clickRef.send() → wallet popup
 * → user signs → tx submitted to BOT Chain testnet.
 *
 * Contract: proof_registry (deployed on casper-test)
 * Entry points: submit_proof, register_agent, revoke_proof
 */
import {
  Args,
  CLValue,
  ContractCallBuilder,
  PublicKey,
} from 'casper-js-sdk'
import type { ICSPRClickSDK } from '@make-software/csprclick-core-types'
import { loadManifest, getCachedManifest } from './onchain'

// Chain name is per-network; kept as a constant because the CSPR.click SDK
// needs it synchronously for tx construction. If we ever ship to mainnet
// this becomes manifest-driven too.
export const CASPER_CHAIN_NAME = 'casper-test'

// Boot-time fallback: last known proof_registry hash. The real value comes
// from the manifest via loadManifest() below. The export stays for backward
// compat, but callers should treat it as advisory and prefer
// getProofRegistryHash() when constructing txs.
export let PROOF_REGISTRY_HASH = '96e97c4d564fe7374ba4e938355fb89f5be2f448decbe9b7727bd3c978a10708'

/**
 * Refresh PROOF_REGISTRY_HASH from the canonical /onchain.json. Fires and
 * forgets — a redeploy that changes the hash before this resolves would
 * still ship the fallback for a few milliseconds, which is acceptable
 * (the frontend cannot construct a tx before mount anyway).
 */
void loadManifest()
  .then((m) => {
    const fresh = m.contracts.proof_registry?.contract_hash
    if (fresh && fresh !== PROOF_REGISTRY_HASH) {
      // eslint-disable-next-line no-console
      console.info(`[liveTx] proof_registry hash refreshed from manifest: ${PROOF_REGISTRY_HASH.slice(0, 8)}… → ${fresh.slice(0, 8)}…`)
      PROOF_REGISTRY_HASH = fresh
    }
  })
  .catch(() => {
    /* manifest unavailable — stay on fallback, log elsewhere */
  })

/** Preferred accessor: returns the manifest-resolved hash if available, else the fallback. */
export function getProofRegistryHash(): string {
  return getCachedManifest()?.contracts.proof_registry?.contract_hash ?? PROOF_REGISTRY_HASH
}

export const PAYMENT_MOTES = 3_000_000_000 // 3 CSPR — sufficient for dictionary writes

export type LiveTxResult =
  | { ok: true; transactionHash: string }
  | { ok: false; cancelled: true }
  | { ok: false; cancelled: false; error: string }

/**
 * Submit a proof on-chain via the connected wallet.
 * Calls the proof_registry contract's `submit_proof` entry point.
 */
export async function submitProofOnChain(
  clickRef: ICSPRClickSDK,
  opts: {
    proofHash: string
    inputHash: string
    outputHash: string
    modelHash: string
    senderPublicKeyHex: string
  },
): Promise<LiveTxResult> {
  try {
    const tx = new ContractCallBuilder()
      .byHash(getProofRegistryHash())
      .entryPoint('submit_proof')
      .runtimeArgs(
        Args.fromMap({
          proof_hash: CLValue.newCLString(opts.proofHash),
          input_hash: CLValue.newCLString(opts.inputHash),
          output_hash: CLValue.newCLString(opts.outputHash),
          model_hash: CLValue.newCLString(opts.modelHash),
        }),
      )
      .from(PublicKey.fromHex(opts.senderPublicKeyHex))
      .chainName(CASPER_CHAIN_NAME)
      .payment(PAYMENT_MOTES)
      .build()

    const res = await clickRef.send(tx.toJSON() as object, opts.senderPublicKeyHex)

    if (res?.transactionHash) {
      return { ok: true, transactionHash: res.transactionHash }
    }
    if (res?.cancelled) {
      return { ok: false, cancelled: true }
    }
    const rawError = (res as any)?.error ?? (res as any)?.errorData ?? 'Unknown error from wallet SDK'
    return { ok: false, cancelled: false, error: typeof rawError === 'string' ? rawError : JSON.stringify(rawError) }
  } catch (err: any) {
    return { ok: false, cancelled: false, error: err?.message || String(err) }
  }
}

/**
 * Register an agent on-chain via the connected wallet.
 * Calls the proof_registry contract's `register_agent` entry point.
 */
export async function registerAgentOnChain(
  clickRef: ICSPRClickSDK,
  opts: {
    agentId: string
    modelHash: string
    senderPublicKeyHex: string
  },
): Promise<LiveTxResult> {
  try {
    const tx = new ContractCallBuilder()
      .byHash(getProofRegistryHash())
      .entryPoint('register_agent')
      .runtimeArgs(
        Args.fromMap({
          agent_id: CLValue.newCLString(opts.agentId),
          model_hash: CLValue.newCLString(opts.modelHash),
        }),
      )
      .from(PublicKey.fromHex(opts.senderPublicKeyHex))
      .chainName(CASPER_CHAIN_NAME)
      .payment(PAYMENT_MOTES)
      .build()

    const res = await clickRef.send(tx.toJSON() as object, opts.senderPublicKeyHex)

    if (res?.transactionHash) {
      return { ok: true, transactionHash: res.transactionHash }
    }
    if (res?.cancelled) {
      return { ok: false, cancelled: true }
    }
    const rawError = (res as any)?.error ?? (res as any)?.errorData ?? 'Unknown error'
    return { ok: false, cancelled: false, error: typeof rawError === 'string' ? rawError : JSON.stringify(rawError) }
  } catch (err: any) {
    return { ok: false, cancelled: false, error: err?.message || String(err) }
  }
}

/**
 * Revoke a proof on-chain via the connected wallet.
 * Calls the proof_registry contract's `revoke_proof` entry point.
 */
export async function revokeProofOnChain(
  clickRef: ICSPRClickSDK,
  opts: {
    proofId: string
    senderPublicKeyHex: string
  },
): Promise<LiveTxResult> {
  try {
    const tx = new ContractCallBuilder()
      .byHash(getProofRegistryHash())
      .entryPoint('revoke_proof')
      .runtimeArgs(
        Args.fromMap({
          proof_id: CLValue.newCLString(opts.proofId),
        }),
      )
      .from(PublicKey.fromHex(opts.senderPublicKeyHex))
      .chainName(CASPER_CHAIN_NAME)
      .payment(PAYMENT_MOTES)
      .build()

    const res = await clickRef.send(tx.toJSON() as object, opts.senderPublicKeyHex)

    if (res?.transactionHash) {
      return { ok: true, transactionHash: res.transactionHash }
    }
    if (res?.cancelled) {
      return { ok: false, cancelled: true }
    }
    const rawError = (res as any)?.error ?? (res as any)?.errorData ?? 'Unknown error'
    return { ok: false, cancelled: false, error: typeof rawError === 'string' ? rawError : JSON.stringify(rawError) }
  } catch (err: any) {
    return { ok: false, cancelled: false, error: err?.message || String(err) }
  }
}
