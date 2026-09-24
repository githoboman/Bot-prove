"""CasperProver Python SDK.

Public API::

    from casperprover import Client, ProveRequest, VerifyReceipt

    c = Client(base_url="https://casperprover-api-ylsh.onrender.com",
               api_key="pk_...")
    proof = c.prove(ProveRequest(agent="a", model="m", input="hi", output="ok"))
    ok = c.verify(proof.id).valid

Feature parity with the Go SDK: `Prove`, `Verify`, `Batch`, `Anchor`,
`VerifyReceipt`, plus per-request idempotency-key + custom headers.
"""

from .client import (
    Client,
    ApiError,
    ProveRequest,
    ProveResponse,
    VerifyResponse,
    BatchItem,
    BatchResponse,
    AnchorResponse,
)
from .receipt import (
    ProofReceipt,
    ReceiptValidationError,
    verify_receipt,
    verify_receipt_bytes,
    hash_field,
)

__all__ = [
    "Client",
    "ApiError",
    "ProveRequest",
    "ProveResponse",
    "VerifyResponse",
    "BatchItem",
    "BatchResponse",
    "AnchorResponse",
    "ProofReceipt",
    "ReceiptValidationError",
    "verify_receipt",
    "verify_receipt_bytes",
    "hash_field",
]

__version__ = "0.1.2"
