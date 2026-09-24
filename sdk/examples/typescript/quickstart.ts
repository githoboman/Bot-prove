// Two-minute TypeScript / Node quickstart for the CasperProver API.
//
// Run (Node 20+):
//     tsx sdk/examples/typescript/quickstart.ts
//     # or
//     ts-node sdk/examples/typescript/quickstart.ts
//
// Env:
//     CP_API_URL   base URL (default http://localhost:9090)
//     CP_API_KEY   optional X-API-Key

const BASE = process.env.CP_API_URL ?? "http://localhost:9090";
const API_KEY = process.env.CP_API_KEY;

const headers: Record<string, string> = { "Content-Type": "application/json" };
if (API_KEY) headers["X-API-Key"] = API_KEY;

async function call<T = unknown>(method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`${method} ${path} -> ${resp.status}: ${text}`);
  }
  const ct = resp.headers.get("content-type") ?? "";
  return ct.includes("json") ? ((await resp.json()) as T) : ((await resp.text()) as unknown as T);
}

function pretty(label: string, obj: unknown): void {
  console.log(`--- ${label} ---`);
  console.log(JSON.stringify(obj, null, 2));
}

async function main(): Promise<void> {
  // 1. Health check.
  const health = await call("GET", "/health");
  pretty("health", health);

  // 2. Submit a proof.
  const proofHash = "beefdead" + Date.now().toString(16).padStart(16, "0");
  const submit = await call<{ proof_id?: string }>("POST", "/proofs", {
    agent_id: "quickstart-ts",
    proof_hash: proofHash,
  });
  pretty("submit_proof", submit);

  const pid = submit.proof_id;
  if (!pid) throw new Error("no proof_id returned");

  // 3. Get.
  const got = await call("GET", `/proofs/${pid}`);
  pretty("get_proof", got);

  // 4. Verify.
  const ver = await call("POST", `/proofs/${pid}/verify`);
  pretty("verify_proof", ver);

  console.log("quickstart OK.");
}

main().catch((err) => {
  console.error("quickstart failed:", err);
  process.exit(1);
});
