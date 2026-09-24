/**
 * TrustStatus — single, reusable badge component with four canonical labels.
 *
 * The four labels below are the ONLY allowed values. Every existing "trust"
 * pill on the site (currently ad-hoc pills scattered in ZK/Contracts pages
 * and elsewhere) MUST be replaced by this component, and every future badge
 * MUST reuse one of these four values.
 *
 * The wording matches the claim-freeze pass (commit 982adad, PR #14) and the
 * REAL CRYPTO / ON-CHAIN / SIMULATION boundary already documented in
 * docs/JUDGE_GUIDE.md. "Built, not deployed" is the honest counterpart for
 * source-only artifacts (e.g. contracts that exist in-repo but have no
 * on-chain address).
 *
 * Explicitly NOT allowed: "verified", "live", "production-ready", "audited",
 * "battle-tested", "trustless" — those imply a fact this component cannot
 * prove from the source manifest. Do not add new variants without a
 * corresponding, verifiable source of truth (a deployed address, a real
 * cryptographic endpoint, or an explicit "written-only" marker in a data
 * file such as CONTRACTS[].deployed === false).
 */

import React from 'react';

export type TrustStatusKind =
  | 'real-cryptography'
  | 'on-chain'
  | 'simulation'
  | 'built-not-deployed';

interface TrustStatusStyle {
  label: string;
  className: string;
  title: string;
}

const STYLES: Record<TrustStatusKind, TrustStatusStyle> = {
  'real-cryptography': {
    label: 'Real cryptography',
    className:
      'bg-red-600/20 text-red-300 border-red-500/40',
    title:
      'Runs a real cryptographic primitive in the Bot Prove engine (e.g. Groth16 BN254 via gnark, ML-DSA-65 via Cloudflare CIRCL). Executes off-chain in this repo.',
  },
  'on-chain': {
    label: 'On-chain',
    className:
      'bg-green-600/20 text-green-300 border-green-500/40',
    title:
      'Deployed on BOT Chain testnet. A real contract address exists and the code path anchors to that address.',
  },
  simulation: {
    label: 'Simulation',
    className:
      'bg-yellow-600/20 text-yellow-300 border-yellow-500/40',
    title:
      'Illustrative or hash-based simulation. Not a real cryptographic verification and not anchored on-chain.',
  },
  'built-not-deployed': {
    label: 'Built, not deployed',
    className:
      'bg-slate-600/20 text-slate-300 border-slate-500/40',
    title:
      'Source code exists in this repository but the contract has no on-chain address. Written and reviewable, not yet deployed.',
  },
};

export interface TrustStatusProps {
  kind: TrustStatusKind;
  /** Optional extra className to control spacing/alignment; visual style is fixed. */
  className?: string;
  /** Optional override for the accessible tooltip; defaults to the canonical description. */
  title?: string;
}

/** Small pill badge. Do not restyle inline — the four variants are canonical. */
const TrustStatus: React.FC<TrustStatusProps> = ({ kind, className = '', title }) => {
  const style = STYLES[kind];
  return (
    <span
      role="status"
      aria-label={style.label}
      title={title ?? style.title}
      data-trust-status={kind}
      className={`inline-flex items-center px-2 py-0.5 rounded text-[10px] font-bold tracking-wide border whitespace-nowrap ${style.className} ${className}`}
    >
      {style.label}
    </span>
  );
};

export default TrustStatus;
