#!/usr/bin/env node
// mass-runner-errfix.mjs — third pass, targeted at the root causes of the
// 241 reverts in the first two passes. Runs on a separate branch and reports
// alongside prior runs — the two earlier passes stay intact.
//
// Root causes found in the on-chain report + contract source review:
//
//   1) SDK-client never parsed the pid returned by proof_registry.submit_proof
//      (contract mints `pid = "P-{next_id()}"`). All downstream calls that
//      reference it (verifier_gate.verify/batch_check, defi_mock.check_kyc,
//      proof_registry.revoke_proof) were fed the proof_hash instead → correct
//      ERR_NOT_FOUND reverts on the contract side.
//      FIX: read the `pctr` uref via query_global_state BEFORE + AFTER each
//      submit batch. next_id() is +1 pre-increment, so if pctr = N before,
//      the next submit gets P-{N+1}. We only need serial submits (which we
//      already do) to know the exact pid.
//
//   2) proof_registry.register_agent is per-caller-once → both signers need
//      exactly ONE successful register_agent. Second attempts correctly
//      revert with ERR_AGENT_EXISTS. We now check `agents` dict and only
//      register if absent.
//
//   3) defi_mock.grant_access is admin-only (installer = DMO). Anna cannot
//      call it — we only run it from DMO. Its `check_kyc` requires a valid
//      pid that verifier_gate.is_valid returns true for → real pid required.
//
//   4) proof_of_inference.verify_proof requires the caller to be a registered
//      verifier. We first call register_verifier (installer-only = DMO) to
//      whitelist both signers, then verify.
//
//   5) model_registry.register_model requires 64-hex model_hash. Already
//      fixed in pass 2; still-open reverts there were ERR_ALREADY_REGISTERED
//      on duplicate hashes → we now always generate a fresh hash per call.
//
// Total: ~110 tx across the six problem entry points. Two signers, serial
// per-signer, ~1 s between sends. Payment cap 3 CSPR per tx.

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
  PrivateKey, KeyAlgorithm, Args, CLValue, CLTypeString,
} = sdk;

const CHAIN = "casper-test";
const NODE = process.env.CASPER_NODE || "https://node.testnet.casper.network/rpc";
const ANNA = PrivateKey.fromPem(fs.readFileSync(process.env.ANNA_PEM || "/tmp/anna.pem", "utf8"), KeyAlgorithm.SECP256K1);
const DMO  = PrivateKey.fromPem(fs.readFileSync(process.env.DMO_PEM  || "/tmp/dmo.pem",  "utf8"), KeyAlgorithm.SECP256K1);
const ANNA_HEX = ANNA.publicKey.toHex();
const DMO_HEX  = DMO.publicKey.toHex();
const ANNA_AH = ANNA.publicKey.accountHash().toBytes();
const DMO_AH  = DMO.publicKey.accountHash().toBytes();

const ONCHAIN = JSON.parse(fs.readFileSync(path.resolve(__dirname, "../frontend/public/onchain.json"), "utf8"));
const PAYMENT = 3_000_000_000;
const SEND_GAP_MS = 1000;
const client = new RpcClient(new HttpHandler(NODE));

const ts = new Date().toISOString().replace(/[:.]/g, "-");
const REPORT_DIR = path.resolve(__dirname, "../reports");
if (!fs.existsSync(REPORT_DIR)) fs.mkdirSync(REPORT_DIR, { recursive: true });
const REPORT = path.join(REPORT_DIR, `mass-runner-errfix-${ts}.jsonl`);
const log = (o) => fs.appendFileSync(REPORT, JSON.stringify(o) + "\n");
const stamp = () => new Date().toISOString();

const randHex = (n = 32) => crypto.randomBytes(n).toString("hex");
const hex64 = () => randHex(32); // 64 hex chars
const sleep = (ms) => new Promise(r => setTimeout(r, ms));

// Contract hashes as bytes
function chHash(name) {
  return Uint8Array.from(Buffer.from(ONCHAIN.contracts[name].contract_hash, "hex"));
}

// Query pctr counter from proof-registry named keys
const PCTR_UREF = "uref-926696cf860ac012d31b38ca4b749d3efb210021e5ef7a83b677f744c5907218-007";
async function readPctr() {
  const res = await fetch(NODE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      jsonrpc: "2.0", id: 1, method: "query_global_state",
      params: { state_identifier: null, key: PCTR_UREF, path: [] },
    }),
  }).then(r => r.json());
  return res?.result?.stored_value?.CLValue?.parsed ?? null;
}

// Read agents dict to see if caller is already registered
const AGENTS_UREF = "uref-5399c66d0bce6b9bb8e787e7b0f1188539c46f54978c342faa2193bcdb46e6bc-007";
async function stateRootHash() {
  const r = await fetch(NODE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "chain_get_state_root_hash", params: {} }),
  }).then(r => r.json());
  return r?.result?.state_root_hash;
}

async function isAgentRegistered(callerAccountHashStr) {
  const srh = await stateRootHash();
  const res = await fetch(NODE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      jsonrpc: "2.0", id: 1, method: "state_get_dictionary_item",
      params: {
        state_root_hash: srh,
        dictionary_identifier: {
          URef: { seed_uref: AGENTS_UREF, dictionary_item_key: callerAccountHashStr },
        },
      },
    }),
  }).then(r => r.json());
  return !!res?.result?.stored_value;
}

async function send(signer, ep, contractName, argsMap, note) {
  const args = new Args(argsMap);
  const tx = new ContractCallBuilder()
    .byHash(ONCHAIN.contracts[contractName].contract_hash)
    .entryPoint(ep)
    .runtimeArgs(args)
    .from(signer.publicKey)
    .chainName(CHAIN)
    .payment(PAYMENT)
    .build();
  tx.sign(signer);
  try {
    const r = await client.putTransaction(tx);
    const hash = r?.transactionHash?.transactionV1?.toHex?.() || r?.transactionHash?.hash?.toHex?.() || String(r?.transactionHash || "");
    log({ ts: stamp(), signer: signer === ANNA ? "anna" : "dmo", contract: contractName, entryPoint: ep, note, tx_hash: hash, status: "sent" });
    return { ok: true, hash };
  } catch (e) {
    log({ ts: stamp(), signer: signer === ANNA ? "anna" : "dmo", contract: contractName, entryPoint: ep, note, error: e.message, status: "send_failed" });
    return { ok: false, error: e.message };
  }
}

// ------ RUN ------
(async () => {
  console.log(`report → ${REPORT}`);

  // 1) Ensure both agents are registered
  const annaAhStr = Buffer.from(ANNA_AH).toString("hex");
  const dmoAhStr = Buffer.from(DMO_AH).toString("hex");
  for (const [name, signer, ahStr] of [["anna", ANNA, annaAhStr], ["dmo", DMO, dmoAhStr]]) {
    // agents dict key = raw hex (Casper 2.x AccountHash::to_string in this contract returns raw hex).
    const key = ahStr;
    const registered = await isAgentRegistered(key);
    console.log(`[${name}] agent registered: ${registered}`);
    if (!registered) {
      const args = new Map([["agent_id", CLValue.newCLString(`agent_${name}_${Date.now()}`)], ["model_hash", CLValue.newCLString(hex64())]]);
      await send(signer, "register_agent", "proof_registry", args, `register_agent bootstrap`);
      await sleep(SEND_GAP_MS);
    }
  }

  // Wait for register_agent to land
  console.log("waiting 45s for register_agent finality...");
  await sleep(45_000);

  // 2) Register both signers as verifiers on proof_of_inference (installer=DMO)
  //    verifier_id must equal what verify_proof will compute: caller.to_string()
  //    → raw hex of account hash (NOT public key hex).
  for (const [name, signer, pubHex, ahHex] of [
    ["anna", ANNA, ANNA_HEX, annaAhStr],
    ["dmo",  DMO,  DMO_HEX,  dmoAhStr],
  ]) {
    const args = new Map([
      ["verifier_id", CLValue.newCLString(ahHex)],
      ["pub_key", CLValue.newCLString(pubHex)],
    ]);
    // Try — will revert with ERR_VERIFIER_EXISTS if already registered. Logged either way.
    await send(DMO, "register_verifier", "proof_of_inference", args, `register_verifier ${name}`);
    await sleep(SEND_GAP_MS);
  }

  console.log("waiting 30s for register_verifier finality...");
  await sleep(30_000);

  // 3) Read pctr, submit proofs serially, capture the exact P-{n} we will produce.
  const beforePctr = await readPctr();
  console.log(`pctr before submits: ${beforePctr}`);

  // Serial submits: each submit_proof mints pid = "P-{beforePctr + i + 1}" (next_id pre-increments)
  // Do 8 submits total (4 per signer, alternating) → known pids for downstream calls.
  const submits = [];
  for (let i = 0; i < 8; i++) {
    const signer = i % 2 === 0 ? ANNA : DMO;
    const args = new Map([["proof_hash", CLValue.newCLString(hex64())], ["input_hash", CLValue.newCLString(hex64())], ["output_hash", CLValue.newCLString(hex64())], ["model_hash", CLValue.newCLString(hex64())]]);
    const r = await send(signer, "submit_proof", "proof_registry", args, `submit_for_verify#${i}`);
    const expectedPid = `P-${beforePctr + i + 1}`;
    submits.push({ signer: signer === ANNA ? "anna" : "dmo", expectedPid, hash: r.hash });
    await sleep(SEND_GAP_MS);
  }

  console.log("waiting 60s for submits to land...");
  await sleep(60_000);

  const afterPctr = await readPctr();
  console.log(`pctr after submits: ${afterPctr}  (expected: ${beforePctr + 8}, got same? ${afterPctr === beforePctr + 8})`);

  // If afterPctr diverges (parallel activity elsewhere), our expectedPid mapping is off.
  // But even with drift, the range [beforePctr+1 .. afterPctr] is the set of pids that
  // include ours. We'll trust the serial-ordering assumption on testnet.

  // 4) Downstream calls with REAL pids
  const realPids = submits.map(s => s.expectedPid);
  console.log(`will use pids: ${realPids.join(", ")}`);

  // 4a) verifier_gate.verify — Anna verifies pids 0,2,4,6 (Anna-submitted); DMO 1,3,5,7
  for (let i = 0; i < realPids.length; i++) {
    const pid = realPids[i];
    // Both signers can call verify (no ownership check on verify_gate)
    const signer = i % 2 === 0 ? ANNA : DMO;
    const args = new Map([["proof_id", CLValue.newCLString(pid)]]);
    await send(signer, "verify", "verifier_gate", args, `verify pid=${pid}`);
    await sleep(SEND_GAP_MS);
  }

  // 4b) verifier_gate.batch_check with all real pids × 2 (both signers)
  for (const signer of [ANNA, DMO]) {
    const args = new Map([["proof_ids", CLValue.newCLList(CLTypeString, realPids.map(p => CLValue.newCLString(p)))]]);
    await send(signer, "batch_check", "verifier_gate", args, `batch_check ${realPids.length} pids`);
    await sleep(SEND_GAP_MS);
  }

  // 4c) defi_mock.check_kyc — both signers can call
  for (let i = 0; i < realPids.length; i++) {
    const pid = realPids[i];
    const signer = i % 2 === 0 ? ANNA : DMO;
    const args = new Map([["proof_id", CLValue.newCLString(pid)]]);
    await send(signer, "check_kyc", "defi_mock", args, `check_kyc pid=${pid}`);
    await sleep(SEND_GAP_MS);
  }

  // 4d) defi_mock.grant_access — DMO only (admin), grant unique users
  // Use dummy account hashes (32 bytes) — the contract only stores them, doesn't verify existence.
  for (let i = 0; i < 4; i++) {
    const pid = realPids[i];
    const dummyUser = Uint8Array.from(crypto.randomBytes(32));
    const args = new Map([["user", CLValue.newCLByteArray(dummyUser)], ["proof_id", CLValue.newCLString(pid)]]);
    await send(DMO, "grant_access", "defi_mock", args, `grant_access user=${Buffer.from(dummyUser).toString("hex").slice(0,12)}... pid=${pid}`);
    await sleep(SEND_GAP_MS);
  }

  // Wait — grant_access needs to land before revoke can succeed against it
  console.log("waiting 30s before revoke...");
  await sleep(30_000);

  // 4e) proof_registry.revoke_proof — caller must equal the agent who submitted it.
  // Anna submitted pids at even indices (0,2,4,6); DMO odd. Revoke 2 per signer.
  for (let i = 0; i < 4; i++) {
    const pid = realPids[i];
    const signer = i % 2 === 0 ? ANNA : DMO;
    const args = new Map([["proof_id", CLValue.newCLString(pid)]]);
    await send(signer, "revoke_proof", "proof_registry", args, `revoke pid=${pid}`);
    await sleep(SEND_GAP_MS);
  }

  // 5) proof_of_inference.register_proof + verify_proof with real pid
  // register_proof mints its own internal proof_id (starts at 0). We need to
  // read proof_counter for proof_of_inference to know what our register will yield.
  const POI_CH = ONCHAIN.contracts.proof_of_inference.contract_hash;
  // Query named keys to find proof_counter uref
  const nkRes = await fetch(NODE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      jsonrpc: "2.0", id: 1, method: "query_global_state",
      params: { state_identifier: null, key: `hash-${POI_CH}`, path: [] },
    }),
  }).then(r => r.json());
  const poiNks = nkRes?.result?.stored_value?.Contract?.named_keys || [];
  const poiPctrKey = poiNks.find(k => k.name === "proof_counter")?.key;
  let poiPctr = null;
  if (poiPctrKey) {
    const pRes = await fetch(NODE, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        jsonrpc: "2.0", id: 1, method: "query_global_state",
        params: { state_identifier: null, key: poiPctrKey, path: [] },
      }),
    }).then(r => r.json());
    poiPctr = pRes?.result?.stored_value?.CLValue?.parsed;
    console.log(`proof_of_inference proof_counter before register: ${poiPctr}`);
  }

  const poiRegistered = [];
  for (let i = 0; i < 4; i++) {
    const signer = i % 2 === 0 ? ANNA : DMO;
    const args = new Map([
      ["model_hash", CLValue.newCLString(hex64())],
      ["input_hash", CLValue.newCLString(hex64())],
      ["output_hash", CLValue.newCLString(hex64())],
      ["proof_hash", CLValue.newCLString(hex64())],
      ["agent_id", CLValue.newCLString(signer === ANNA ? "agent_anna" : "agent_dmo")],
      ["price_bps", CLValue.newCLUint64(BigInt(100 + i))],
    ]);
    // proof_of_inference register_proof mints proof_id = counter (0,1,2,...) then increments
    const expectedPid = String((poiPctr ?? 0) + i);
    poiRegistered.push({ pid: expectedPid, signer: signer === ANNA ? "anna" : "dmo" });
    await send(signer, "register_proof", "proof_of_inference", args, `register_proof pid=${expectedPid}`);
    await sleep(SEND_GAP_MS);
  }

  console.log("waiting 45s for register_proof finality before verify_proof...");
  await sleep(45_000);

  // verify_proof — caller must be a registered verifier. Both were registered above.
  for (const rec of poiRegistered) {
    const signer = rec.signer === "anna" ? ANNA : DMO;
    const args = new Map([["proof_id", CLValue.newCLString(rec.pid)]]);
    await send(signer, "verify_proof", "proof_of_inference", args, `verify_proof pid=${rec.pid}`);
    await sleep(SEND_GAP_MS);
  }

  // 6) model_registry — fresh unique 64-hex hashes so no ERR_ALREADY_REGISTERED
  for (let i = 0; i < 8; i++) {
    const signer = i % 2 === 0 ? ANNA : DMO;
    const mh = hex64();
    const args = new Map([["model_hash", CLValue.newCLString(mh)], ["name", CLValue.newCLString(`errfix_model_${i}`)], ["version", CLValue.newCLString(`1.0.${i}`)], ["ipfs_cid", CLValue.newCLString(`Qm${randHex(22)}`)]]);
    await send(signer, "register_model", "model_registry", args, `register_model mh=${mh.slice(0,12)}...`);
    await sleep(SEND_GAP_MS);
  }

  console.log("waiting 60s for final finality...");
  await sleep(60_000);

  console.log(`\nDONE. Report: ${REPORT}`);
})().catch((e) => {
  console.error("FATAL", e);
  log({ ts: stamp(), fatal: e.message });
  process.exit(1);
});
