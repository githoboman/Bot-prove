"""CasperProver Python SDK.

Thin wrapper around the CasperProver REST API for Python agents.

Usage:
    from sdk.python_client import ProverClient

    client = ProverClient("http://localhost:9090")
    proof = client.submit("agent-1", b"input", b"output", b"model", "inference")
    ok = client.verify(proof["id"])
"""

from __future__ import annotations

import json
from typing import Any
from urllib.request import Request, urlopen
from urllib.error import HTTPError


class ProverClient:
    """Synchronous client for CasperProver API."""

    def __init__(self, base_url: str = "http://localhost:9090") -> None:
        self._base = base_url.rstrip("/")

    def health(self) -> dict[str, Any]:
        return self._get("/health")

    def submit(
        self,
        agent_id: str,
        input_data: bytes,
        output_data: bytes,
        model: bytes,
        use_case: str = "inference",
    ) -> dict[str, Any]:
        """Generate a new proof."""
        return self._post("/proofs", {
            "agent_id": agent_id,
            "input": input_data.decode(),
            "output": output_data.decode(),
            "model": model.decode(),
            "use_case": use_case,
        })

    def get(self, proof_id: str) -> dict[str, Any]:
        """Fetch proof by ID."""
        return self._get(f"/proofs/{proof_id}")

    def list_proofs(self) -> list[dict[str, Any]]:
        """List all proofs."""
        return self._get("/proofs")

    def verify(self, proof_id: str) -> bool:
        """Check if a proof is valid and not revoked."""
        p = self.get(proof_id)
        return p.get("valid", False) and not p.get("revoked", True)

    # -- internal --------------------------------------------------------

    def _get(self, path: str) -> Any:
        req = Request(f"{self._base}{path}")
        req.add_header("User-Agent", "CasperProver-SDK/0.3")
        try:
            with urlopen(req, timeout=30) as resp:
                return json.loads(resp.read())
        except HTTPError as e:
            raise RuntimeError(f"API {e.code}: {e.read().decode()}") from e

    def _post(self, path: str, body: dict) -> Any:
        data = json.dumps(body).encode()
        req = Request(f"{self._base}{path}", data=data, method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("User-Agent", "CasperProver-SDK/0.3")
        try:
            with urlopen(req, timeout=30) as resp:
                return json.loads(resp.read())
        except HTTPError as e:
            raise RuntimeError(f"API {e.code}: {e.read().decode()}") from e
