#!/usr/bin/env node
// mass-runner-fix.mjs — second pass with corrected payloads for entrypoints
// that reverted in the first pass due to input format / preconditions.
//
// Fixes:
//   model_registry.register_model / update_model / verify_model
//     — model_hash must be exactly 64 hex chars (not "mh_anna_...")
//   verifier_gate.verify / batch_check
//     — use REAL proof_ids submitted in pass 1 (from first-run report)
//   defi_mock.check_kyc / grant_access
//     — same: reference real proof_ids that pass verify()
//   proof_registry.revoke_proof
//     — proof_id key = ph (proof_hash); use hex hashes from first-run submits
//
// Total: ~150 fresh, targeted transactions.

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
  PrivateKey, KeyAlgorithm, Args, CLValue,
} = sdk;

const CHAIN = "casper-test";
const NODE = process.env.CASPER_NODE || "https://node.testnet.casper.network/rpc";
const ANNA = PrivateKey.fromPem(fs.readFileSync(process.env.ANNA_PEM || "/tmp/anna.pem", "utf8"), KeyAlgorithm.SECP256K1);
const DMO  = PrivateKey.fromPem(fs.readFileSync(process.env.DMO_PEM  || "/tmp/dmo.pem",  "utf8"), KeyAlgorithm.SECP256K1);
const ANNA_AH = ANNA.publicKey.accountHash().toBytes();
const DMO_AH  = DMO.publicKey.accountHash().toBytes();

const ONCHAIN = JSON.parse(fs.readFileSync(path.resolve(__dirname, "../frontend/public/onchain.json"), "utf8"));
const PAYMENT = 3_000_000_000;
const client = new RpcClient(new HttpHandler(NODE));

const ts = new Date().toISOString().replace(/[:.]/g, "-");
const REPORT_DIR = path.resolve(__dirname, "../reports");
const REPORT = path.join(REPORT_DIR, `mass-runner-fix-${ts}.jsonl`);
function log(o) { fs.appendFileSync(REPORT, JSON.stringify(o) + "\n"); }

function randHex(n = 32) { return crypto.randomBytes(n).toString("hex"); }
function hex64() { return randHex(32); } // 32 bytes → 64 hex chars
function argStr(v) { return CLValue.newCLString(String(v)); }
function argU64(v) { return CLValue.newCLUint64(BigInt(v)); }
function argAH(b)  { return CLValue.newCLByteArray(b instanceof Uint8Array ? b : new Uint8Array(b)); }
function argListStr(l) { const items = l.map(s => CLValue.newCLString(String(s))); return CLValue.newCLList(sdk.CLTypeString, items); }

async function send(signer, contract, entryPoint, argsMap, note) {
  const record = { ts: new Date().toISOString(), signer: signer === ANNA ? "anna" : "dmo", contract, entryPoint, note, payment_motes: PAYMENT };
  try {
    const tx = new ContractCallBuilder()
      .byHash(ONCHAIN.contracts[contract].contract_hash)
      .entryPoint(entryPoint)
      .runtimeArgs(new Args(argsMap))
      .from(signer.publicKey)
      .chainName(CHAIN)
      .payment(PAYMENT)
      .build();
    tx.sign(signer);
    const res = await client.putTransaction(tx);
    const hash = res?.transactionHash?.transactionV1?.hash ?? res?.transactionHash?.hash ?? res?.transactionHash;
    const hashStr = typeof hash === "string" ? hash : JSON.stringify(hash);
    record.tx_hash = hashStr.replace(/^"|"$/g, "");
    log(record);
    console.log(`[sent] ${record.signer} → ${contract}.${entryPoint} ${note} tx=${record.tx_hash.slice(0,10)}…`);
    return record.tx_hash;
  } catch (e) {
    record.error_send = e?.message || String(e);
    log(record);
    console.log(`[FAIL] ${record.signer} → ${contract}.${entryPoint}: ${record.error_send}`);
    return null;
  }
}

async function main() {
  console.log("[fix] plan: 150 tx targeting first-pass reverts with correct payloads");

  const items = [];

  // 1) proof_registry.submit_proof — pre-fill 20 valid proofs (for downstream verify() and check_kyc())
  //    Then register 2 agents (one per signer) since agent MUST exist before submit_proof succeeds.
  const submittedProofs = { anna: [], dmo: [] };

  // Register both agents FIRST (need for submit_proof to pass ERR_AGENT_NOT_FOUND)
  items.push({ signer: ANNA, contract: "proof_registry", entryPoint: "register_agent",
    argsMap: new Map([["agent_id", argStr("agent_anna_fix_" + randHex(4))], ["model_hash", argStr(hex64())]]),
    note: "agent-anna-fix" });
  items.push({ signer: DMO, contract: "proof_registry", entryPoint: "register_agent",
    argsMap: new Map([["agent_id", argStr("agent_dmo_fix_" + randHex(4))], ["model_hash", argStr(hex64())]]),
    note: "agent-dmo-fix" });

  // 20 submit_proof each signer — using hex64 model_hash so it also satisfies model_registry
  for (const [sig, tag] of [[ANNA, "anna"], [DMO, "dmo"]]) {
    for (let i = 0; i < 20; i++) {
      const ph = hex64();
      submittedProofs[tag].push(ph);
      items.push({ signer: sig, contract: "proof_registry", entryPoint: "submit_proof",
        argsMap: new Map([
          ["proof_hash", argStr(ph)],
          ["input_hash", argStr(hex64())],
          ["output_hash", argStr(hex64())],
          ["model_hash", argStr(hex64())],
        ]), note: `submit#${i}` });
    }
  }

  // 2) verifier_gate.verify — use REAL proof_ids just submitted (10 each signer = 20)
  //    Note: verify() reads registry, but registry stores under key ph (proof_hash), returning the proof.
  //    verify's real path calls read_proof(pid) so pid must equal a stored proof_hash.
  for (const [sig, tag] of [[ANNA, "anna"], [DMO, "dmo"]]) {
    for (let i = 0; i < 10; i++) {
      items.push({ signer: sig, contract: "verifier_gate", entryPoint: "verify",
        argsMap: new Map([["proof_id", argStr(submittedProofs[tag][i])]]),
        note: `verify-real#${i}` });
    }
  }

  // 3) verifier_gate.batch_check — 5 each signer with real proof_ids
  for (const [sig, tag] of [[ANNA, "anna"], [DMO, "dmo"]]) {
    for (let i = 0; i < 5; i++) {
      const list = [submittedProofs[tag][10 + i], submittedProofs[tag][11 + i], submittedProofs[tag][12 + i]];
      items.push({ signer: sig, contract: "verifier_gate", entryPoint: "batch_check",
        argsMap: new Map([["proof_ids", argListStr(list)]]),
        note: `batch-real#${i}` });
    }
  }

  // 4) model_registry — proper 64-hex model_hash
  const registeredModels = { anna: [], dmo: [] };
  for (const [sig, tag] of [[ANNA, "anna"], [DMO, "dmo"]]) {
    for (let i = 0; i < 10; i++) {
      const mh = hex64();
      registeredModels[tag].push(mh);
      items.push({ signer: sig, contract: "model_registry", entryPoint: "register_model",
        argsMap: new Map([
          ["model_hash", argStr(mh)],
          ["name", argStr("ModelFix-" + tag + "-" + i)],
          ["version", argStr("2.0." + i)],
          ["ipfs_cid", argStr("Qm" + randHex(20))],
        ]), note: `register-fix#${i}` });
    }
    for (let i = 0; i < 5; i++) {
      items.push({ signer: sig, contract: "model_registry", entryPoint: "update_model",
        argsMap: new Map([
          ["model_hash", argStr(registeredModels[tag][i])],
          ["version", argStr("2.1." + i)],
          ["ipfs_cid", argStr("Qm" + randHex(20))],
        ]), note: `update-fix#${i}` });
    }
    for (let i = 0; i < 5; i++) {
      items.push({ signer: sig, contract: "model_registry", entryPoint: "verify_model",
        argsMap: new Map([["model_hash", argStr(registeredModels[tag][5 + i])]]),
        note: `verify-fix#${i}` });
    }
  }

  // 5) defi_mock.check_kyc — with real proof_ids (5 each signer)
  //    defi_mock calls verifier.is_valid(proof_id). is_valid returns 1 if proof exists AND is not revoked.
  //    Since submitted proofs from step 1 are neither revoked nor expired, this should pass check_kyc.
  for (const [sig, tag] of [[ANNA, "anna"], [DMO, "dmo"]]) {
    for (let i = 0; i < 5; i++) {
      items.push({ signer: sig, contract: "defi_mock", entryPoint: "check_kyc",
        argsMap: new Map([["proof_id", argStr(submittedProofs[tag][15 + i])]]),
        note: `kyc-real#${i}` });
    }
  }

  // 6) proof_registry.revoke_proof — 5 each signer with real ph they submitted
  //    (In pass 1 we used submittedProofs but revoke expects pid=ph. Should work now.)
  //    Skip: reserved these proofs for verify/kyc above. Instead submit 5 more per signer just for revoke.
  for (const [sig, tag] of [[ANNA, "anna"], [DMO, "dmo"]]) {
    for (let i = 0; i < 5; i++) {
      const ph = hex64();
      // First submit, then revoke.
      items.push({ signer: sig, contract: "proof_registry", entryPoint: "submit_proof",
        argsMap: new Map([
          ["proof_hash", argStr(ph)],
          ["input_hash", argStr(hex64())],
          ["output_hash", argStr(hex64())],
          ["model_hash", argStr(hex64())],
        ]), note: `submit-for-revoke#${i}` });
      items.push({ signer: sig, contract: "proof_registry", entryPoint: "revoke_proof",
        argsMap: new Map([["proof_id", argStr(ph)]]),
        note: `revoke-real#${i}` });
    }
  }

  console.log(`[fix] ${items.length} tx`);

  // Split into two phases so state-dependent tx wait for their preconditions.
  //   Phase A: register_agent + all submit_proof (preconditions).
  //   Then wait ~60s for finality.
  //   Phase B: verify / batch_check / check_kyc / model_registry / revoke.
  const phaseA = items.filter(x =>
    (x.contract === "proof_registry" && (x.entryPoint === "register_agent" || x.note?.startsWith("submit"))) &&
    !x.note?.startsWith("submit-for-revoke")
  );
  const phaseB = items.filter(x => !phaseA.includes(x));
  console.log(`[fix] phaseA=${phaseA.length} phaseB=${phaseB.length}`);

  async function sendList(list, label) {
    const annaQ = list.filter(x => x.signer === ANNA);
    const dmoQ  = list.filter(x => x.signer === DMO);
    async function sendQ(q, tag) {
      const sent = [];
      for (const it of q) {
        const h = await send(it.signer, it.contract, it.entryPoint, it.argsMap, it.note);
        sent.push({ ...it, tx_hash: h });
        await new Promise(r => setTimeout(r, 900));
      }
      console.log(`[${label}/${tag}] sent ${sent.filter(s => s.tx_hash).length}/${sent.length}`);
      return sent;
    }
    const [a, d] = await Promise.all([sendQ(annaQ, "anna"), sendQ(dmoQ, "dmo")]);
    return [...a, ...d];
  }

  const phaseAS = await sendList(phaseA, "A");
  console.log("[fix] phase A sent; waiting 90s for finality before phase B…");
  await new Promise(r => setTimeout(r, 90_000));
  const phaseBS = await sendList(phaseB, "B");
  const annaS = [...phaseAS.filter(x => x.signer === ANNA), ...phaseBS.filter(x => x.signer === ANNA)];
  const dmoS  = [...phaseAS.filter(x => x.signer === DMO),  ...phaseBS.filter(x => x.signer === DMO)];
  const all = [...annaS, ...dmoS];
  console.log(`[fix] done sending ${all.length}. Wait 90s for finality then reconcile via cspr.live.`);
  await new Promise(r => setTimeout(r, 90_000));
  console.log("[fix] complete");
}
main().catch(e => { console.error(e); process.exit(1); });
