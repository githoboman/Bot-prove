import React, { useState, useCallback } from 'react';
import { CheckCircle, XCircle, Loader2, Search, ExternalLink } from 'lucide-react';
import { verifyProof, VerifyProofResponse } from '../lib/api';

/**
 * Judge dashboard — a single, dependency-light static page for hackathon
 * judges to independently verify a proof against the live API without
 * touching any wallet flow or the rest of the lab UI.
 *
 * Route: /judge (see App.tsx). Deliberately outside the /lab shell so it
 * renders standalone, fast, and with zero wallet/CSPR.click bootstrap.
 */

function truncHash(h: string, len = 12): string {
  if (!h || h.length <= len * 2) return h || '—';
  return h.slice(0, len) + '...' + h.slice(-6);
}

const JudgeDashboard: React.FC = () => {
  const [proofId, setProofId] = useState('');
  const [input, setInput] = useState('');
  const [output, setOutput] = useState('');
  const [model, setModel] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<VerifyProofResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleVerify = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!proofId.trim()) {
        setError('Proof ID is required.');
        return;
      }
      setLoading(true);
      setError(null);
      setResult(null);
      try {
        const res = await verifyProof({
          proof_id: proofId.trim(),
          ...(input.trim() ? { input: input.trim() } : {}),
          ...(output.trim() ? { output: output.trim() } : {}),
          ...(model.trim() ? { model: model.trim() } : {}),
        });
        if (res.success && res.data) {
          setResult(res.data);
        } else {
          setError(res.error || res.message || 'Verification failed.');
        }
      } catch (err: any) {
        setError(err?.message || 'Verification request failed.');
      } finally {
        setLoading(false);
      }
    },
    [proofId, input, output, model],
  );

  const statusColor = result
    ? result.revoked
      ? 'text-red-400 border-red-700/40 bg-red-900/20'
      : result.valid
        ? 'text-green-400 border-green-700/40 bg-green-900/20'
        : 'text-red-400 border-red-700/40 bg-red-900/20'
    : '';

  const statusLabel = result
    ? result.revoked
      ? 'REVOKED'
      : result.valid
        ? 'VALID'
        : 'INVALID'
    : '';

  return (
    <div className="min-h-screen bg-[#0a0a12] text-gray-200 flex flex-col">
      <header className="border-b border-[#222235] px-6 py-4 flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-gray-100">Bot Prove — Judge Dashboard</h1>
          <p className="text-xs text-gray-500 mt-0.5">
            Independently verify any proof against the live API. No wallet required.
          </p>
        </div>
        <a
          href="/"
          className="text-xs text-gray-400 hover:text-gray-200 flex items-center gap-1"
        >
          Back to site <ExternalLink className="w-3 h-3" />
        </a>
      </header>

      <main className="flex-1 max-w-2xl w-full mx-auto px-6 py-10 space-y-8">
        <div className="p-4 bg-[#1a1a2a] rounded-lg border border-[#222235] text-xs text-gray-400">
          Enter a <span className="text-gray-200 font-medium">proof_id</span> (required) and,
          optionally, the original <span className="text-gray-200 font-medium">input</span>,{' '}
          <span className="text-gray-200 font-medium">output</span>, and{' '}
          <span className="text-gray-200 font-medium">model</span> to re-check hash consistency.
          This calls the same <code className="text-gray-300">POST /verify</code> endpoint the SDK
          and on-chain flows use.
        </div>

        <form onSubmit={handleVerify} className="space-y-4">
          <div>
            <label htmlFor="proof_id" className="block text-sm font-medium text-gray-300 mb-1">
              Proof ID <span className="text-red-400">*</span>
            </label>
            <input
              id="proof_id"
              value={proofId}
              onChange={(e) => setProofId(e.target.value)}
              placeholder="e.g. proof_8f2a..."
              className="w-full bg-[#13131d] border border-[#222235] rounded-md px-3 py-2 text-sm text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>

          <div className="grid grid-cols-1 gap-4">
            <div>
              <label htmlFor="input" className="block text-sm font-medium text-gray-300 mb-1">
                Input (optional)
              </label>
              <textarea
                id="input"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                rows={2}
                className="w-full bg-[#13131d] border border-[#222235] rounded-md px-3 py-2 text-sm text-gray-100 focus:outline-none focus:border-blue-500"
              />
            </div>
            <div>
              <label htmlFor="output" className="block text-sm font-medium text-gray-300 mb-1">
                Output (optional)
              </label>
              <textarea
                id="output"
                value={output}
                onChange={(e) => setOutput(e.target.value)}
                rows={2}
                className="w-full bg-[#13131d] border border-[#222235] rounded-md px-3 py-2 text-sm text-gray-100 focus:outline-none focus:border-blue-500"
              />
            </div>
            <div>
              <label htmlFor="model" className="block text-sm font-medium text-gray-300 mb-1">
                Model (optional)
              </label>
              <input
                id="model"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder="e.g. gpt-4o"
                className="w-full bg-[#13131d] border border-[#222235] rounded-md px-3 py-2 text-sm text-gray-100 focus:outline-none focus:border-blue-500"
              />
            </div>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="w-full flex items-center justify-center gap-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white font-medium py-2.5 rounded-md transition-colors"
          >
            {loading ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" /> Verifying…
              </>
            ) : (
              <>
                <Search className="w-4 h-4" /> Verify Proof
              </>
            )}
          </button>
        </form>

        {error && (
          <div className="p-4 rounded-lg border border-red-700/40 bg-red-900/20 text-red-400 text-sm flex items-start gap-2">
            <XCircle className="w-4 h-4 mt-0.5 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {result && (
          <div className={`p-5 rounded-lg border ${statusColor} space-y-3`}>
            <div className="flex items-center gap-2">
              {result.valid && !result.revoked ? (
                <CheckCircle className="w-5 h-5" />
              ) : (
                <XCircle className="w-5 h-5" />
              )}
              <span className="font-semibold text-base">{statusLabel}</span>
            </div>
            <dl className="text-xs space-y-1.5 text-gray-300">
              <div className="flex justify-between gap-4">
                <dt className="text-gray-500">Proof ID</dt>
                <dd className="font-mono">{truncHash(result.proof_id)}</dd>
              </div>
              {result.error && (
                <div className="flex justify-between gap-4">
                  <dt className="text-gray-500">Error</dt>
                  <dd className="text-right">{result.error}</dd>
                </div>
              )}
              {result.checks && (
                <div className="pt-2 border-t border-white/10 space-y-1">
                  {Object.entries(result.checks).map(([k, v]) => (
                    <div key={k} className="flex justify-between gap-4">
                      <dt className="text-gray-500">{k}</dt>
                      <dd className="flex items-center gap-1">
                        {v ? (
                          <CheckCircle className="w-3.5 h-3.5 text-green-400" />
                        ) : (
                          <XCircle className="w-3.5 h-3.5 text-red-400" />
                        )}
                      </dd>
                    </div>
                  ))}
                </div>
              )}
            </dl>
          </div>
        )}
      </main>

      <footer className="border-t border-[#222235] px-6 py-4 text-center text-[11px] text-gray-500">
        Bot Prove Judge Dashboard · calls the live verification API directly.
      </footer>
    </div>
  );
};

export default JudgeDashboard;
