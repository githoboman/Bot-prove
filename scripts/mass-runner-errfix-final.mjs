#!/usr/bin/env node
// mass-runner-errfix-final.mjs — final touch-up. Two remaining reverts:
//   1) batch_check ran out of gas → bump payment to 8 CSPR for that call
//   2) revoke_proof failed because block-ordering didn't match send-ordering,
//      so my expected owner-of-pid mapping was off. Fix: query the proof dict
//      for each pid, read the real agent, and revoke with the matching signer.

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const require = createRequire(import.meta.url);
const sdk = require(path.resolve(__dirname, "../frontend/node_modules/casper-js-sdk"));

const { ContractCallBuilder, HttpHandler, RpcClient, PrivateKey, KeyAlgorithm, Args, CLValue, CLTypeString } = sdk;

const CHAIN = "casper-test";
const NODE = "https://node.testnet.casper.network/rpc";
const ANNA = PrivateKey.fromPem(fs.readFileSync("/tmp/anna.pem", "utf8"), KeyAlgorithm.SECP256K1);
const DMO  = PrivateKey.fromPem(fs.readFileSync("/tmp/dmo.pem",  "utf8"), KeyAlgorithm.SECP256K1);
const ANNA_AH_HEX = Buffer.from(ANNA.publicKey.accountHash().toBytes()).toString("hex");
const DMO_AH_HEX  = Buffer.from(DMO.publicKey.accountHash().toBytes()).toString("hex");

const ONCHAIN = JSON.parse(fs.readFileSync(path.resolve(__dirname, "../frontend/public/onchain.json"), "utf8"));
const client = new RpcClient(new HttpHandler(NODE));

const ts = new Date().toISOString().replace(/[:.]/g, "-");
const REPORT = path.join(__dirname, "..", "reports", `mass-runner-errfix-final-${ts}.jsonl`);
const log = (o) => fs.appendFileSync(REPORT, JSON.stringify(o) + "\n");
const stamp = () => new Date().toISOString();
const sleep = (ms) => new Promise(r => setTimeout(r, ms));

async function stateRootHash() {
  const r = await fetch(NODE, { method: "POST", headers: {"Content-Type":"application/json"},
    body: JSON.stringify({jsonrpc:"2.0",id:1,method:"chain_get_state_root_hash",params:{}}) }).then(r=>r.json());
  return r.result.state_root_hash;
}

const PROOFS_UREF = "uref-03a3db8b5c0c2cf439c8c0c5b3ee0291ab59f04015cca154317d47988a40d0ef-007";

async function readProof(pid) {
  const srh = await stateRootHash();
  const r = await fetch(NODE, { method:"POST", headers:{"Content-Type":"application/json"},
    body: JSON.stringify({jsonrpc:"2.0",id:1,method:"state_get_dictionary_item",
      params:{state_root_hash:srh, dictionary_identifier:{URef:{seed_uref:PROOFS_UREF,dictionary_item_key:pid}}}})
  }).then(r=>r.json());
  const p = r?.result?.stored_value?.CLValue?.parsed;
  if (!p) return null;
  return { pid, agent: p[0][1], valid: p[2][1], revoked: p[2][2] };
}

async function send(signer, ep, contractName, argsMap, note, payment = 3_000_000_000) {
  const args = new Args(argsMap);
  const tx = new ContractCallBuilder()
    .byHash(ONCHAIN.contracts[contractName].contract_hash)
    .entryPoint(ep)
    .runtimeArgs(args)
    .from(signer.publicKey)
    .chainName(CHAIN)
    .payment(payment)
    .build();
  tx.sign(signer);
  try {
    const r = await client.putTransaction(tx);
    const hash = r?.transactionHash?.transactionV1?.toHex?.() || r?.transactionHash?.hash?.toHex?.() || String(r?.transactionHash);
    log({ ts: stamp(), signer: signer === ANNA ? "anna" : "dmo", contract: contractName, entryPoint: ep, note, tx_hash: hash, payment, status: "sent" });
    return { ok: true, hash };
  } catch (e) {
    log({ ts: stamp(), signer: signer === ANNA ? "anna" : "dmo", contract: contractName, entryPoint: ep, note, error: e.message, status: "send_failed" });
    return { ok: false };
  }
}

(async () => {
  console.log(`report → ${REPORT}`);

  // 1) batch_check with 8 CSPR payment (Out of gas fix)
  //    Read the still-live (non-revoked) pids from the errfix run: P-298, P-299, P-302, P-303, P-304, P-305
  const candidatePids = ["P-298","P-299","P-300","P-301","P-302","P-303","P-304","P-305"];
  const proofs = [];
  for (const pid of candidatePids) {
    const p = await readProof(pid);
    if (p) proofs.push(p);
    await sleep(200);
  }
  console.log("proofs read:", proofs.length);
  const liveProofs = proofs.filter(p => p.revoked === 0);
  console.log("live (non-revoked) proofs:", liveProofs.map(p => p.pid).join(", "));

  const pidsForBatch = proofs.map(p => p.pid); // include all for batch_check — it doesn't care about revoked
  for (const signer of [ANNA, DMO]) {
    const args = new Map([
      ["proof_ids", CLValue.newCLList(CLTypeString, pidsForBatch.map(p => CLValue.newCLString(p)))],
    ]);
    await send(signer, "batch_check", "verifier_gate", args, `batch_check ${pidsForBatch.length} pids (payment=8CSPR)`, 8_000_000_000);
    await sleep(1000);
  }

  // 2) revoke_proof with correct signer per pid.
  //    liveProofs still has un-revoked ones; revoke 2 per owner if possible.
  const byOwner = { [ANNA_AH_HEX]: [], [DMO_AH_HEX]: [] };
  for (const p of liveProofs) if (byOwner[p.agent]) byOwner[p.agent].push(p.pid);
  console.log("live per owner:", { anna: byOwner[ANNA_AH_HEX], dmo: byOwner[DMO_AH_HEX] });

  for (const [ahHex, signer, label] of [[ANNA_AH_HEX, ANNA, "anna"], [DMO_AH_HEX, DMO, "dmo"]]) {
    const pids = (byOwner[ahHex] || []).slice(0, 2);
    for (const pid of pids) {
      const args = new Map([["proof_id", CLValue.newCLString(pid)]]);
      await send(signer, "revoke_proof", "proof_registry", args, `revoke pid=${pid} (owner=${label})`);
      await sleep(1000);
    }
  }

  console.log("waiting 60s for finality...");
  await sleep(60_000);
  console.log(`DONE. Report: ${REPORT}`);
})().catch((e) => { console.error("FATAL", e); log({ts: stamp(), fatal: e.message}); process.exit(1); });
