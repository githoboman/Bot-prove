#!/usr/bin/env node
// smoke-call.mjs — call one entrypoint on a deployed contract via casper-js-sdk 5.0.12
// Prints gas cost + success. Sequential caller: designed to be driven by a wrapper.
//
// Usage:
//   smoke-call.mjs <secret-key.pem> <contract-hash-hex> <entry_point> <args-json>
//
// args-json example:
//   '{"proof_hash":"deadbeef","input_hash":"aa","output_hash":"bb","model_hash":"m1"}'
//
// Env:
//   CASPER_NODE, CASPER_CHAIN (defaults: casper testnet public rpc)
//   PAYMENT_MOTES (default 3_000_000_000 = 3 CSPR)

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const require = createRequire(import.meta.url);
const sdk = require(path.resolve(__dirname, "../frontend/node_modules/casper-js-sdk"));

const {
  ContractCallBuilder, HttpHandler, RpcClient,
  PrivateKey, KeyAlgorithm, PublicKey,
  Args, CLValue, ContractHash, AccountHash, Key,
} = sdk;

function die(msg) { console.error(`[smoke] ERROR: ${msg}`); process.exit(1); }

async function main() {
  const [, , keyPath, contractHex, entryPoint, argsJson] = process.argv;
  if (!keyPath || !contractHex || !entryPoint) {
    console.error("usage: smoke-call.mjs <secret.pem> <contract-hash-hex-64> <entry_point> [args-json]");
    process.exit(2);
  }
  if (!fs.existsSync(keyPath)) die(`key not found: ${keyPath}`);
  if (!/^[0-9a-fA-F]{64}$/.test(contractHex)) die(`bad contract hash: ${contractHex}`);

  const chainName = process.env.CASPER_CHAIN || "casper-test";
  const nodeUrl   = process.env.CASPER_NODE  || "https://node.testnet.casper.network/rpc";
  const payment   = Number(process.env.PAYMENT_MOTES || "3000000000"); // 3 CSPR default

  const pem = fs.readFileSync(keyPath, "utf8");
  const key = PrivateKey.fromPem(pem, KeyAlgorithm.SECP256K1);

  const rawArgs = argsJson ? JSON.parse(argsJson) : {};

  // Build Args from a spec map {name: {type, value}}
  //   simple usage: {"batch_id": "b1"}  → auto-string
  //   typed:        {"amount": {"type":"u512","value":"1000"}}
  const argMap = new Map();
  for (const [k, v] of Object.entries(rawArgs)) {
    let cl;
    if (v && typeof v === "object" && "type" in v) {
      const t = v.type;
      const val = v.value;
      if (t === "u64") cl = CLValue.newCLUint64(BigInt(val));
      else if (t === "u512") cl = CLValue.newCLUInt512(val);
      else if (t === "u32") cl = CLValue.newCLUInt32(Number(val));
      else if (t === "string") cl = CLValue.newCLString(String(val));
      else if (t === "account_hash") {
        const ah = AccountHash.newAccountHash(Buffer.from(val.replace(/^account-hash-/, ""), "hex"));
        cl = CLValue.newCLByteArray(ah.toBytes()); // fallback — most contracts read raw AccountHash
      }
      else die(`unknown arg type: ${t}`);
    } else if (typeof v === "string") {
      cl = CLValue.newCLString(v);
    } else if (typeof v === "number") {
      cl = CLValue.newCLUint64(BigInt(v));
    } else {
      die(`unhandled arg ${k}: ${JSON.stringify(v)}`);
    }
    argMap.set(k, cl);
  }

  const tx = new ContractCallBuilder()
    .byHash(contractHex)
    .entryPoint(entryPoint)
    .runtimeArgs(new Args(argMap))
    .from(key.publicKey)
    .chainName(chainName)
    .payment(payment)
    .build();
  tx.sign(key);

  const client = new RpcClient(new HttpHandler(nodeUrl));
  const startedAt = Date.now();
  const res = await client.putTransaction(tx);
  const hash =
    res?.transactionHash?.transactionV1?.hash ??
    res?.transactionHash?.hash ??
    res?.transactionHash ?? res?.hash ??
    JSON.stringify(res);
  const hashStr = typeof hash === "string" ? hash : JSON.stringify(hash);

  const waitMs = Number(process.env.WAIT_TIMEOUT_MS || "420000"); // 7 min default
  const deadline = Date.now() + waitMs;
  let final = null;
  while (Date.now() < deadline) {
    await new Promise(r => setTimeout(r, 4000));
    try {
      const info = await client.getTransaction(hashStr);
      const r0 = info?.executionInfo?.executionResult ?? info?.execution_info?.execution_result;
      if (r0) { final = { info, r0 }; break; }
    } catch (_) { /* pending */ }
  }
  if (!final) die(`timeout awaiting finality (tx=${hashStr})`);

  const err = final.r0?.errorMessage ?? final.r0?.error_message ?? null;
  // In Condor 2.x, gas cost is under executionResult.consumed / .cost / .payment
  const consumed = final.r0?.consumed ?? final.r0?.cost ?? final.r0?.payment?.amount ?? null;
  const block = final.info?.executionInfo?.blockHash ?? "?";
  const dur = Date.now() - startedAt;

  const record = {
    entry_point: entryPoint,
    contract: contractHex.slice(0, 10) + "...",
    signer: key.publicKey.toHex().slice(0, 12) + "...",
    tx_hash: hashStr,
    block: block?.toString?.().slice?.(0, 10) + "..." ?? block,
    consumed_motes: consumed?.toString?.() ?? consumed,
    consumed_cspr: consumed ? (Number(consumed) / 1e9).toFixed(4) : null,
    error: err,
    duration_ms: dur,
    ok: !err,
  };
  console.log(JSON.stringify(record));
  if (err) process.exit(3);
}

main().catch(e => die(e?.message || e));
