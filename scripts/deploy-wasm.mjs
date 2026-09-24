#!/usr/bin/env node
// deploy-wasm.mjs — deploy one shrunk .wasm contract to Casper testnet via
// casper-js-sdk 5.0.12. Uses SessionBuilder (module-bytes session code) with
// a targeted payment amount and prints the deploy hash + wait for finality.
//
// Usage:
//   scripts/deploy-wasm.mjs <path-to-contract.wasm> <secret-key.pem> [payment-motes]
//
// Env:
//   CASPER_NODE=<rpc-url>   default https://node.testnet.cspr.cloud/rpc
//   CASPER_CHAIN=<name>     default casper-test
//
// The script fails FAST with a clear message on:
//   * missing wasm / key
//   * >65_536 byte wasm (would be rejected by installOrUpgrade)
//   * unresolved put_deploy
//
// This is a session-code deploy (single-shot execution of the wasm's `call`
// entry point). It matches how the four already-deployed CP contracts were
// installed. After success, run the CP entry-point smoke test separately.

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const require = createRequire(import.meta.url);

// Reach into frontend/node_modules for casper-js-sdk 5.0.12.
const sdk = require(path.resolve(__dirname, "../frontend/node_modules/casper-js-sdk"));

const {
  SessionBuilder,
  HttpHandler,
  RpcClient,
  PrivateKey,
  KeyAlgorithm,
  Args,
} = sdk;

const CAP = 65536;

async function main() {
  const [, , wasmPath, keyPath, paymentRaw] = process.argv;
  if (!wasmPath || !keyPath) {
    console.error("usage: deploy-wasm.mjs <contract.wasm> <secret-key.pem> [payment-motes]");
    process.exit(2);
  }
  if (!fs.existsSync(wasmPath)) die(`wasm not found: ${wasmPath}`);
  if (!fs.existsSync(keyPath))  die(`key  not found: ${keyPath}`);

  const wasm = fs.readFileSync(wasmPath);
  const size = wasm.length;
  console.log(`[deploy] wasm=${wasmPath}  size=${size} bytes  cap=${CAP}`);
  if (size > CAP) die(`wasm ${size}B exceeds installOrUpgrade cap ${CAP}B; run wasm-opt -Oz first`);

  const paymentMotes = Number(paymentRaw || "150000000000"); // 150 CSPR default
  if (!Number.isFinite(paymentMotes)) die(`invalid payment: ${paymentRaw}`);
  console.log(`[deploy] payment=${paymentMotes} motes`);

  const chainName = process.env.CASPER_CHAIN || "casper-test";
  const nodeUrl   = process.env.CASPER_NODE  || "https://node.testnet.cspr.cloud/rpc";
  const apiKey    = process.env.CSPR_CLOUD_API_KEY || "";
  console.log(`[deploy] chain=${chainName}  node=${nodeUrl}${apiKey?"  auth=on":""}`);

  const pem = fs.readFileSync(keyPath, "utf8");
  const key = PrivateKey.fromPem(pem, KeyAlgorithm.SECP256K1);
  console.log(`[deploy] signer=${key.publicKey.toHex()}`);

  // Build the deploy: session = module bytes, deploy body = the wasm.
  // NB casper-js-sdk 5.0.12 API is chainable:
  //   .wasm(bytes).installOrUpgrade().runtimeArgs({})
  //   .from(pk).chainName(...).payment(...).build()
  const deploy = new SessionBuilder()
    .wasm(wasm)
    .installOrUpgrade()
    .runtimeArgs(new Args(new Map()))
    .from(key.publicKey)
    .chainName(chainName)
    .payment(paymentMotes)
    .build();
  deploy.sign(key);

  const http = new HttpHandler(nodeUrl);
  if (apiKey) http.setCustomHeaders({ Authorization: apiKey });
  const rpc  = new RpcClient(http);

  const res = await rpc.putTransaction(deploy);
  const hash = (res && (res.transactionHash || res.transaction_hash || res.hash)) || JSON.stringify(res);
  console.log(`[deploy] submitted tx=${JSON.stringify(hash)}`);
  console.log(`[deploy] explorer: https://testnet.cspr.live/transaction/${typeof hash === 'string' ? hash : ''}`);

  try {
    const finalized = await rpc.waitForTransaction(deploy, 120_000);
    console.log(`[deploy] finalized`);
    console.log(JSON.stringify(finalized, null, 2).slice(0, 2000));
    console.log(`[deploy] SUCCESS`);
  } catch (e) {
    die(`timed out or failed: ${(e && e.message) || e}`);
  }
}

function die(msg) {
  console.error(`[deploy] ERROR: ${msg}`);
  process.exit(1);
}
function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

main().catch(e => die((e && e.stack) || e));
