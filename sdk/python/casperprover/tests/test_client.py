"""Tests for the CasperProver Python SDK.

These mirror the Go tests in sdk/primitives_test.go against a stdlib
`http.server` fixture. Run via `python -m unittest casperprover.tests`
from `sdk/python/`.
"""

from __future__ import annotations

import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from typing import Optional

from casperprover import (
    BatchItem,
    Client,
    ProveRequest,
    ReceiptValidationError,
    hash_field,
    verify_receipt_bytes,
)


class _Recording(BaseHTTPRequestHandler):
    reply: bytes = b"{}"
    last_method: str = ""
    last_path: str = ""
    last_headers: dict = {}
    last_body: str = ""

    def _capture(self) -> None:
        length = int(self.headers.get("Content-Length", "0") or 0)
        body = self.rfile.read(length).decode("utf-8") if length else ""
        _Recording.last_method = self.command
        _Recording.last_path = self.path
        _Recording.last_headers = {k.lower(): v for k, v in self.headers.items()}
        _Recording.last_body = body
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(_Recording.reply)

    def do_GET(self) -> None:  # noqa: N802 - stdlib signature
        self._capture()

    def do_POST(self) -> None:  # noqa: N802 - stdlib signature
        self._capture()

    def log_message(self, fmt: str, *args) -> None:  # noqa: D401 - quiet tests
        return


class _Server:
    def __init__(self) -> None:
        self.httpd = HTTPServer(("127.0.0.1", 0), _Recording)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()

    @property
    def url(self) -> str:
        host, port = self.httpd.server_address
        return f"http://{host}:{port}"

    def close(self) -> None:
        self.httpd.shutdown()
        self.httpd.server_close()
        self.thread.join(timeout=1.0)


class SdkTests(unittest.TestCase):
    def setUp(self) -> None:
        self.srv = _Server()
        _Recording.reply = b'{"id":"pf-1","proof_hash":"deadbeef"}'
        _Recording.last_method = ""
        _Recording.last_path = ""
        _Recording.last_headers = {}
        _Recording.last_body = ""

    def tearDown(self) -> None:
        self.srv.close()

    def _client(self, **kw) -> Client:
        return Client(base_url=self.srv.url, **kw)

    def test_prove_uses_v1_prefix(self) -> None:
        got = self._client().prove(ProveRequest(agent="a", model="m",
                                                 input="in", output="out"))
        self.assertEqual(got.id, "pf-1")
        self.assertEqual(_Recording.last_path, "/v1/proofs")
        self.assertEqual(_Recording.last_method, "POST")

    def test_prove_sends_idempotency_key(self) -> None:
        self._client().prove(ProveRequest(agent="a"), idempotency_key="key-42")
        self.assertEqual(_Recording.last_headers.get("x-idempotency-key"),
                         "key-42")

    def test_verify_sends_proof_id(self) -> None:
        _Recording.reply = b'{"valid":true,"proof_id":"pf-9"}'
        got = self._client().verify("pf-9")
        self.assertTrue(got.valid)
        self.assertEqual(got.proof_id, "pf-9")
        self.assertEqual(_Recording.last_path, "/v1/verify")
        self.assertIn('"proof_id": "pf-9"', _Recording.last_body)

    def test_batch_sends_all_items(self) -> None:
        _Recording.reply = b'{"verified":["a","b"],"total":2}'
        got = self._client().batch([BatchItem(proof_id="a"),
                                     BatchItem(proof_id="b")], mode="strict")
        self.assertEqual(got.total, 2)
        self.assertEqual(len(got.verified), 2)
        self.assertEqual(_Recording.last_path, "/v1/batch/verify-zk")
        body = json.loads(_Recording.last_body)
        self.assertEqual(body["mode"], "strict")

    def test_anchor_uses_proof_path(self) -> None:
        _Recording.reply = b'{"proof_id":"pf-x","tx_hash":"aa11","strict_mode":true}'
        got = self._client().anchor("pf-x", idempotency_key="anchor-key")
        self.assertEqual(got.tx_hash, "aa11")
        self.assertTrue(got.strict_mode)
        self.assertEqual(_Recording.last_path, "/v1/proofs/pf-x/anchor")
        self.assertEqual(_Recording.last_headers.get("x-idempotency-key"),
                         "anchor-key")

    def test_unversioned_client_keeps_legacy_path(self) -> None:
        self._client(api_version="").prove(ProveRequest(agent="a"))
        self.assertEqual(_Recording.last_path, "/proofs")

    def test_receipt_validator_happy(self) -> None:
        payload = json.dumps({
            "id": "pf-1",
            "input": "hello",
            "output": "world",
            "model": "gpt-toy-v1",
            "input_hash": hash_field("hello"),
            "output_hash": hash_field("world"),
            "model_hash": hash_field("gpt-toy-v1"),
        }).encode()
        r = verify_receipt_bytes(payload)
        self.assertEqual(r.id, "pf-1")

    def test_receipt_validator_detects_tamper(self) -> None:
        payload = json.dumps({
            "id": "pf-1",
            "input": "hello",
            "input_hash": hash_field("goodbye"),
        }).encode()
        with self.assertRaises(ReceiptValidationError) as ctx:
            verify_receipt_bytes(payload)
        self.assertEqual(ctx.exception.field_name, "input_hash")

    def test_hash_field_matches_go(self) -> None:
        # sha256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
        self.assertEqual(
            hash_field("hello"),
            "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
        )


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
