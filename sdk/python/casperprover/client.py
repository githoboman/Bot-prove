"""CasperProver Python SDK - core client.

Thin stdlib-only wrapper around the CasperProver v1 REST API. Feature parity
with the Go SDK at sdk/primitives.go: `prove`, `verify`, `batch`, `anchor`,
plus per-request idempotency-key support.

The client is intentionally synchronous. Async wrappers can be added later
without breaking the surface.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field, asdict
from typing import Any, Dict, Iterable, List, Mapping, Optional
from urllib.error import HTTPError
from urllib.request import Request, urlopen


DEFAULT_BASE_URL = "http://localhost:9090"
DEFAULT_TIMEOUT = 30.0


class ApiError(Exception):
    """Raised when the CasperProver API returns a non-2xx status."""

    def __init__(self, status: int, body: str, message: str = "") -> None:
        self.status = status
        self.body = body
        super().__init__(message or f"api error (status {status}): {body}")


@dataclass
class ProveRequest:
    agent: str = ""
    input: str = ""
    output: str = ""
    model: str = ""
    use_case: str = ""
    mode: str = ""

    def to_json(self) -> Dict[str, Any]:
        # Match the Go SDK's json tag names (snake_case, omitempty semantics).
        m: Dict[str, Any] = {"agent": self.agent, "input": self.input,
                             "output": self.output, "model": self.model}
        if self.use_case:
            m["use_case"] = self.use_case
        if self.mode:
            m["mode"] = self.mode
        return m


@dataclass
class ProveResponse:
    id: str = ""
    proof_hash: str = ""
    vk_hash: str = ""
    input_hash: str = ""
    output_hash: str = ""
    model_hash: str = ""
    verdict: str = ""
    created_at: str = ""
    raw: Dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_json(cls, data: Mapping[str, Any]) -> "ProveResponse":
        return cls(
            id=str(data.get("id", "")),
            proof_hash=str(data.get("proof_hash", "")),
            vk_hash=str(data.get("vk_hash", "")),
            input_hash=str(data.get("input_hash", "")),
            output_hash=str(data.get("output_hash", "")),
            model_hash=str(data.get("model_hash", "")),
            verdict=str(data.get("verdict", "")),
            created_at=str(data.get("created_at", "")),
            raw=dict(data),
        )


@dataclass
class VerifyResponse:
    valid: bool = False
    proof_id: str = ""
    reason: str = ""
    raw: Dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_json(cls, data: Mapping[str, Any]) -> "VerifyResponse":
        return cls(
            valid=bool(data.get("valid", False)),
            proof_id=str(data.get("proof_id", "")),
            reason=str(data.get("reason", "")),
            raw=dict(data),
        )


@dataclass
class BatchItem:
    proof_id: str = ""
    model_id: str = ""
    input: str = ""
    output: str = ""
    extra: Dict[str, Any] = field(default_factory=dict)

    def to_json(self) -> Dict[str, Any]:
        m: Dict[str, Any] = {}
        if self.proof_id:
            m["proof_id"] = self.proof_id
        if self.model_id:
            m["model_id"] = self.model_id
        if self.input:
            m["input"] = self.input
        if self.output:
            m["output"] = self.output
        for k, v in self.extra.items():
            m.setdefault(k, v)
        return m


@dataclass
class BatchResponse:
    verified: List[str] = field(default_factory=list)
    failed: List[str] = field(default_factory=list)
    total: int = 0
    mode: str = ""
    raw: Dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_json(cls, data: Mapping[str, Any]) -> "BatchResponse":
        return cls(
            verified=list(data.get("verified", []) or []),
            failed=list(data.get("failed", []) or []),
            total=int(data.get("total", 0) or 0),
            mode=str(data.get("mode", "")),
            raw=dict(data),
        )


@dataclass
class AnchorResponse:
    proof_id: str = ""
    tx_hash: str = ""
    block_hash: str = ""
    anchored_at: str = ""
    strict_mode: bool = False
    deployer_key_id: str = ""
    raw: Dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_json(cls, data: Mapping[str, Any]) -> "AnchorResponse":
        return cls(
            proof_id=str(data.get("proof_id", "")),
            tx_hash=str(data.get("tx_hash", "")),
            block_hash=str(data.get("block_hash", "")),
            anchored_at=str(data.get("anchored_at", "")),
            strict_mode=bool(data.get("strict_mode", False)),
            deployer_key_id=str(data.get("deployer_key_id", "")),
            raw=dict(data),
        )


class Client:
    """Synchronous CasperProver v1 client.

    Args:
        base_url: API base URL. Defaults to http://localhost:9090.
        api_key: Value sent as ``X-API-Key``. Only required if the server
            was started with ``API_KEY`` set.
        api_version: Path prefix; "v1" by default. Pass "" for legacy routes.
        timeout: Per-request timeout in seconds.
    """

    def __init__(
        self,
        base_url: str = DEFAULT_BASE_URL,
        api_key: Optional[str] = None,
        api_version: str = "v1",
        timeout: float = DEFAULT_TIMEOUT,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.api_version = api_version.strip("/")
        self.timeout = timeout

    # ---- transport ----------------------------------------------------

    def _prefix(self) -> str:
        return f"/{self.api_version}" if self.api_version else ""

    def _request(
        self,
        method: str,
        path: str,
        body: Optional[Any] = None,
        *,
        idempotency_key: Optional[str] = None,
        headers: Optional[Mapping[str, str]] = None,
    ) -> Dict[str, Any]:
        data: Optional[bytes] = None
        req_headers: Dict[str, str] = {}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            req_headers["Content-Type"] = "application/json"
        if self.api_key:
            req_headers["X-API-Key"] = self.api_key
        if idempotency_key:
            req_headers["X-Idempotency-Key"] = idempotency_key
        if headers:
            req_headers.update(headers)

        req = Request(self.base_url + path, data=data, method=method,
                      headers=req_headers)
        try:
            with urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read()
        except HTTPError as e:
            raw = e.read()
            raise ApiError(e.code, raw.decode("utf-8", errors="replace")) from e

        if not raw:
            return {}
        return json.loads(raw)

    # ---- primitives ---------------------------------------------------

    def health(self) -> Dict[str, Any]:
        return self._request("GET", "/health")

    def prove(
        self,
        req: ProveRequest,
        *,
        idempotency_key: Optional[str] = None,
        headers: Optional[Mapping[str, str]] = None,
    ) -> ProveResponse:
        raw = self._request(
            "POST",
            f"{self._prefix()}/proofs",
            req.to_json(),
            idempotency_key=idempotency_key,
            headers=headers,
        )
        return ProveResponse.from_json(raw)

    def verify(
        self,
        proof_id: str,
        *,
        idempotency_key: Optional[str] = None,
        headers: Optional[Mapping[str, str]] = None,
    ) -> VerifyResponse:
        raw = self._request(
            "POST",
            f"{self._prefix()}/verify",
            {"proof_id": proof_id},
            idempotency_key=idempotency_key,
            headers=headers,
        )
        return VerifyResponse.from_json(raw)

    def batch(
        self,
        proofs: Iterable[BatchItem],
        mode: str = "",
        *,
        idempotency_key: Optional[str] = None,
        headers: Optional[Mapping[str, str]] = None,
    ) -> BatchResponse:
        body: Dict[str, Any] = {"proofs": [p.to_json() for p in proofs]}
        if mode:
            body["mode"] = mode
        raw = self._request(
            "POST",
            f"{self._prefix()}/batch/verify-zk",
            body,
            idempotency_key=idempotency_key,
            headers=headers,
        )
        return BatchResponse.from_json(raw)

    def anchor(
        self,
        proof_id: str,
        *,
        idempotency_key: Optional[str] = None,
        headers: Optional[Mapping[str, str]] = None,
    ) -> AnchorResponse:
        raw = self._request(
            "POST",
            f"{self._prefix()}/proofs/{proof_id}/anchor",
            None,
            idempotency_key=idempotency_key,
            headers=headers,
        )
        return AnchorResponse.from_json(raw)
