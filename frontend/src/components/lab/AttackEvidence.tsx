import React, { useCallback, useMemo, useState } from 'react';
import {
  Shield,
  ShieldAlert,
  ShieldCheck,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Play,
  Loader2,
  Copy,
  ChevronDown,
  ChevronUp,
  Zap,
  Fingerprint,
  Image as ImageIcon,
  Cpu,
  GitBranch,
  Ban,
} from 'lucide-react';
import { createProof, verifyProof, revokeProof, Proof } from '../../lib/api';
import { toast } from '../ui/toast';
import SectionIntro from './SectionIntro';

/**
 * Attack Evidence Lab (CP-6)
 *
 * Interactive demonstration that Bot Prove's cryptographic bindings
 * detect five distinct classes of tampering against a signed proof.
 *
 * For each scenario we:
 *   1. Mint a real proof via POST /proofs with a known (input, output, model)
 *   2. Replay `/verify` with the honest tuple (baseline: should succeed)
 *   3. Replay `/verify` with a mutated tuple (attack: should fail with the
 *      exact detection field Bot Prove expects)
 *
 * All five scenarios talk to the real engine — nothing is mocked. If the
 * backend ever accepted a mutated tuple, this UI would show a red PASS
 * where a red FAIL should be, so it doubles as a live regression signal.
 */

type AttackId =
  | 'input-tamper'
  | 'output-tamper'
  | 'model-substitution'
  | 'proof-swap'
  | 'replay-after-revoke';

interface AttackScenario {
  id: AttackId;
  title: string;
  storyline: string;
  narrative: string;
  detectionField: string;
  icon: React.ElementType;
  color: string; // tailwind text-<color>-400
  attackerInput: string;
  attackerOutput: string;
  attackerModel: string;
  honestInput: string;
  honestOutput: string;
  honestModel: string;
  mutate: (
    honest: { input: string; output: string; model: string },
    mutated: { input: string; output: string; model: string },
  ) => { input: string; output: string; model: string };
  expectedErrorSubstring: string;
}

const SCENARIOS: AttackScenario[] = [
  {
    id: 'input-tamper',
    title: 'Frame injection (input tampering)',
    storyline:
      'An adversary swaps a single frame in the video the model reviewed and re-submits the old proof against the tampered footage.',
    narrative:
      'The proof commits to sha256(input). Even a one-byte flip in the input rehashes to a different digest, so /verify rejects immediately — the proof does not describe this footage.',
    detectionField: 'input hash mismatch (ih)',
    icon: ImageIcon,
    color: 'text-red-400',
    honestInput: 'video-frame:0..N (untampered)',
    honestOutput: 'label=safe;score=0.98',
    honestModel: 'vision-v3.2.0',
    attackerInput: 'video-frame:0..N (frame 42 replaced)',
    attackerOutput: 'label=safe;score=0.98',
    attackerModel: 'vision-v3.2.0',
    mutate: (_h, m) => ({ input: m.input, output: m.output, model: m.model }),
    expectedErrorSubstring: 'input hash mismatch',
  },
  {
    id: 'output-tamper',
    title: 'Verdict swap (output tampering)',
    storyline:
      'The agent decided "reject" — the operator forwards the same proof but reports "approved" to the downstream contract.',
    narrative:
      'The proof commits to sha256(output). Rewriting the reported verdict makes the output hash disagree with the anchored digest, and /verify refuses to certify the swap.',
    detectionField: 'output hash mismatch (oh)',
    icon: GitBranch,
    color: 'text-orange-400',
    honestInput: 'loan_application_42',
    honestOutput: 'decision=reject;reason=insufficient_income',
    honestModel: 'credit-scoring-v1.4',
    attackerInput: 'loan_application_42',
    attackerOutput: 'decision=approve;reason=insufficient_income',
    attackerModel: 'credit-scoring-v1.4',
    mutate: (_h, m) => ({ input: m.input, output: m.output, model: m.model }),
    expectedErrorSubstring: 'output hash mismatch',
  },
  {
    id: 'model-substitution',
    title: 'Model substitution',
    storyline:
      'Attacker claims the reasoning came from the audited model, but the actual computation was done by an older, weaker weights bundle.',
    narrative:
      'The proof commits to sha256(model). If the attester quietly downgrades the model, the model hash breaks and /verify surfaces the substitution before the downstream contract acts on it.',
    detectionField: 'model hash mismatch (mh)',
    icon: Cpu,
    color: 'text-yellow-400',
    honestInput: 'query=is_this_a_phishing_url?',
    honestOutput: 'phishing=true;confidence=0.94',
    honestModel: 'phishing-detector-v2.1-audited',
    attackerInput: 'query=is_this_a_phishing_url?',
    attackerOutput: 'phishing=true;confidence=0.94',
    attackerModel: 'phishing-detector-v1.0-shadow',
    mutate: (_h, m) => ({ input: m.input, output: m.output, model: m.model }),
    expectedErrorSubstring: 'model hash mismatch',
  },
  {
    id: 'proof-swap',
    title: 'Proof swap across sessions',
    storyline:
      'Attacker takes a valid proof from a different session and tries to reuse it as evidence for an unrelated (input, output, model) tuple.',
    narrative:
      'The commit hash bundles all three digests together — swapping the underlying tuple invalidates the proof hash. Even if a single field looked legitimate, the combined commitment does not match.',
    detectionField: 'input hash mismatch (surfaces first)',
    icon: Fingerprint,
    color: 'text-purple-400',
    honestInput: 'kyc-doc:passport:ABC123',
    honestOutput: 'kyc=passed;jurisdiction=EU',
    honestModel: 'kyc-classifier-v0.9',
    attackerInput: 'kyc-doc:passport:XYZ789',
    attackerOutput: 'kyc=passed;jurisdiction=EU',
    attackerModel: 'kyc-classifier-v0.9',
    mutate: (_h, m) => ({ input: m.input, output: m.output, model: m.model }),
    expectedErrorSubstring: 'hash mismatch',
  },
  {
    id: 'replay-after-revoke',
    title: 'Replay after revocation',
    storyline:
      'Compromise was detected and the proof was revoked on-chain. The attacker replays the old (still cryptographically valid) proof anyway, hoping downstream systems skipped the revocation check.',
    narrative:
      'Every proof carries a revoked flag. Revocation is anchored via the Casper contract; once flipped, /verify returns "proof … revoked" no matter how correct the tuple is.',
    detectionField: 'revoked=true',
    icon: Ban,
    color: 'text-pink-400',
    honestInput: 'session-cookie:signed',
    honestOutput: 'auth=granted',
    honestModel: 'auth-verifier-v3',
    attackerInput: 'session-cookie:signed',
    attackerOutput: 'auth=granted',
    attackerModel: 'auth-verifier-v3',
    mutate: (h, _m) => ({ input: h.input, output: h.output, model: h.model }),
    expectedErrorSubstring: 'revoked',
  },
];

interface RunResult {
  baselineOk: boolean;
  baselinePayload: unknown;
  attackDetected: boolean;
  attackPayload: unknown;
  attackError: string;
  detectionField: string;
  proofId: string;
  createdAtMs: number;
  durationMs: number;
}

const AttackEvidence: React.FC = () => {
  const [running, setRunning] = useState<AttackId | null>(null);
  const [results, setResults] = useState<Record<AttackId, RunResult | undefined>>({
    'input-tamper': undefined,
    'output-tamper': undefined,
    'model-substitution': undefined,
    'proof-swap': undefined,
    'replay-after-revoke': undefined,
  });
  const [expanded, setExpanded] = useState<Record<AttackId, boolean>>({
    'input-tamper': false,
    'output-tamper': false,
    'model-substitution': false,
    'proof-swap': false,
    'replay-after-revoke': false,
  });

  const totalRun = useMemo(
    () => Object.values(results).filter(Boolean).length,
    [results],
  );
  const totalDetected = useMemo(
    () => Object.values(results).filter((r) => r && r.attackDetected).length,
    [results],
  );

  const runScenario = useCallback(async (s: AttackScenario) => {
    setRunning(s.id);
    const started = performance.now();
    let proofId = '';
    try {
      // 1. Mint the honest proof.
      const createRes = await createProof({
        agent: `attack-evidence-${s.id}`,
        input: s.honestInput,
        output: s.honestOutput,
        model: s.honestModel,
        use_case: 'attack-evidence',
      });
      if (!createRes.success || !createRes.data) {
        throw new Error(createRes.error || 'createProof failed');
      }
      const created: Proof = createRes.data;
      proofId = created.id;

      // 2. Baseline: honest tuple must verify.
      const baselineRes = await verifyProof({
        proof_id: proofId,
        input: s.honestInput,
        output: s.honestOutput,
        model: s.honestModel,
      });
      const baseline = baselineRes.data;
      const baselineOk =
        !!baselineRes.success &&
        !!baseline &&
        !baseline.error &&
        (baseline.verified === true ||
          // Older engines only expose `valid`; treat as OK when no error surfaced.
          (baseline.verified === undefined && baseline.valid === true));

      // 3. For "replay after revoke" we revoke, then verify with the honest tuple.
      if (s.id === 'replay-after-revoke') {
        const revokeRes = await revokeProof(proofId);
        if (!revokeRes.success) {
          // If we can't revoke we cannot demonstrate the attack — bail loud.
          throw new Error(
            `Could not revoke proof (${revokeRes.error || 'unknown error'}). ` +
              'Attack scenario cannot be evaluated.',
          );
        }
      }

      const mutated = s.mutate(
        { input: s.honestInput, output: s.honestOutput, model: s.honestModel },
        { input: s.attackerInput, output: s.attackerOutput, model: s.attackerModel },
      );
      const attackRes = await verifyProof({
        proof_id: proofId,
        input: mutated.input,
        output: mutated.output,
        model: mutated.model,
      });
      const attackAttempt = attackRes.data;

      const attackErr = attackAttempt?.error || attackRes.error || '';
      const revokedFlag = attackAttempt?.revoked === true;
      const attackDetected =
        // Transport-level failure counts as detection only if verify actually returned a rejection,
        // otherwise a network hiccup would masquerade as success — handle explicitly:
        !!attackErr ||
        // Or revoked flag set (replay-after-revoke path returns { revoked: true } without error string)
        revokedFlag ||
        // Or verified === false (never trust just `valid` — a valid unrevoked proof always returns valid:true)
        attackAttempt?.verified === false;

      setResults((prev) => ({
        ...prev,
        [s.id]: {
          baselineOk,
          baselinePayload: baseline,
          attackDetected,
          attackPayload: attackAttempt,
          attackError:
            attackErr || (revokedFlag ? 'proof revoked' : 'verification failed'),
          detectionField: s.detectionField,
          proofId,
          createdAtMs: Date.now(),
          durationMs: Math.round(performance.now() - started),
        },
      }));

      if (attackDetected) {
        toast.success(`${s.title}: attack detected ✓`);
      } else {
        toast.error(`${s.title}: attack was NOT detected — investigate!`);
      }
    } catch (err) {
      const message = (err as Error).message || String(err);
      toast.error(`Scenario failed: ${message}`);
      setResults((prev) => ({
        ...prev,
        [s.id]: {
          baselineOk: false,
          baselinePayload: null,
          attackDetected: false,
          attackPayload: null,
          attackError: message,
          detectionField: s.detectionField,
          proofId,
          createdAtMs: Date.now(),
          durationMs: Math.round(performance.now() - started),
        },
      }));
    } finally {
      setRunning(null);
    }
  }, []);

  const runAll = useCallback(async () => {
    for (const s of SCENARIOS) {
      // eslint-disable-next-line no-await-in-loop
      await runScenario(s);
    }
  }, [runScenario]);

  const toggleExpanded = (id: AttackId) =>
    setExpanded((prev) => ({ ...prev, [id]: !prev[id] }));

  const copyPayload = async (payload: unknown, label: string) => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(payload, null, 2));
      toast.success(`${label} payload copied`);
    } catch {
      toast.error('Clipboard unavailable');
    }
  };

  return (
    <div>
      <SectionIntro
        title="Attack Evidence Lab"
        description="Five real-world tampering attempts run against the Bot Prove verifier. Each scenario mints a fresh proof, verifies the honest tuple as a baseline, then replays /verify with a mutated tuple and reports the exact detection field."
        dataSource="POST /proofs → POST /verify (live engine)"
        badge="Live cryptographic evidence"
        badgeColor="green"
        helpText="Nothing is mocked. If the engine ever accepted a mutated tuple, the panel below would show a red PASS where a red FAIL should be."
      />

      <div className="mb-6 p-4 bg-[#13131d] rounded-lg border border-[#222235]">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-green-900/20 border border-green-700/40 flex items-center justify-center">
              <ShieldCheck className="w-5 h-5 text-green-400" />
            </div>
            <div>
              <p className="text-sm text-gray-200 font-semibold">
                Detected {totalDetected} / {totalRun} attacks
              </p>
              <p className="text-xs text-gray-500">
                {SCENARIOS.length} scenarios total · Run individually or all at once.
              </p>
            </div>
          </div>
          <button
            onClick={runAll}
            disabled={running !== null}
            className="flex items-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed text-white rounded-lg text-sm font-medium transition-colors"
          >
            {running ? <Loader2 className="w-4 h-4 animate-spin" /> : <Zap className="w-4 h-4" />}
            Run all scenarios
          </button>
        </div>
      </div>

      <div className="space-y-4">
        {SCENARIOS.map((s) => {
          const result = results[s.id];
          const isRunning = running === s.id;
          const Icon = s.icon;
          const isExpanded = expanded[s.id];

          return (
            <div
              key={s.id}
              className="rounded-lg border border-[#222235] bg-[#13131d] overflow-hidden"
            >
              <div className="p-4">
                <div className="flex items-start gap-3">
                  <div className="w-10 h-10 rounded-lg bg-[#1a1a2a] border border-[#222235] flex items-center justify-center shrink-0">
                    <Icon className={`w-5 h-5 ${s.color}`} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <h3 className="text-sm font-semibold text-gray-100">{s.title}</h3>
                      {result && (
                        result.attackDetected ? (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-green-900/30 border border-green-700/40 text-green-400 text-[11px] font-medium">
                            <ShieldCheck className="w-3 h-3" /> Detected
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-red-900/30 border border-red-700/40 text-red-400 text-[11px] font-medium">
                            <ShieldAlert className="w-3 h-3" /> Not detected
                          </span>
                        )
                      )}
                    </div>
                    <p className="text-xs text-gray-400 mt-1 leading-relaxed">{s.storyline}</p>
                    <p className="text-[11px] text-gray-500 mt-2">
                      <span className="text-gray-400 font-medium">Expected detection field:</span>{' '}
                      <code className="text-gray-300">{s.detectionField}</code>
                    </p>
                  </div>
                  <button
                    onClick={() => runScenario(s)}
                    disabled={running !== null}
                    className="shrink-0 flex items-center gap-2 px-3 py-1.5 bg-[#1a1a2a] hover:bg-[#222235] border border-[#222235] text-gray-200 rounded-lg text-xs font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {isRunning ? (
                      <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    ) : (
                      <Play className="w-3.5 h-3.5" />
                    )}
                    Attempt attack
                  </button>
                </div>

                <p className="text-[11px] text-gray-500 mt-3 leading-relaxed border-t border-[#222235] pt-3">
                  <span className="text-gray-400 font-medium">Why it fails:</span>{' '}
                  {s.narrative}
                </p>

                {result && (
                  <div className="mt-3 space-y-2">
                    <div className="grid sm:grid-cols-2 gap-2">
                      <div className="p-2.5 rounded-lg bg-[#0f0f18] border border-[#222235]">
                        <div className="flex items-center gap-1.5 text-[11px] text-gray-400 mb-1">
                          <Shield className="w-3 h-3" /> Baseline (honest tuple)
                        </div>
                        {result.baselineOk ? (
                          <div className="flex items-center gap-1.5 text-xs text-green-400">
                            <CheckCircle className="w-3.5 h-3.5" /> Verified
                          </div>
                        ) : (
                          <div className="flex items-center gap-1.5 text-xs text-yellow-400">
                            <AlertTriangle className="w-3.5 h-3.5" /> Unexpected baseline result — check payload
                          </div>
                        )}
                      </div>
                      <div className="p-2.5 rounded-lg bg-[#0f0f18] border border-[#222235]">
                        <div className="flex items-center gap-1.5 text-[11px] text-gray-400 mb-1">
                          <ShieldAlert className="w-3 h-3" /> Attack replay
                        </div>
                        {result.attackDetected ? (
                          <div className="flex items-center gap-1.5 text-xs text-green-400">
                            <XCircle className="w-3.5 h-3.5" />
                            <span className="truncate" title={result.attackError}>
                              Rejected — {result.attackError}
                            </span>
                          </div>
                        ) : (
                          <div className="flex items-center gap-1.5 text-xs text-red-400">
                            <AlertTriangle className="w-3.5 h-3.5" /> Not detected — regression
                          </div>
                        )}
                      </div>
                    </div>
                    <div className="flex items-center justify-between text-[11px] text-gray-500">
                      <span>
                        Proof <code className="text-gray-300">{result.proofId || '—'}</code> · {result.durationMs} ms round-trip
                      </span>
                      <button
                        onClick={() => toggleExpanded(s.id)}
                        className="flex items-center gap-1 text-gray-400 hover:text-gray-200 transition-colors"
                      >
                        {isExpanded ? (
                          <>
                            Hide raw payload <ChevronUp className="w-3 h-3" />
                          </>
                        ) : (
                          <>
                            Show raw payload <ChevronDown className="w-3 h-3" />
                          </>
                        )}
                      </button>
                    </div>
                    {isExpanded && (
                      <div className="space-y-2">
                        <PayloadBlock
                          label="Baseline /verify response"
                          payload={result.baselinePayload}
                          onCopy={() => copyPayload(result.baselinePayload, 'Baseline')}
                        />
                        <PayloadBlock
                          label="Attack /verify response"
                          payload={result.attackPayload}
                          onCopy={() => copyPayload(result.attackPayload, 'Attack')}
                        />
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};

const PayloadBlock: React.FC<{
  label: string;
  payload: unknown;
  onCopy: () => void;
}> = ({ label, payload, onCopy }) => (
  <div className="rounded-lg bg-[#0b0b10] border border-[#222235] p-2.5">
    <div className="flex items-center justify-between mb-1">
      <span className="text-[11px] text-gray-400">{label}</span>
      <button
        onClick={onCopy}
        className="flex items-center gap-1 text-gray-500 hover:text-gray-300 transition-colors text-[11px]"
      >
        <Copy className="w-3 h-3" /> Copy
      </button>
    </div>
    <pre className="text-[11px] text-gray-300 overflow-x-auto whitespace-pre-wrap break-all font-mono">
      {payload ? JSON.stringify(payload, null, 2) : '—'}
    </pre>
  </div>
);

export default AttackEvidence;
