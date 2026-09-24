#!/usr/bin/env python3
"""Read-only CasperProver judge demo; live writes require explicit opt-in.

Contract hashes, API URL, and frontend URL are loaded from the canonical
on-chain manifest at ``deploy-out/onchain.json`` (Gate 1.5). Falls back to
last-known values only if the manifest is not present next to the script.
"""
from __future__ import annotations
import argparse, json, os, sys, urllib.error, urllib.request
from dataclasses import dataclass
from pathlib import Path

_FALLBACK_CONTRACTS = {
    "Proof Registry": "96e97c4d564fe7374ba4e938355fb89f5be2f448decbe9b7727bd3c978a10708",
    "Verifier Gate": "a37f9cde9dbdc5bb8b9e92c663bdc59b83b42c89dc75ec73f7f7cde2619f77d3",
    "DeFi Mock": "fe0c45f67c8cd99f0bda0047399a113588870ec0d79d9102f44107303f0b39ef",
    "Stake Slashing": "1ad1b3d94be631532d6daf3a195fafc9dfe8a16504e87d87784d51089b983d52",
    "Proof Aggregation": "b29f32abcc029d523de212bd7c87993f2f1bf96ba1523091c7b01adf6d63d2bb",
    "Model Registry": "b3cdd1df25714b341e34f6bb29f6c7900267e44c7742c81221e1eab5e64a340a",
    "Proof of Inference": "3d772fe1618fde438c4ffdaec22d83ffd9b4a1d769d6da32a38d56f12498b318",
    "Governance": "38d2fbd24998719fac160c27e2e5435a99bcdebd4c36beac76abe84063a0cf3e",
    "ZK Verifier": "4a5d09419fbc147e4114adb2e50473addd5d7057ae58c6223e0edaa4fb89a262",
}
_FALLBACK_API = "https://casperprover-api-ylsh.onrender.com"
_FALLBACK_SITE = "https://casperprover.xyz"
RPC = "https://node.testnet.casper.network/rpc"

_MANIFEST_KEYS = {
    "Proof Registry": "proof_registry",
    "Verifier Gate": "verifier_gate",
    "DeFi Mock": "defi_mock",
    "Stake Slashing": "stake_slashing",
    "Proof Aggregation": "proof_aggregation",
    "Model Registry": "model_registry",
    "Proof of Inference": "proof_of_inference",
    "Governance": "governance",
    "ZK Verifier": "zk_verifier",
}


def _load_manifest() -> dict | None:
    override = os.environ.get("CP_MANIFEST_PATH")
    candidates: list[Path] = []
    if override:
        candidates.append(Path(override))
    here = Path(__file__).resolve()
    for base in (here.parent.parent, here.parent, Path.cwd()):
        candidates.append(base / "deploy-out" / "onchain.json")
    for c in candidates:
        try:
            with c.open("r", encoding="utf-8") as f:
                return json.load(f)
        except FileNotFoundError:
            continue
        except json.JSONDecodeError:
            continue
    return None


_MANIFEST = _load_manifest()


def _contracts_from_manifest() -> dict[str, str]:
    if not _MANIFEST:
        return dict(_FALLBACK_CONTRACTS)
    resolved: dict[str, str] = {}
    for label, key in _MANIFEST_KEYS.items():
        entry = _MANIFEST.get("contracts", {}).get(key)
        if entry and isinstance(entry.get("contract_hash"), str):
            resolved[label] = entry["contract_hash"]
        else:
            resolved[label] = _FALLBACK_CONTRACTS[label]
    return resolved


def _api_from_manifest() -> str:
    if _MANIFEST:
        v = _MANIFEST.get("verification", {}) or {}
        h = v.get("api_health")
        if isinstance(h, str) and h.endswith("/health"):
            return h[: -len("/health")]
        if isinstance(h, str):
            return h.rstrip("/")
    return _FALLBACK_API


def _site_from_manifest() -> str:
    if _MANIFEST:
        v = _MANIFEST.get("verification", {}) or {}
        f = v.get("frontend")
        if isinstance(f, str) and f:
            return f.rstrip("/")
    return _FALLBACK_SITE


DEFAULT_API = _api_from_manifest()
DEFAULT_SITE = _site_from_manifest()
CONTRACTS = _contracts_from_manifest()

@dataclass
class Result:
    name: str
    ok: bool
    detail: str

class Client:
    def __init__(self, timeout: float = 20): self.timeout = timeout
    def request(self, url: str, *, method="GET", payload=None, api_key=""):
        data = json.dumps(payload).encode() if payload is not None else None
        headers = {"Accept": "application/json", "User-Agent": "CasperProver-Judge-Demo/1.0"}
        if data: headers["Content-Type"] = "application/json"
        if api_key: headers["X-API-Key"] = api_key
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        with urllib.request.urlopen(req, timeout=self.timeout) as response:
            body = response.read().decode("utf-8", errors="replace")
            return response.status, json.loads(body) if body else {}

def contract_checks(client: Client) -> list[Result]:
    out = []
    for name, contract_hash in CONTRACTS.items():
        payload = {"jsonrpc":"2.0","id":1,"method":"query_global_state","params":{"state_identifier":None,"key":f"hash-{contract_hash}","path":[]}}
        try:
            _, body = client.request(RPC, method="POST", payload=payload)
            out.append(Result(name, "result" in body, f"{contract_hash[:16]}… on Casper testnet" if "result" in body else str(body.get("error", "not found"))))
        except Exception as exc: out.append(Result(name, False, friendly_error(exc)))
    return out

def read_checks(client: Client, api: str, site: str) -> list[Result]:
    checks = []
    try:
        _, body = client.request(f"{api}/health")
        checks.append(Result("API health", body.get("status") == "ok", f"version {body.get('version', 'unknown')}"))
    except Exception as exc: checks.append(Result("API health", False, friendly_error(exc)))
    try:
        _, body = client.request(f"{api}/proofs")
        count = len(body) if isinstance(body, list) else len(body.get("proofs", []))
        checks.append(Result("Proof registry API", True, f"{count} proof record(s) readable"))
    except Exception as exc: checks.append(Result("Proof registry API", False, friendly_error(exc)))
    try:
        req = urllib.request.Request(site, headers={"User-Agent":"CasperProver-Judge-Demo/1.0"})
        with urllib.request.urlopen(req, timeout=client.timeout) as response:
            text = response.read(200_000).decode(errors="replace").lower()
        checks.append(Result("Frontend", "casperprover" in text, site))
    except Exception as exc: checks.append(Result("Frontend", False, friendly_error(exc)))
    return checks

def crypto_checks(client: Client, api: str, api_key: str) -> list[Result]:
    if not api_key:
        return [Result("Real Groth16 round-trip", True, "SKIP — set CP_JUDGE_API_KEY; no hidden/default key")]
    try:
        _, proof = client.request(f"{api}/zk/groth16-real/prove", method="POST", payload={"preimage":"42"}, api_key=api_key)
        request = {"hash": proof["hash"], "proof_hex": proof["proof_hex"]}
        _, verified = client.request(f"{api}/zk/groth16-real/verify", method="POST", payload=request, api_key=api_key)
        ok = bool(verified.get("valid"))
        return [Result("Real Groth16 round-trip", ok, "real off-chain gnark/BN254 MiMC prove + verify; not on-chain pairing verification")]
    except Exception as exc: return [Result("Real Groth16 round-trip", False, friendly_error(exc))]

def friendly_error(exc: Exception) -> str:
    if isinstance(exc, urllib.error.HTTPError): return f"HTTP {exc.code} {exc.reason}"
    if isinstance(exc, urllib.error.URLError): return f"network error: {exc.reason}"
    return f"{type(exc).__name__}: {exc}"

def print_result(r: Result):
    skipped = r.detail.startswith("SKIP")
    status = "SKIP" if skipped else ("PASS" if r.ok else "FAIL")
    color = "\033[33m" if skipped else ("\033[32m" if r.ok else "\033[31m")
    print(f"{color}[{status}]\033[0m {r.name}: {r.detail}")

def main() -> int:
    p = argparse.ArgumentParser(description="Reproducible, read-only-by-default CasperProver judge demo")
    p.add_argument("--api", default=DEFAULT_API); p.add_argument("--site", default=DEFAULT_SITE)
    p.add_argument("--api-key", default=os.getenv("CP_JUDGE_API_KEY", ""), help="or set CP_JUDGE_API_KEY; never committed")
    p.add_argument("--timeout", type=float, default=20); p.add_argument("--json", action="store_true")
    args = p.parse_args(); client = Client(args.timeout)
    print(r"""[36m
   ____                          ____
  / ___|__ _ ___ _ __   ___ _ _|  _ \ _ __ _____   _____ _ __
 | |   / _` / __| '_ \ / _ \ '__| |_) | '__/ _ \ \ / / _ \ '__|
 | |__| (_| \__ \ |_) |  __/ |  |  __/| | | (_) \ V /  __/ |
  \____\__,_|___/ .__/ \___|_|  |_|   |_|  \___/ \_/ \___|_|
                |_|       JUDGE VERIFICATION[0m""")
    results = contract_checks(client) + read_checks(client, args.api.rstrip('/'), args.site.rstrip('/')) + crypto_checks(client, args.api.rstrip('/'), args.api_key)
    if args.json: print(json.dumps([r.__dict__ for r in results], indent=2))
    else:
        for result in results: print_result(result)
        print("\nProof boundary: REAL CRYPTO = off-chain gnark/BN254 MiMC; ON-CHAIN = Casper hashes/registries; SIMULATION = legacy conceptual endpoints.")
        print("Docs: https://github.com/anna-stolbovskaja/CasperProver#readme")
    failures = sum(not r.ok for r in results)
    skipped = sum(r.detail.startswith("SKIP") for r in results)
    passed = len(results) - failures - skipped
    print(f"\nSummary: {passed} passed, {skipped} skipped, {failures} failed")
    return 1 if failures else 0

if __name__ == "__main__": raise SystemExit(main())
