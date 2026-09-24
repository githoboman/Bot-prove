#!/usr/bin/env node
// mass-runner.mjs — orchestrate 350 tx (25 per contract per signer × 7 contracts × 2 signers)
// across all 7 CasperProver contracts on testnet.
// Records every tx result to /data/cp/repo/reports/mass-runner-<timestamp>.jsonl
//
// Usage:
//   CASPER_NODE=<rpc> ANNA_PEM=/tmp/anna.pem DMO_PEM=/tmp/dmo.pem node mass-runner.mjs [--dry] [--limit N] [--contracts a,b,c]
//
// Strategy per contract (25 tx / signer, entrypoints mixed for coverage):
//   proof_registry      : submit_proof×15 + register_agent×5 + revoke_proof×5     [both signers]
//   verifier_gate       : verify×20 + batch_check×5                                [both signers]
//   defi_mock           : grant_access×15 + revoke_access×5 + check_kyc×5          [DMO admin only – Anna calls check_kyc×25]
//   stake_slashing      : (permissionless read-heavy) get_purse×10 + get_stake×15  [both signers — record_stake needs session contract, so we use view-heavy]
//   proof_aggregation   : create_batch×5 + add_proof×15 + finalize_batch×5         [DMO installer only; Anna does add_proof×25 into DMO's batches]
//   model_registry      : register_model×15 + update_model×5 + verify_model×5      [both signers — register_model is permissionless, updates are owner-gated so we use only own models]
//   proof_of_inference  : register_proof×20 + verify_proof×5                       [both signers]

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";
import crypto from "node:crypto";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const require = createRequire(import.meta.url);
const sdk = require(path.resolve(__dirname, "../frontend/node_modules/casper-js-sdk"));

const {
  ContractCallBuilder, HttpHandler, RpcClient,
  PrivateKey, KeyAlgorithm, PublicKey,
  Args, CLValue, AccountHash, Key, CLValueParser,
} = sdk;

const CHAIN = process.env.CASPER_CHAIN || "casper-test";
const NODE  = process.env.CASPER_NODE  || "https://node.testnet.casper.network/rpc";
const ANNA_PEM = process.env.ANNA_PEM || "/tmp/anna.pem";
const DMO_PEM  = process.env.DMO_PEM  || "/tmp/dmo.pem";

const ONCHAIN = JSON.parse(fs.readFileSync(
  path.resolve(__dirname, "../frontend/public/onchain.json"), "utf8"));

// dry-run: build TXs, log intent, do NOT send.
const args = process.argv.slice(2);
const DRY = args.includes("--dry");
const LIMIT = (() => {
  const i = args.findIndex(a => a === "--limit");
  return i >= 0 ? parseInt(args[i+1], 10) : Infinity;
})();
const ONLY_CONTRACTS = (() => {
  const i = args.findIndex(a => a === "--contracts");
  return i >= 0 ? args[i+1].split(",") : null;
})();

const ts = new Date().toISOString().replace(/[:.]/g, "-");
const REPORT_DIR = path.resolve(__dirname, "../reports");
fs.mkdirSync(REPORT_DIR, { recursive: true });
const REPORT = path.join(REPORT_DIR, `mass-runner-${ts}.jsonl`);
const SUMMARY = path.join(REPORT_DIR, `mass-runner-${ts}.summary.md`);

function log(o) { fs.appendFileSync(REPORT, JSON.stringify(o) + "\n"); }

function randHex(n = 32) { return crypto.randomBytes(n).toString("hex"); }
function shortHash(s) { return typeof s === "string" ? s.slice(0, 10) + "…" : String(s); }

function loadKey(pemPath) {
  const pem = fs.readFileSync(pemPath, "utf8");
  return PrivateKey.fromPem(pem, KeyAlgorithm.SECP256K1);
}

const ANNA = loadKey(ANNA_PEM);
const DMO  = loadKey(DMO_PEM);

const ANNA_PK = ANNA.publicKey.toHex();
const DMO_PK  = DMO.publicKey.toHex();
const DMO_ACCOUNT_HASH = DMO.publicKey.accountHash().toBytes(); // 32 bytes

console.log(`[runner] node=${NODE}`);
console.log(`[runner] anna=${ANNA_PK.slice(0, 20)}…`);
console.log(`[runner] dmo =${DMO_PK.slice(0, 20)}…`);
console.log(`[runner] chain=${CHAIN} dry=${DRY} limit=${LIMIT === Infinity ? "∞" : LIMIT}`);
console.log(`[runner] report=${REPORT}`);

const client = new RpcClient(new HttpHandler(NODE));

// --------- typed arg builders ---------
function argStr(v) { return CLValue.newCLString(String(v)); }
function argU64(v) { return CLValue.newCLUint64(BigInt(v)); }
function argU32(v) { return CLValue.newCLUInt32(Number(v)); }
function argU512(v) { return CLValue.newCLUInt512(String(v)); }
function argAccountHashBytes(hashBytes32) {
  // Contract's runtime::get_named_arg::<AccountHash>() deserializes 32 raw bytes.
  const u8 = hashBytes32 instanceof Uint8Array ? hashBytes32 : new Uint8Array(hashBytes32);
  return CLValue.newCLByteArray(u8);
}
function argListStr(list) {
  // sdk.CLTypeString is already an instance (not a constructor).
  const items = list.map(s => CLValue.newCLString(String(s)));
  return CLValue.newCLList(sdk.CLTypeString, items);
}

// --------- one tx ---------
async function callContract({ signer, contract, entryPoint, argsMap, payment = 3_000_000_000, note = "" }) {
  const record = {
    ts: new Date().toISOString(),
    signer: signer === ANNA ? "anna" : "dmo",
    contract, entryPoint, note,
    payment_motes: payment,
  };
  if (DRY) {
    record.dry = true;
    log(record);
    console.log(`[dry] ${record.signer} → ${contract}.${entryPoint} ${note}`);
    return record;
  }

  try {
    const tx = new ContractCallBuilder()
      .byHash(ONCHAIN.contracts[contract].contract_hash)
      .entryPoint(entryPoint)
      .runtimeArgs(new Args(argsMap))
      .from(signer.publicKey)
      .chainName(CHAIN)
      .payment(payment)
      .build();
    tx.sign(signer);
    const res = await client.putTransaction(tx);
    const hash =
      res?.transactionHash?.transactionV1?.hash ??
      res?.transactionHash?.hash ??
      res?.transactionHash ?? res?.hash;
    const hashStr = typeof hash === "string" ? hash : JSON.stringify(hash);
    record.tx_hash = hashStr;
    console.log(`[sent] ${record.signer} → ${contract}.${entryPoint} tx=${shortHash(hashStr)}`);
    log(record);
    return { record, hashStr };
  } catch (e) {
    record.error_send = e?.message || String(e);
    console.log(`[send-fail] ${record.signer} → ${contract}.${entryPoint}: ${record.error_send}`);
    log(record);
    return { record, hashStr: null };
  }
}

// --------- await finality (batched) ---------
async function awaitFinality(sentList, deadlineMs = 240_000) {
  const start = Date.now();
  const pending = new Map();
  for (const s of sentList) {
    if (s?.hashStr) pending.set(s.hashStr, s.record);
  }
  console.log(`[wait] finality for ${pending.size} tx…`);
  const done = [];
  while (pending.size && Date.now() - start < deadlineMs) {
    await new Promise(r => setTimeout(r, 5000));
    for (const [hash, rec] of Array.from(pending.entries())) {
      try {
        const info = await client.getTransaction(hash);
        const r0 = info?.executionInfo?.executionResult ?? info?.execution_info?.execution_result;
        if (r0) {
          const err = r0?.errorMessage ?? r0?.error_message ?? null;
          const consumed = r0?.consumed ?? r0?.cost ?? r0?.payment?.amount ?? null;
          const block = info?.executionInfo?.blockHash ?? "?";
          const result = {
            ...rec,
            tx_hash: hash,
            block: typeof block === "string" ? block : String(block),
            consumed_motes: consumed?.toString?.() ?? consumed,
            consumed_cspr: consumed ? (Number(consumed) / 1e9).toFixed(4) : null,
            error: err,
            ok: !err,
            final: true,
          };
          log(result);
          done.push(result);
          pending.delete(hash);
        }
      } catch { /* still pending */ }
    }
  }
  if (pending.size) {
    console.log(`[wait] timeout — ${pending.size} tx still pending`);
    for (const [hash, rec] of pending) {
      const result = { ...rec, tx_hash: hash, final: false, error: "finality-timeout" };
      log(result);
      done.push(result);
    }
  }
  return done;
}

// ---------------- entrypoint plans ----------------
// Each contract's plan returns an array of {signer, contract, entryPoint, argsMap, note}

function planProofRegistry() {
  const items = [];
  const contract = "proof_registry";
  // Anna: 15 submit + 5 register_agent + 5 revoke of her own proofs
  const annaProofs = [];
  for (let i = 0; i < 15; i++) {
    const ph = randHex(32);
    annaProofs.push(ph);
    items.push({ signer: ANNA, contract, entryPoint: "submit_proof",
      argsMap: new Map([
        ["proof_hash", argStr(ph)],
        ["input_hash", argStr(randHex(32))],
        ["output_hash", argStr(randHex(32))],
        ["model_hash", argStr("m_anna_" + i)],
      ]), note: `submit#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ signer: ANNA, contract, entryPoint: "register_agent",
      argsMap: new Map([
        ["agent_id", argStr("agent_anna_" + i)],
        ["model_hash", argStr("m_anna_reg_" + i)],
      ]), note: `agent#${i}` });
  }
  // Anna revoke 5 of her own proofs. proof_id in this contract == proof_hash (submit stores under ph key)
  for (let i = 0; i < 5; i++) {
    items.push({ signer: ANNA, contract, entryPoint: "revoke_proof",
      argsMap: new Map([["proof_id", argStr(annaProofs[i])]]), note: `revoke#${i}` });
  }
  // DMO: 15 submit + 5 register_agent + 5 revoke
  const dmoProofs = [];
  for (let i = 0; i < 15; i++) {
    const ph = randHex(32);
    dmoProofs.push(ph);
    items.push({ signer: DMO, contract, entryPoint: "submit_proof",
      argsMap: new Map([
        ["proof_hash", argStr(ph)],
        ["input_hash", argStr(randHex(32))],
        ["output_hash", argStr(randHex(32))],
        ["model_hash", argStr("m_dmo_" + i)],
      ]), note: `submit#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ signer: DMO, contract, entryPoint: "register_agent",
      argsMap: new Map([
        ["agent_id", argStr("agent_dmo_" + i)],
        ["model_hash", argStr("m_dmo_reg_" + i)],
      ]), note: `agent#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ signer: DMO, contract, entryPoint: "revoke_proof",
      argsMap: new Map([["proof_id", argStr(dmoProofs[i])]]), note: `revoke#${i}` });
  }
  return items;
}

function planVerifierGate() {
  const items = [];
  const contract = "verifier_gate";
  // verify() and batch_check() — no admin. Contract calls proof_registry read.
  // Even with unknown proof_id it exercises the entry point (rate-limit gates it too).
  for (const signer of [ANNA, DMO]) {
    for (let i = 0; i < 20; i++) {
      items.push({ signer, contract, entryPoint: "verify",
        argsMap: new Map([["proof_id", argStr(randHex(32))]]), note: `verify#${i}` });
    }
    for (let i = 0; i < 5; i++) {
      // batch_check expects Vec<String>
      const list = Array.from({length: 3}, () => randHex(16));
      items.push({ signer, contract, entryPoint: "batch_check",
        argsMap: new Map([["proof_ids", argListStr(list)]]), note: `batch#${i}` });
    }
  }
  return items;
}

function planDefiMock() {
  const items = [];
  const contract = "defi_mock";
  // grant_access / revoke_access are admin-only (= DMO). check_kyc & is_whitelisted are permissionless.
  // Plan: DMO does 15 grant + 5 revoke + 5 check_kyc. Anna does 25 check_kyc.
  const annaAH = ANNA.publicKey.accountHash().toBytes(); // Uint8Array 32
  for (let i = 0; i < 15; i++) {
    items.push({ signer: DMO, contract, entryPoint: "grant_access",
      argsMap: new Map([
        ["user", argAccountHashBytes(annaAH)], // grant Anna access with different proof_ids
        ["proof_id", argStr("proof_grant_" + i + "_" + randHex(6))],
      ]), note: `grant#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ signer: DMO, contract, entryPoint: "revoke_access",
      argsMap: new Map([["user", argAccountHashBytes(annaAH)]]),
      note: `revoke#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ signer: DMO, contract, entryPoint: "check_kyc",
      argsMap: new Map([["proof_id", argStr("proof_check_dmo_" + i + "_" + randHex(6))]]),
      note: `check_kyc#${i}` });
  }
  for (let i = 0; i < 25; i++) {
    items.push({ signer: ANNA, contract, entryPoint: "check_kyc",
      argsMap: new Map([["proof_id", argStr("proof_check_anna_" + i + "_" + randHex(6))]]),
      note: `check_kyc#${i}` });
  }
  return items;
}

function planStakeSlashing() {
  const items = [];
  const contract = "stake_slashing";
  // record_stake requires actual purse transfer via a session contract — skip write and use safe entrypoints.
  // report_and_slash requires a revoked proof in registry, hard to orchestrate 25×.
  // Fallback: hit read-heavy entrypoints get_purse (returns URef) + get_stake for coverage. These still consume gas.
  // Also try unstake with 0 amount — should revert cleanly.
  for (const signer of [ANNA, DMO]) {
    for (let i = 0; i < 10; i++) {
      items.push({ signer, contract, entryPoint: "get_purse",
        argsMap: new Map(), note: `getpurse#${i}` });
    }
    const targetAH = signer === ANNA ? ANNA.publicKey.accountHash().toBytes() : DMO.publicKey.accountHash().toBytes();
    for (let i = 0; i < 15; i++) {
      items.push({ signer, contract, entryPoint: "get_stake",
        argsMap: new Map([["agent", argAccountHashBytes(targetAH)]]),
        note: `getstake#${i}` });
    }
  }
  return items;
}

function planProofAggregation() {
  const items = [];
  const contract = "proof_aggregation";
  // create_batch and finalize_batch are installer-only (DMO).
  // add_proof is permissionless (any caller — writes to installer's batches).
  // DMO plan (25): 5 create_batch + 15 add_proof + 5 finalize_batch.
  // Anna plan (25): 25 add_proof (into DMO-created batches).
  const batchIds = [];
  for (let i = 0; i < 5; i++) {
    const bid = "batch_" + i + "_" + randHex(4);
    batchIds.push(bid);
    items.push({ signer: DMO, contract, entryPoint: "create_batch",
      argsMap: new Map([
        ["batch_id", argStr(bid)],
        ["merkle_root", argStr(randHex(32))],
        ["max_proofs", argU64(100)],
      ]), note: `create#${i}` });
  }
  // DMO adds 15 proofs across batches
  let leaf = 0;
  for (let i = 0; i < 15; i++) {
    items.push({ signer: DMO, contract, entryPoint: "add_proof",
      argsMap: new Map([
        ["batch_id", argStr(batchIds[i % 5])],
        ["proof_hash", argStr(randHex(32))],
        ["leaf_index", argU64(leaf++)],
      ]), note: `addproof_dmo#${i}` });
  }
  // Anna adds 25 proofs into existing batches
  for (let i = 0; i < 25; i++) {
    items.push({ signer: ANNA, contract, entryPoint: "add_proof",
      argsMap: new Map([
        ["batch_id", argStr(batchIds[i % 5])],
        ["proof_hash", argStr(randHex(32))],
        ["leaf_index", argU64(50 + i)],
      ]), note: `addproof_anna#${i}` });
  }
  // DMO finalizes 5 batches
  for (let i = 0; i < 5; i++) {
    items.push({ signer: DMO, contract, entryPoint: "finalize_batch",
      argsMap: new Map([["batch_id", argStr(batchIds[i])]]),
      note: `finalize#${i}` });
  }
  return items;
}

function planModelRegistry() {
  const items = [];
  const contract = "model_registry";
  // register_model is permissionless; update_model and deprecate_model check owner.
  // Plan: each signer registers 15 own models, updates 5, then verify_model 5.
  for (const [signer, tag] of [[ANNA, "anna"], [DMO, "dmo"]]) {
    const models = [];
    for (let i = 0; i < 15; i++) {
      const mh = "mh_" + tag + "_" + randHex(16);
      models.push(mh);
      items.push({ signer, contract, entryPoint: "register_model",
        argsMap: new Map([
          ["model_hash", argStr(mh)],
          ["name", argStr("Model-" + tag + "-" + i)],
          ["version", argStr("1.0." + i)],
          ["ipfs_cid", argStr("Qm" + randHex(20))],
        ]), note: `register#${i}` });
    }
    for (let i = 0; i < 5; i++) {
      items.push({ signer, contract, entryPoint: "update_model",
        argsMap: new Map([
          ["model_hash", argStr(models[i])],
          ["version", argStr("1.1." + i)],
          ["ipfs_cid", argStr("Qm" + randHex(20))],
        ]), note: `update#${i}` });
    }
    // verify_model is permissionless read+write (verifier flag). Use own models.
    for (let i = 0; i < 5; i++) {
      items.push({ signer, contract, entryPoint: "verify_model",
        argsMap: new Map([["model_hash", argStr(models[5 + i])]]),
        note: `verify#${i}` });
    }
  }
  return items;
}

function planProofOfInference() {
  const items = [];
  const contract = "proof_of_inference";
  for (const [signer, tag] of [[ANNA, "anna"], [DMO, "dmo"]]) {
    const proofs = [];
    for (let i = 0; i < 20; i++) {
      // register_proof returns proof_id (auto counter). To verify we'd need to read counter -
      // simpler: verify a synthetic proof_id string too, expected NOT_FOUND but exercises path
      items.push({ signer, contract, entryPoint: "register_proof",
        argsMap: new Map([
          ["model_hash", argStr(randHex(32))],
          ["input_hash", argStr(randHex(32))],
          ["output_hash", argStr(randHex(32))],
          ["proof_hash", argStr(randHex(32))],
          ["agent_id", argStr("agent_" + tag + "_" + i)],
          ["price_bps", argU64(100)],
        ]), note: `register#${i}` });
    }
    for (let i = 0; i < 5; i++) {
      // verify_proof by known ids — proof_ids in this contract are numeric counter strings starting at 1.
      // Use signer-agnostic verifier attempt on first few counter positions.
      items.push({ signer, contract, entryPoint: "verify_proof",
        argsMap: new Map([["proof_id", argStr(String(i + 1))]]),
        note: `verify#${i}` });
    }
  }
  return items;
}

// ---------------- main ----------------
async function main() {
  const contractsPlan = {
    proof_registry:      planProofRegistry,
    verifier_gate:       planVerifierGate,
    defi_mock:           planDefiMock,
    stake_slashing:      planStakeSlashing,
    proof_aggregation:   planProofAggregation,
    model_registry:      planModelRegistry,
    proof_of_inference:  planProofOfInference,
  };

  // Order: proof_registry first (verifier_gate reads from it), then verifier_gate,
  // then the rest.
  const ORDER = ["proof_registry", "verifier_gate", "model_registry",
                 "proof_of_inference", "defi_mock", "stake_slashing",
                 "proof_aggregation"];

  const all = [];
  for (const c of ORDER) {
    if (ONLY_CONTRACTS && !ONLY_CONTRACTS.includes(c)) continue;
    const plan = contractsPlan[c]();
    console.log(`[plan] ${c}: ${plan.length} tx`);
    all.push(...plan);
  }

  const total = Math.min(all.length, LIMIT);
  console.log(`[plan] total planned: ${all.length}, executing: ${total}`);

  const CONCURRENCY = 2; // Anna and DMO in parallel
  // Split by signer to run in parallel streams
  const annaQueue = all.slice(0, total).filter(x => x.signer === ANNA);
  const dmoQueue  = all.slice(0, total).filter(x => x.signer === DMO);
  console.log(`[plan] anna=${annaQueue.length} dmo=${dmoQueue.length}`);

  const inFlight = [];
  // Send phase — spread each queue over time. To avoid same-signer overlap (Casper
  // sequences per account), send sequentially per signer, but parallel across the two.
  async function sendQueue(queue, label) {
    const sent = [];
    for (let i = 0; i < queue.length; i++) {
      const item = queue[i];
      const res = await callContract(item);
      sent.push(res);
      // Small pause between sends per signer to give the pending-pool time to accept.
      await new Promise(r => setTimeout(r, DRY ? 0 : 800));
    }
    console.log(`[${label}] sent ${sent.length}`);
    return sent;
  }
  const [annaSent, dmoSent] = await Promise.all([
    sendQueue(annaQueue, "anna"),
    sendQueue(dmoQueue,  "dmo"),
  ]);

  const all_sent = [...annaSent, ...dmoSent];
  if (DRY) {
    console.log(`[dry] would send ${all_sent.length} tx`);
    return;
  }

  // Await finality of all sent tx (they may confirm in blocks after send-phase ends)
  const finals = await awaitFinality(all_sent, 600_000); // 10 min

  // Aggregate
  const okCount = finals.filter(f => f.ok).length;
  const errCount = finals.filter(f => !f.ok).length;
  const totalGasMotes = finals.reduce((s, f) => s + (f.consumed_motes ? BigInt(f.consumed_motes) : 0n), 0n);
  const totalGasCspr = Number(totalGasMotes) / 1e9;

  const perContract = {};
  for (const f of finals) {
    const k = f.contract;
    if (!perContract[k]) perContract[k] = { sent: 0, ok: 0, err: 0, gas: 0 };
    perContract[k].sent++;
    if (f.ok) perContract[k].ok++;
    else perContract[k].err++;
    perContract[k].gas += Number(f.consumed_motes || 0) / 1e9;
  }

  // Write summary
  let md = `# Mass runner report — ${ts}\n\n`;
  md += `- Network: ${CHAIN}\n- Node: ${NODE}\n`;
  md += `- Anna: \`${ANNA_PK}\`\n- DMO:  \`${DMO_PK}\`\n\n`;
  md += `## Totals\n\n`;
  md += `- Sent: ${all_sent.length}\n- Ok: ${okCount}\n- Errored: ${errCount}\n- Gas consumed: ${totalGasCspr.toFixed(4)} CSPR\n\n`;
  md += `## Per-contract\n\n`;
  md += `| Contract | Sent | Ok | Err | Gas (CSPR) | Avg (CSPR) |\n|---|---|---|---|---|---|\n`;
  for (const [k, v] of Object.entries(perContract)) {
    md += `| ${k} | ${v.sent} | ${v.ok} | ${v.err} | ${v.gas.toFixed(3)} | ${(v.gas / Math.max(v.sent, 1)).toFixed(4)} |\n`;
  }
  md += `\n## Raw log\n\n${REPORT}\n`;
  fs.writeFileSync(SUMMARY, md);

  console.log(`\n=== SUMMARY ===\n${md}\n`);
  console.log(`[done] report: ${REPORT}\n[done] summary: ${SUMMARY}`);
}

main().catch(e => {
  console.error("[fatal]", e);
  process.exit(1);
});
