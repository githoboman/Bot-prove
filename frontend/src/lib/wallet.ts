/**
 * Casper Wallet integration via CSPR.click SDK.
 *
 * Uses the official CSPR.click (cspr.click) platform for wallet connection,
 * supporting Casper Wallet, Ledger, MetaMask Snap, and WalletConnect.
 *
 * Keys:
 *   AppID: configured via CSPR.click dashboard for casperprover.xyz
 *   CSPR.cloud: API access for balance queries etc.
 */

export const CSPR_CLICK_APP_ID = '9584b117-384b-4433-aaba-c6407a06'
export const CSPR_CLOUD_API_KEY = '019f3d52-8d25-79c9-922f-f072df32d62c'

export interface WalletState {
  connected: boolean
  publicKey: string | null
  accountHash: string | null
  provider: string | null
}

export function shortKey(key: string): string {
  if (key.length <= 16) return key
  return key.slice(0, 8) + '...' + key.slice(-6)
}
