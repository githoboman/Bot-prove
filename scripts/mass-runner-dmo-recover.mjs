#!/usr/bin/env node
// Re-send only the DMO-side transactions that failed in the first pass.
// Contracts affected (DMO half missing 25 tx each): defi_mock, stake_slashing, proof_aggregation
// And proof_of_inference DMO tail (11 register + 5 verify).
// Uses reduced payment cap 1.5 CSPR since real consumed ≪ 3.

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

const CHAIN = process.env.CASPER_CHAIN || "casper-test";
const NODE  = process.env.CASPER_NODE  || "https://node.testnet.casper.network/rpc";
const DMO_PEM = process.env.DMO_PEM || "/tmp/dmo.pem";
const ANNA_PEM = process.env.ANNA_PEM || "/tmp/anna.pem";
const PAYMENT = 3_000_000_000; // 3 CSPR (Casper 2.x minimum for contract calls)

const ONCHAIN = JSON.parse(fs.readFileSync(
  path.resolve(__dirname, "../frontend/public/onchain.json"), "utf8"));

const ts = new Date().toISOString().replace(/[:.]/g, "-");
const REPORT_DIR = path.resolve(__dirname, "../reports");
fs.mkdirSync(REPORT_DIR, { recursive: true });
const REPORT = path.join(REPORT_DIR, `mass-runner-dmo-recover-${ts}.jsonl`);
function log(o) { fs.appendFileSync(REPORT, JSON.stringify(o) + "\n"); }

function loadKey(pemPath) {
  return PrivateKey.fromPem(fs.readFileSync(pemPath, "utf8"), KeyAlgorithm.SECP256K1);
}

const DMO = loadKey(DMO_PEM);
const ANNA = loadKey(ANNA_PEM);
const ANNA_AH = ANNA.publicKey.accountHash().toBytes();
const DMO_AH = DMO.publicKey.accountHash().toBytes();

const client = new RpcClient(new HttpHandler(NODE));

function randHex(n = 32) { return crypto.randomBytes(n).toString("hex"); }
function argStr(v) { return CLValue.newCLString(String(v)); }
function argU64(v) { return CLValue.newCLUint64(BigInt(v)); }
function argAH(bytes) { return CLValue.newCLByteArray(bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes)); }

async function send({ contract, entryPoint, argsMap, note }) {
  const record = {
    ts: new Date().toISOString(), signer: "dmo",
    contract, entryPoint, note, payment_motes: PAYMENT,
  };
  try {
    const tx = new ContractCallBuilder()
      .byHash(ONCHAIN.contracts[contract].contract_hash)
      .entryPoint(entryPoint)
      .runtimeArgs(new Args(argsMap))
      .from(DMO.publicKey)
      .chainName(CHAIN)
      .payment(PAYMENT)
      .build();
    tx.sign(DMO);
    const res = await client.putTransaction(tx);
    const hash = res?.transactionHash?.transactionV1?.hash ??
      res?.transactionHash?.hash ?? res?.transactionHash ?? res?.hash;
    const hashStr = typeof hash === "string" ? hash : JSON.stringify(hash);
    record.tx_hash = hashStr;
    log(record);
    console.log(`[sent] ${contract}.${entryPoint} ${note} tx=${hashStr.slice(1, 12)}…`);
    return hashStr;
  } catch (e) {
    record.error_send = e?.message || String(e);
    log(record);
    console.log(`[FAIL] ${contract}.${entryPoint}: ${record.error_send}`);
    return null;
  }
}

async function main() {
  console.log(`[dmo-recover] payment=${PAYMENT / 1e9} CSPR each`);

  const items = [];

  // defi_mock DMO: 15 grant_access + 5 revoke + 5 check_kyc
  for (let i = 0; i < 15; i++) {
    items.push({ contract: "defi_mock", entryPoint: "grant_access",
      argsMap: new Map([["user", argAH(ANNA_AH)], ["proof_id", argStr("proof_grant_R" + i + "_" + randHex(6))]]),
      note: `grant#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ contract: "defi_mock", entryPoint: "revoke_access",
      argsMap: new Map([["user", argAH(ANNA_AH)]]), note: `revoke#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ contract: "defi_mock", entryPoint: "check_kyc",
      argsMap: new Map([["proof_id", argStr("proof_check_dmoR_" + i + "_" + randHex(6))]]),
      note: `check_kyc#${i}` });
  }

  // stake_slashing DMO: 10 get_purse + 15 get_stake
  for (let i = 0; i < 10; i++) {
    items.push({ contract: "stake_slashing", entryPoint: "get_purse",
      argsMap: new Map(), note: `getpurse#${i}` });
  }
  for (let i = 0; i < 15; i++) {
    items.push({ contract: "stake_slashing", entryPoint: "get_stake",
      argsMap: new Map([["agent", argAH(DMO_AH)]]), note: `getstake#${i}` });
  }

  // proof_aggregation DMO: 5 create + 15 add + 5 finalize
  const batchIds = [];
  for (let i = 0; i < 5; i++) {
    const bid = "batchR_" + i + "_" + randHex(4);
    batchIds.push(bid);
    items.push({ contract: "proof_aggregation", entryPoint: "create_batch",
      argsMap: new Map([
        ["batch_id", argStr(bid)],
        ["merkle_root", argStr(randHex(32))],
        ["max_proofs", argU64(100)],
      ]), note: `create#${i}` });
  }
  let leaf = 0;
  for (let i = 0; i < 15; i++) {
    items.push({ contract: "proof_aggregation", entryPoint: "add_proof",
      argsMap: new Map([
        ["batch_id", argStr(batchIds[i % 5])],
        ["proof_hash", argStr(randHex(32))],
        ["leaf_index", argU64(leaf++)],
      ]), note: `addproof_dmoR#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ contract: "proof_aggregation", entryPoint: "finalize_batch",
      argsMap: new Map([["batch_id", argStr(batchIds[i])]]), note: `finalize#${i}` });
  }

  // proof_of_inference DMO tail: register #9..#19 (11 more) + verify #0..#4 (5)
  for (let i = 9; i < 20; i++) {
    items.push({ contract: "proof_of_inference", entryPoint: "register_proof",
      argsMap: new Map([
        ["model_hash", argStr(randHex(32))],
        ["input_hash", argStr(randHex(32))],
        ["output_hash", argStr(randHex(32))],
        ["proof_hash", argStr(randHex(32))],
        ["agent_id", argStr("agent_dmoR_" + i)],
        ["price_bps", argU64(100)],
      ]), note: `register#${i}` });
  }
  for (let i = 0; i < 5; i++) {
    items.push({ contract: "proof_of_inference", entryPoint: "verify_proof",
      argsMap: new Map([["proof_id", argStr(String(i + 1))]]),
      note: `verify#${i}` });
  }

  console.log(`[plan] ${items.length} tx`);
  const sent = [];
  for (const it of items) {
    const hash = await send(it);
    sent.push({ ...it, hash });
    await new Promise(r => setTimeout(r, 900));
  }
  console.log(`[done sending] ${sent.filter(x=>x.hash).length}/${sent.length}`);

  // Wait for finality
  const pending = new Map();
  for (const s of sent) if (s.hash) pending.set(s.hash, s);
  console.log(`[wait] finality for ${pending.size} tx…`);
  const deadline = Date.now() + 480_000;
  const done = [];
  while (pending.size && Date.now() < deadline) {
    await new Promise(r => setTimeout(r, 5000));
    for (const [hash, s] of Array.from(pending.entries())) {
      try {
        const info = await client.getTransaction(hash);
        const r0 = info?.executionInfo?.executionResult ?? info?.execution_info?.execution_result;
        if (r0) {
          const err = r0?.errorMessage ?? r0?.error_message ?? null;
          const consumed = r0?.consumed ?? r0?.cost ?? r0?.payment?.amount ?? null;
          const rec = {
            ...s, tx_hash: hash, block: info?.executionInfo?.blockHash ?? "?",
            consumed_motes: consumed?.toString?.() ?? consumed,
            consumed_cspr: consumed ? (Number(consumed) / 1e9).toFixed(4) : null,
            error: err, ok: !err, final: true,
          };
          delete rec.hash; delete rec.argsMap;
          log(rec);
          done.push(rec);
          pending.delete(hash);
        }
      } catch {}
    }
  }
  if (pending.size) {
    console.log(`[wait] timeout, ${pending.size} pending`);
    for (const [h, s] of pending) {
      const rec = { ...s, tx_hash: h, final: false, error: "finality-timeout" };
      delete rec.hash; delete rec.argsMap;
      log(rec);
      done.push(rec);
    }
  }
  const ok = done.filter(d => d.ok).length;
  const err = done.length - ok;
  const gas = done.reduce((s, d) => s + Number(d.consumed_motes || 0), 0) / 1e9;
  console.log(`\n=== DONE ===\nok=${ok} err=${err} gas=${gas.toFixed(4)} CSPR`);
  console.log(`report: ${REPORT}`);
}
main().catch(e => { console.error(e); process.exit(1); });
