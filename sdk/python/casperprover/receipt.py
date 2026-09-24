"""Client-side proof-receipt validator.

Bit-for-bit compatible with the Go implementation in sdk/receipt.go and the
TypeScript implementation in sdk/typescript/src/receipt.ts. The three
implementations share the same tampering fixtures (see sdk/testdata).
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any, Dict, Mapping, Optional, Union


@dataclass
class ProofReceipt:
    id: str = ""
    agent: str = ""
    model: str = ""
    input: str = ""
    output: str = ""
    use_case: str = ""
    proof_hash: str = ""
    vk_hash: str = ""
    input_hash: str = ""
    output_hash: str = ""
    model_hash: str = ""
    verdict: str = ""
    created_at: str = ""
    raw: Dict[str, Any] = field(default_factory=dict)


class ReceiptValidationError(Exception):
    """Raised when a receipt field does not re-derive to the expected hash."""

    def __init__(self, field_name: str, expected: str, actual: str) -> None:
        self.field_name = field_name
        self.expected = expected
        self.actual = actual
        super().__init__(
            f"receipt field {field_name!r} mismatch: "
            f"expected {expected}, got {actual}"
        )


def hash_field(value: str) -> str:
    """Return canonical lowercase hex SHA-256 of the UTF-8 encoding of `value`.

    This matches Go's `HashField` and the TS `hashField` character for
    character.
    """
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _normalize_hex(h: str) -> str:
    return h.lower().removeprefix("0x")


def _hex_equal(a: str, b: str) -> bool:
    na, nb = _normalize_hex(a), _normalize_hex(b)
    if len(na) != len(nb):
        return False
    for ch in na + nb:
        if not (("0" <= ch <= "9") or ("a" <= ch <= "f")):
            return False
    return na == nb


def verify_receipt(data: Mapping[str, Any]) -> ProofReceipt:
    """Validate a parsed receipt dict and return a ProofReceipt.

    Missing hashes are tolerated (partial receipts). Any hash the server
    supplied that does not re-derive from the plaintext raises
    ReceiptValidationError.
    """
    receipt = ProofReceipt(
        id=str(data.get("id", "")),
        agent=str(data.get("agent", "")),
        model=str(data.get("model", "")),
        input=str(data.get("input", "")),
        output=str(data.get("output", "")),
        use_case=str(data.get("use_case", "")),
        proof_hash=str(data.get("proof_hash", "")),
        vk_hash=str(data.get("vk_hash", "")),
        input_hash=str(data.get("input_hash", "")),
        output_hash=str(data.get("output_hash", "")),
        model_hash=str(data.get("model_hash", "")),
        verdict=str(data.get("verdict", "")),
        created_at=str(data.get("created_at", "")),
        raw=dict(data),
    )
    if not receipt.id:
        raise ValueError("receipt missing id")
    # Iterate in alphabetical order for deterministic error paths.
    checks = sorted(
        [
            ("input_hash", receipt.input, receipt.input_hash),
            ("model_hash", receipt.model, receipt.model_hash),
            ("output_hash", receipt.output, receipt.output_hash),
        ]
    )
    for name, plain, supplied in checks:
        if not supplied:
            continue
        expected = hash_field(plain)
        if not _hex_equal(expected, supplied):
            raise ReceiptValidationError(name, expected, supplied)
    return receipt


def verify_receipt_bytes(payload: Union[bytes, bytearray, str]) -> ProofReceipt:
    """Convenience: JSON-parse `payload` then call verify_receipt."""
    if isinstance(payload, (bytes, bytearray)):
        payload = payload.decode("utf-8")
    data = json.loads(payload)
    if not isinstance(data, dict):
        raise ValueError("receipt must be a JSON object")
    return verify_receipt(data)
