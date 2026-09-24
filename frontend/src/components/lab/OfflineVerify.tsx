/**
 * OfflineVerify — client-side receipt / decision-record verifier
 * (backlog 9.4).
 *
 * Judges and integrators paste a receipt JSON (or a decision record
 * as returned by /v1/decisions/log/{id}) into a textarea. The page:
 *
 *   1. Recomputes the SHA-256 chain root the same way
 *      engine/internal/decision does.
 *   2. Recomputes the request/response hashes against optional
 *      pasted raw request/response.
 *   3. Reports every mismatch inline with red flags.
 *
 * Fully offline. Uses the SubtleCrypto Web API only. Works when
 * disconnected (PWA cache serves the app shell + this route).
 */

import { useState } from 'react'
import TrustBadge from './TrustBadge'

type AnyRecord = { [k: string]: any }

async function sha256Hex(input: string): Promise<string> {
  const buf = new TextEncoder().encode(input)
  const digest = await crypto.subtle.digest('SHA-256', buf)
  return Array.from(new Uint8Array(digest))
    .map(b => b.toString(16).padStart(2, '0'))
    .join('')
}

async function verifyChainRoot(rec: AnyRecord): Promise<{ ok: boolean; recomputed: string }> {
  const shadow = { ...rec, chain_root_hash: '' }
  const recomputed = await sha256Hex(JSON.stringify(shadow))
  return { ok: recomputed === rec.chain_root_hash, recomputed }
}

async function verifyRequestResponse(
  rec: AnyRecord, rawReq: string, rawResp: string
): Promise<{ reqMatch: boolean | null; respMatch: boolean | null; reqHash: string; respHash: string }> {
  const out = { reqMatch: null as boolean | null, respMatch: null as boolean | null, reqHash: '', respHash: '' }
  if (rawReq) {
    out.reqHash = await sha256Hex(rawReq)
    out.reqMatch = out.reqHash === rec.request_hash
  }
  if (rawResp) {
    out.respHash = await sha256Hex(rawResp)
    out.respMatch = out.respHash === rec.response_hash
  }
  return out
}

export default function OfflineVerify() {
  const [pasted, setPasted] = useState('')
  const [rawReq, setRawReq] = useState('')
  const [rawResp, setRawResp] = useState('')
  const [parseError, setParseError] = useState<string | null>(null)
  const [result, setResult] = useState<any>(null)
  const [busy, setBusy] = useState(false)

  async function verify() {
    setParseError(null)
    setResult(null)
    let rec: AnyRecord
    try {
      const parsed = JSON.parse(pasted)
      // Accept either {record: {...}} envelope or bare record.
      rec = parsed.record ?? parsed
    } catch (e: any) {
      setParseError('Invalid JSON: ' + (e.message || e))
      return
    }
    setBusy(true)
    try {
      const chain = await verifyChainRoot(rec)
      const rr = await verifyRequestResponse(rec, rawReq, rawResp)
      setResult({ rec, chain, rr })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ padding: 16, maxWidth: 900, fontFamily: 'system-ui, sans-serif' }}>
      <h2 style={{ margin: 0 }}>
        Offline verify <TrustBadge kind="real" title="Client-side SHA-256; works with no network" />
      </h2>
      <p style={{ color: '#555', fontSize: 13 }}>
        Paste a decision record or receipt below. The page recomputes the SHA-256 chain
        root and any request/response hashes locally in your browser \u2014 no server round-trip.
        Add the raw request/response if you have them to prove hash match.
      </p>

      <label style={label}>Decision record / receipt JSON</label>
      <textarea
        value={pasted}
        onChange={e => setPasted(e.target.value)}
        rows={10}
        style={ta}
        placeholder='{"id":"dec-...", "request_hash":"...", "chain_root_hash":"...", ...}'
      />

      <label style={label}>Raw request (optional)</label>
      <textarea
        value={rawReq}
        onChange={e => setRawReq(e.target.value)}
        rows={3}
        style={ta}
      />

      <label style={label}>Raw response (optional)</label>
      <textarea
        value={rawResp}
        onChange={e => setRawResp(e.target.value)}
        rows={3}
        style={ta}
      />

      <div style={{ marginTop: 8 }}>
        <button onClick={verify} disabled={busy || !pasted} style={btn}>
          {busy ? 'verifying\u2026' : 'verify offline'}
        </button>
      </div>

      {parseError && (
        <div style={errBox}>{parseError}</div>
      )}

      {result && (
        <div style={{ marginTop: 16 }}>
          <h3 style={{ marginBottom: 8 }}>
            Result{' '}
            <TrustBadge kind={result.chain.ok ? 'real' : 'simulation'} compact />
          </h3>
          <div style={row}>
            <strong>Chain root:</strong>{' '}
            <span style={{ color: result.chain.ok ? '#0d5c2b' : '#c40000' }}>
              {result.chain.ok ? 'OK' : 'TAMPERED'}
            </span>
            <div style={mono}>
              expected: {result.rec.chain_root_hash}
              <br />
              got:      {result.chain.recomputed}
            </div>
          </div>

          {result.rr.reqMatch !== null && (
            <div style={row}>
              <strong>Request hash:</strong>{' '}
              <span style={{ color: result.rr.reqMatch ? '#0d5c2b' : '#c40000' }}>
                {result.rr.reqMatch ? 'MATCH' : 'MISMATCH'}
              </span>
              <div style={mono}>
                expected: {result.rec.request_hash}
                <br />
                got:      {result.rr.reqHash}
              </div>
            </div>
          )}

          {result.rr.respMatch !== null && (
            <div style={row}>
              <strong>Response hash:</strong>{' '}
              <span style={{ color: result.rr.respMatch ? '#0d5c2b' : '#c40000' }}>
                {result.rr.respMatch ? 'MATCH' : 'MISMATCH'}
              </span>
              <div style={mono}>
                expected: {result.rec.response_hash}
                <br />
                got:      {result.rr.respHash}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

const label = { display: 'block', marginTop: 12, marginBottom: 4, fontSize: 13, color: '#333' }
const ta = { width: '100%', fontFamily: 'monospace', fontSize: 12, padding: 8, boxSizing: 'border-box' as const, borderRadius: 4, border: '1px solid #ccc' }
const btn = { background: '#0a2c73', color: 'white', border: 0, borderRadius: 4, padding: '8px 16px', cursor: 'pointer', fontSize: 13 }
const errBox = { marginTop: 12, background: '#ffe6e6', color: '#7a0000', padding: 10, borderRadius: 4 }
const row = { border: '1px solid #ddd', borderRadius: 6, padding: 10, marginBottom: 8 }
const mono = { fontFamily: 'monospace', fontSize: 11, marginTop: 4, color: '#555', whiteSpace: 'pre' as const }
