/**
 * TrustBadge — shared honest-claims label.
 *
 * Rules (backlog 3.7 alignment):
 *   REAL         — computation actually performed by the runtime
 *                  (e.g. real Groth16 pairing check, real SHA-256 hash).
 *   ON-CHAIN     — result was written to BOT Chain and can be
 *                  independently re-fetched from the RPC.
 *   SIMULATION   — the response is a plausible mock produced without
 *                  invoking the underlying primitive; kept for
 *                  demo/UX continuity but MUST be marked.
 *
 * The badge is deliberately loud (colored border + uppercase text).
 * Every judge-facing view that renders a decision, proof, or hash
 * must place one of these labels next to the value.
 */

import type { CSSProperties } from 'react'

export type TrustKind = 'real' | 'on-chain' | 'simulation'

const palette: Record<TrustKind, { bg: string; fg: string; border: string; label: string }> = {
  'real':       { bg: '#e8f8ee', fg: '#0d5c2b', border: '#2ea15b', label: 'REAL' },
  'on-chain':   { bg: '#e6f0ff', fg: '#0a2c73', border: '#375ea2', label: 'ON-CHAIN' },
  'simulation': { bg: '#fff2d6', fg: '#7a4a00', border: '#c8912b', label: 'SIMULATION' },
}

export default function TrustBadge({
  kind,
  title,
  compact,
}: {
  kind: TrustKind
  title?: string
  compact?: boolean
}) {
  const p = palette[kind]
  const style: CSSProperties = {
    display: 'inline-block',
    padding: compact ? '1px 6px' : '2px 8px',
    fontSize: compact ? 10 : 11,
    fontWeight: 700,
    letterSpacing: '0.05em',
    lineHeight: 1.4,
    color: p.fg,
    background: p.bg,
    border: `1px solid ${p.border}`,
    borderRadius: 4,
    fontFamily: 'system-ui, -apple-system, sans-serif',
    verticalAlign: 'middle',
    marginLeft: 4,
  }
  return (
    <span style={style} title={title ?? p.label}>
      {p.label}
    </span>
  )
}
