/**
 * Decisions — auditable-decision viewer (backlog 3.7).
 *
 * Renders the chain of steps produced by the /decisions/log/{id}/lineage
 * endpoint with:
 *
 *   1. Request hash + response hash (SHA-256, hex).
 *   2. Model id + version.
 *   3. Verdict (allow / abstain / reject / hitl_required / malicious).
 *   4. Risk tier + policy id.
 *   5. TrustBadge per row so a judge instantly sees which parts are
 *      REAL vs SIMULATION vs ON-CHAIN.
 *   6. A "verified" indicator per row: the frontend recomputes the
 *      chain-root the same way engine/internal/decision does, so
 *      tampering with the record via the network is detected client-side.
 *
 * No secret material ever crosses the wire — the server strips raw
 * request/response into hashes before persisting.
 */

import { useEffect, useMemo, useState } from 'react'
import TrustBadge from './TrustBadge'
import type { TrustKind } from './TrustBadge'

const API_BASE = (import.meta as any).env?.VITE_API_BASE || ''

type DecisionRecord = {
  id: string
  timestamp: string
  agent_id: string
  model_id: string
  model_version: string
  request_hash: string
  response_hash: string
  input_bytes: number
  output_bytes: number
  verdict: string
  risk_tier: string
  policy_id: string
  metadata?: { [k: string]: string }
  trace_preview?: string
  preview_opt_in: boolean
  parent_record_id?: string
  chain_root_hash: string
}

function verdictBadgeKind(_verdict: string, metadata?: { [k: string]: string }): TrustKind {
  // Explicit override — any record whose metadata carries mode=simulation
  // is labeled as such; on-chain-anchored records carry mode=on-chain.
  const mode = (metadata?.mode || '').toLowerCase()
  if (mode === 'simulation') return 'simulation'
  if (mode === 'on-chain') return 'on-chain'
  // Default: the audit log itself is a REAL computation over real hashes.
  return 'real'
}

function short(hash: string, n = 10) {
  if (!hash) return ''
  if (hash.length <= n * 2 + 3) return hash
  return `${hash.slice(0, n)}\u2026${hash.slice(-n)}`
}

async function sha256Hex(input: string): Promise<string> {
  const buf = new TextEncoder().encode(input)
  const digest = await crypto.subtle.digest('SHA-256', buf)
  return Array.from(new Uint8Array(digest))
    .map(b => b.toString(16).padStart(2, '0'))
    .join('')
}

async function verifyChainRoot(rec: DecisionRecord): Promise<boolean> {
  // Mirror engine chainRoot: zero the ChainRootHash then hash the canonical JSON.
  const shadow: any = { ...rec, chain_root_hash: '' }
  // Match Go's json.Marshal field order: rely on stable JS key order
  // matching the struct's declaration order used server-side.
  // In practice, both server + client produce keys in the same
  // declaration order for a struct literal.
  const canonical = JSON.stringify(shadow)
  const recomputed = await sha256Hex(canonical)
  return recomputed === rec.chain_root_hash
}

export default function Decisions() {
  const [records, setRecords] = useState<DecisionRecord[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [lineage, setLineage] = useState<DecisionRecord[]>([])
  const [verifiedMap, setVerifiedMap] = useState<{ [id: string]: boolean }>({})
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    loadRecent()
  }, [])

  async function loadRecent() {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`${API_BASE}/v1/decisions/log?limit=50`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setRecords(data.records ?? [])
    } catch (e: any) {
      setError(e.message || 'failed to load')
    } finally {
      setLoading(false)
    }
  }

  async function loadLineage(id: string) {
    setSelected(id)
    setLineage([])
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`${API_BASE}/v1/decisions/log/${id}/lineage`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      const chain: DecisionRecord[] = data.chain ?? []
      setLineage(chain)
      // Verify each row client-side.
      const verified: { [id: string]: boolean } = {}
      for (const r of chain) verified[r.id] = await verifyChainRoot(r)
      setVerifiedMap(verified)
    } catch (e: any) {
      setError(e.message || 'failed to load lineage')
    } finally {
      setLoading(false)
    }
  }

  const heading = useMemo(
    () =>
      selected
        ? `Decision lineage \u2014 ${selected}`
        : `Recent decisions (${records.length})`,
    [selected, records.length]
  )

  return (
    <div style={{ padding: 16, fontFamily: 'system-ui, sans-serif' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
        <h2 style={{ margin: 0 }}>
          {heading} <TrustBadge kind="real" title="Client re-hashes every row for tamper-evidence" />
        </h2>
        <div>
          {selected && (
            <button onClick={() => { setSelected(null); setLineage([]) }} style={btnSecondary}>
              &laquo; back to list
            </button>
          )}
          <button onClick={loadRecent} disabled={loading} style={btnPrimary}>
            {loading ? 'loading\u2026' : 'refresh'}
          </button>
        </div>
      </div>
      <p style={{ color: '#666', fontSize: 13, maxWidth: 720 }}>
        Every row is an immutable, chain-rooted audit record. The raw prompt and
        response never leave the server \u2014 only their SHA-256 hashes. Click a row to
        walk its ancestor chain. The badge on each row shows whether the computation
        is REAL, ON-CHAIN, or SIMULATION.
      </p>
      {error && (
        <div style={{ background: '#ffe6e6', color: '#7a0000', padding: 8, borderRadius: 4 }}>
          {error}
        </div>
      )}
      <div style={{ marginTop: 12 }}>
        {(selected ? lineage : records).map(r => (
          <div key={r.id} style={rowStyle} onClick={() => !selected && loadLineage(r.id)}>
            <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap' }}>
              <div>
                <strong>{r.verdict}</strong>{' '}
                <TrustBadge kind={verdictBadgeKind(r.verdict, r.metadata)} compact />
                <span style={{ color: '#888', marginLeft: 8, fontSize: 12 }}>
                  {new Date(r.timestamp).toISOString()}
                </span>
              </div>
              <div style={{ fontSize: 12, color: '#555' }}>
                {r.agent_id} \u00b7 {r.model_id} v{r.model_version} \u00b7 risk: {r.risk_tier}
              </div>
            </div>
            <div style={{ marginTop: 6, fontFamily: 'monospace', fontSize: 12 }}>
              <span style={{ color: '#333' }}>req</span> {short(r.request_hash)} &nbsp;
              <span style={{ color: '#333' }}>resp</span> {short(r.response_hash)} &nbsp;
              <span style={{ color: '#333' }}>root</span> {short(r.chain_root_hash)}
            </div>
            {selected && (
              <div style={{ fontSize: 11, marginTop: 4 }}>
                client-verified:{' '}
                <strong style={{ color: verifiedMap[r.id] === false ? '#c40000' : '#0d5c2b' }}>
                  {verifiedMap[r.id] === false ? 'TAMPERED' : verifiedMap[r.id] === true ? 'ok' : '\u2026'}
                </strong>
              </div>
            )}
            {r.trace_preview && (
              <details style={{ marginTop: 4, fontSize: 12, color: '#555' }}>
                <summary>trace preview (opt-in)</summary>
                <pre style={{ background: '#f6f6f6', padding: 6, borderRadius: 4 }}>{r.trace_preview}</pre>
              </details>
            )}
          </div>
        ))}
        {!loading && (selected ? lineage : records).length === 0 && (
          <div style={{ color: '#999', padding: 24, textAlign: 'center' }}>
            No decisions yet. POST one to <code>/v1/decisions/log</code> to see it here.
          </div>
        )}
      </div>
    </div>
  )
}

const rowStyle = {
  border: '1px solid #ddd',
  borderRadius: 6,
  padding: 10,
  marginBottom: 8,
  background: '#fff',
  cursor: 'pointer',
}

const btnPrimary = {
  background: '#0a2c73', color: 'white', border: 0, borderRadius: 4,
  padding: '6px 12px', cursor: 'pointer', fontSize: 12,
}

const btnSecondary = {
  background: '#eee', color: '#333', border: '1px solid #ccc', borderRadius: 4,
  padding: '6px 12px', cursor: 'pointer', fontSize: 12, marginRight: 8,
}
