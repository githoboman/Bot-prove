"""Two-minute Python quickstart for the CasperProver API.

Run:
    python -m sdk.examples.python.quickstart

Requires:
    pip install requests

Env:
    CP_API_URL   base URL (default http://localhost:9090)
    CP_API_KEY   optional X-API-Key
"""
from __future__ import annotations

import json
import os
import sys
import time
from typing import Any

import requests

BASE = os.getenv("CP_API_URL", "http://localhost:9090")
API_KEY = os.getenv("CP_API_KEY")

HEADERS: dict[str, str] = {"Content-Type": "application/json"}
if API_KEY:
    HEADERS["X-API-Key"] = API_KEY


def _pretty(label: str, obj: Any) -> None:
    print(f"--- {label} ---")
    print(json.dumps(obj, indent=2, ensure_ascii=False))


def _call(method: str, path: str, body: Any = None) -> Any:
    url = f"{BASE}{path}"
    resp = requests.request(method, url, headers=HEADERS, json=body, timeout=15)
    if resp.status_code >= 400:
        print(f"error {resp.status_code}: {resp.text}", file=sys.stderr)
        sys.exit(1)
    ct = resp.headers.get("content-type", "")
    return resp.json() if "json" in ct else resp.text


def main() -> None:
    # 1. Health check first — makes failure obvious.
    health = _call("GET", "/health")
    _pretty("health", health)

    # 2. Submit a demo proof.
    proof_hash = "cafebabe" + f"{time.time_ns():016x}"
    submit = _call("POST", "/proofs", {"agent_id": "quickstart-py", "proof_hash": proof_hash})
    _pretty("submit_proof", submit)

    pid = submit.get("proof_id")
    if not pid:
        print("no proof_id returned", file=sys.stderr)
        sys.exit(1)

    # 3. Fetch it.
    got = _call("GET", f"/proofs/{pid}")
    _pretty("get_proof", got)

    # 4. Verify.
    ver = _call("POST", f"/proofs/{pid}/verify")
    _pretty("verify_proof", ver)

    print("quickstart OK.")


if __name__ == "__main__":
    main()
