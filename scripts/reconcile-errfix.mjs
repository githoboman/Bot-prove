#!/usr/bin/env node
// Reconcile only the errfix run against cspr.live.
import fs from "node:fs";
import path from "node:path";

const REPORT_DIR = "/data/cp/repo/reports";
const F = process.argv[2] || fs.readdirSync(REPORT_DIR).filter(f => f.startsWith("mass-runner-errfix-") && f.endsWith(".jsonl")).sort().pop();
console.log("[reconcile] file:", F);
const lines = fs.readFileSync(path.join(REPORT_DIR, F), "utf8").split("\n").filter(Boolean);
const sends = lines.map(l => JSON.parse(l)).filter(r => r.tx_hash);
console.log(`[reconcile] tx count: ${sends.length}`);

async function fetchOne(hash) {
  const url = `https://api.testnet.cspr.live/deploys/${hash}`;
  try {
    const res = await fetch(url);
    if (!res.ok) return { hash, found: false, http: res.status };
    const j = await res.json();
    const d = j.data;
    if (!d) return { hash, found: false };
    return {
      hash,
      found: true,
      block_height: d.block_height,
      contract_hash: d.contract_hash,
      error: d.error_message ?? null,
      cost_motes: d.cost,
      status: d.status,
    };
  } catch (e) {
    return { hash, found: false, err: e.message };
  }
}

const results = [];
for (let i = 0; i < sends.length; i++) {
  const s = sends[i];
  let r = await fetchOne(s.tx_hash);
  // Retry up to 3 times if not found (finality lag)
  for (let t = 0; t < 3 && !r.found; t++) {
    await new Promise(r => setTimeout(r, 10_000));
    r = await fetchOne(s.tx_hash);
  }
  const merged = { ...s, cspr_live: r };
  results.push(merged);
  process.stdout.write(`\r[reconcile] ${i + 1}/${sends.length} ok=${results.filter(x=>x.cspr_live.found && !x.cspr_live.error).length} err=${results.filter(x=>x.cspr_live.found && x.cspr_live.error).length} missing=${results.filter(x=>!x.cspr_live.found).length}   `);
}
console.log();

const out = path.join(REPORT_DIR, `reconciled-errfix-${new Date().toISOString().replace(/[:.]/g,"-")}.jsonl`);
fs.writeFileSync(out, results.map(r => JSON.stringify(r)).join("\n") + "\n");
console.log("[reconcile] wrote:", out);

// Summary
const byEp = {};
for (const r of results) {
  const k = `${r.contract}.${r.entryPoint}`;
  const s = byEp[k] || (byEp[k] = { ok: 0, err: 0, missing: 0, errors: {} });
  if (!r.cspr_live.found) s.missing++;
  else if (r.cspr_live.error) {
    s.err++;
    s.errors[r.cspr_live.error] = (s.errors[r.cspr_live.error] || 0) + 1;
  }
  else s.ok++;
}
console.log("\n=== SUMMARY per entrypoint ===");
console.log("contract.entrypoint | ok | err | missing | err_reasons");
for (const [k, v] of Object.entries(byEp)) {
  const reasons = Object.entries(v.errors).map(([e,n]) => `${e}(${n})`).join(", ");
  console.log(`  ${k} | ${v.ok} | ${v.err} | ${v.missing} | ${reasons}`);
}

const total = results.length;
const okAll = results.filter(r => r.cspr_live.found && !r.cspr_live.error).length;
const errAll = results.filter(r => r.cspr_live.found && r.cspr_live.error).length;
const missAll = results.filter(r => !r.cspr_live.found).length;
console.log(`\n=== TOTALS ===`);
console.log(`total=${total} ok=${okAll} err=${errAll} missing=${missAll}`);
