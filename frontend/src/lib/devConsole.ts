// Devtools console info panel.
//
// Prints a compact, judge/engineer-friendly summary on load: which
// contracts are live, the API health snapshot, and links to the repo/docs.
// Read-only — fetches the same public /onchain.json and /health endpoints
// the UI already uses, no secrets, no state mutation.
import { loadManifest } from './onchain';

const BASE_URL = '/prover-api';

async function fetchHealth(): Promise<Record<string, unknown> | null> {
  try {
    const res = await fetch(`${BASE_URL}/health`);
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}

export function installDevConsoleInfo(): void {
  if (typeof window === 'undefined' || !window.console) return;

  console.log(
    '%cBot Prove%c — real Groth16 ZK proofs anchored on BOT Chain testnet',
    'color:#ef4444;font-weight:bold;font-size:14px',
    'color:#9ca3af'
  );
  console.log(
    '%cGitHub: %chttps://github.com/githoboman/Bot-prove  %c|  Docs: %c/docs/api',
    'color:#6b7280', 'color:#0ea5e9', 'color:#6b7280', 'color:#0ea5e9'
  );

  void (async () => {
    const [manifest, health] = await Promise.all([
      loadManifest().catch(() => null),
      fetchHealth(),
    ]);

    if (manifest) {
      const rows = Object.entries(manifest.contracts).map(([key, c]) => ({
        contract: key,
        hash: `${c.contract_hash.slice(0, 12)}…`,
        explorer: `https://scan.botchain.ai/contract/${c.contract_hash}`,
      }));
      console.log(`%c${rows.length} contracts live on ${manifest.network}:`, 'color:#22c55e;font-weight:bold');
      console.table(rows);
    }

    if (health) {
      console.log('%capi /health:', 'color:#22c55e;font-weight:bold', health);
    } else {
      console.log('%capi /health: unreachable (backend may be cold-starting on Render free tier)', 'color:#f59e0b');
    }

    console.log(
      '%cPoking around? cp/JUDGE_GUIDE.md in the repo walks through every endpoint with copy-pasteable curl commands. If you need the judge API key for authenticated writes, ping the team directly — it is intentionally not shipped in this bundle.',
      'color:#6b7280;font-style:italic'
    );
  })();
}
