#!/usr/bin/env node
// Reconcile mass-runner reports against cspr.live API to find real execution results.
// Reads all mass-runner-*.jsonl in /data/cp/repo/reports and produces a summary.

import fs from "node:fs";
import path from "node:path";

const REPORT_DIR = "/data/cp/repo/reports";
const FILES = fs.readdirSync(REPORT_DIR).filter(f => f.startsWith("mass-runner") && f.endsWith(".jsonl"));
console.log("[reconcile] files:", FILES);

// Gather all send events (with tx_hash)
const sends = [];
for (const f of FILES) {
  const lines = fs.readFileSync(path.join(REPORT_DIR, f), "utf8").split("\n").filter(Boolean);
  for (const l of lines) {
    const r = JSON.parse(l);
    if (r.tx_hash && !r.final) {
      // Deduplicate: keep first send with a given tx_hash
      sends.push({ ...r, source_file: f });
    }
  }
}
// Dedup by tx_hash
const seen = new Set();
const uniqSends = sends.filter(s => {
  const h = String(s.tx_hash).replace(/^"|"$/g, "");
  if (seen.has(h)) return false;
  seen.add(h);
  s.tx_hash = h;
  return true;
});
console.log(`[reconcile] unique sends: ${uniqSends.length}`);

async function fetchStatus(hash) {
  const url = `https://api.testnet.cspr.live/deploys/${hash}`;
  const res = await fetch(url);
  if (!res.ok) return { hash, found: false, http: res.status };
  const j = await res.json();
  if (!j.data) return { hash, found: false };
  const d = j.data;
  return {
    hash,
    found: true,
    block_hash: d.block_hash,
    block_height: d.block_height,
    entry_point: d.entry_point?.name || d.entry_point,
    contract_hash: d.contract_hash,
    error: d.error_message ?? null,
    cost_motes: d.cost, // motes consumed (payment)
    consumed_motes: d.consumed, // actual consumed
    caller: d.caller_public_key,
    status: d.status,
  };
}

async function main() {
  const out = [];
  let idx = 0;
  const CONC = 8;
  const queue = [...uniqSends];
  async function worker() {
    while (queue.length) {
      const s = queue.shift();
      const st = await fetchStatus(s.tx_hash);
      out.push({ ...s, cspr_live: st });
      idx++;
      if (idx % 50 === 0) console.log(`[reconcile] ${idx}/${uniqSends.length}`);
    }
  }
  await Promise.all(Array.from({length: CONC}, worker));

  // Aggregate
  const perContract = {};
  let totalOk = 0, totalErr = 0, totalMissing = 0, totalGas = 0n;
  for (const r of out) {
    const c = r.contract;
    if (!perContract[c]) perContract[c] = { total: 0, ok: 0, err: 0, missing: 0, gas: 0n, entrypoints: {} };
    perContract[c].total++;
    if (!r.cspr_live.found) {
      perContract[c].missing++;
      totalMissing++;
      continue;
    }
    const ep = r.entryPoint;
    if (!perContract[c].entrypoints[ep]) perContract[c].entrypoints[ep] = { ok: 0, err: 0, gas: 0n };
    if (r.cspr_live.error) {
      perContract[c].err++;
      perContract[c].entrypoints[ep].err++;
      totalErr++;
    } else {
      perContract[c].ok++;
      perContract[c].entrypoints[ep].ok++;
      totalOk++;
    }
    const g = BigInt(r.cspr_live.cost_motes || r.cspr_live.cost || 0);
    perContract[c].gas += g;
    perContract[c].entrypoints[ep].gas += g;
    totalGas += g;
  }

  const ts = new Date().toISOString().replace(/[:.]/g, "-");
  const REPORT = path.join(REPORT_DIR, `reconciled-${ts}.jsonl`);
  fs.writeFileSync(REPORT, out.map(r => JSON.stringify(r)).join("\n") + "\n");

  const SUMMARY = path.join(REPORT_DIR, `mass-runner-final-summary.md`);
  let md = `# Mass runner final report — ${new Date().toISOString()}\n\n`;
  md += `Reconciled ${uniqSends.length} transactions against testnet.cspr.live.\n\n`;
  md += `## Totals\n\n`;
  md += `- **Total sent**: ${uniqSends.length}\n`;
  md += `- **Succeeded on-chain**: ${totalOk}\n`;
  md += `- **Errored on-chain**: ${totalErr}\n`;
  md += `- **Not found in explorer (finality pending or rejected)**: ${totalMissing}\n`;
  md += `- **Total gas billed**: ${(Number(totalGas) / 1e9).toFixed(4)} CSPR\n\n`;
  md += `## Per-contract\n\n`;
  md += `| Contract | Total | Ok | Err | Missing | Gas (CSPR) | Avg (CSPR) |\n`;
  md += `|---|---|---|---|---|---|---|\n`;
  for (const [c, v] of Object.entries(perContract)) {
    const gasC = Number(v.gas) / 1e9;
    md += `| ${c} | ${v.total} | ${v.ok} | ${v.err} | ${v.missing} | ${gasC.toFixed(3)} | ${(gasC / Math.max(v.ok, 1)).toFixed(4)} |\n`;
  }
  md += `\n## Per-entrypoint\n\n`;
  md += `| Contract | Entry point | Ok | Err | Gas (CSPR) | Avg (CSPR) |\n`;
  md += `|---|---|---|---|---|---|\n`;
  for (const [c, v] of Object.entries(perContract)) {
    for (const [ep, e] of Object.entries(v.entrypoints)) {
      const gasE = Number(e.gas) / 1e9;
      md += `| ${c} | ${ep} | ${e.ok} | ${e.err} | ${gasE.toFixed(3)} | ${(gasE / Math.max(e.ok + e.err, 1)).toFixed(4)} |\n`;
    }
  }

  md += `\n## Signers\n\n`;
  const signerAgg = {};
  for (const r of out) {
    const s = r.signer;
    if (!signerAgg[s]) signerAgg[s] = { total: 0, ok: 0, err: 0, missing: 0, gas: 0n };
    signerAgg[s].total++;
    if (!r.cspr_live.found) signerAgg[s].missing++;
    else if (r.cspr_live.error) signerAgg[s].err++;
    else signerAgg[s].ok++;
    signerAgg[s].gas += BigInt(r.cspr_live.cost_motes || r.cspr_live.cost || 0);
  }
  md += `| Signer | Total | Ok | Err | Missing | Gas (CSPR) |\n|---|---|---|---|---|---|\n`;
  for (const [s, v] of Object.entries(signerAgg)) {
    md += `| ${s} | ${v.total} | ${v.ok} | ${v.err} | ${v.missing} | ${(Number(v.gas) / 1e9).toFixed(4)} |\n`;
  }

  md += `\n## Raw data\n\n- Reconciled per-tx: ${REPORT}\n- Source logs: ${FILES.map(f => "/data/cp/repo/reports/" + f).join(", ")}\n`;

  fs.writeFileSync(SUMMARY, md);
  console.log("\n" + md);
  console.log(`\n[reconcile] summary: ${SUMMARY}`);
  console.log(`[reconcile] per-tx:   ${REPORT}`);
}
main().catch(e => { console.error(e); process.exit(1); });
