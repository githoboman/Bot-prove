"""Tests for scripts/verify_proof_by_id.py.

The script is std-lib only and does live HTTP by default; the tests
stub urllib.request.urlopen so no network is touched.
"""

from __future__ import annotations

import io
import json
import sys
import types
import urllib.error
import urllib.request
from pathlib import Path
from unittest.mock import patch

import pytest


# The script lives under scripts/ (not a package). Load it as a module.
def _load_script():
    import importlib.util

    root = Path(__file__).resolve().parent.parent
    script_path = root / "scripts" / "verify_proof_by_id.py"
    spec = importlib.util.spec_from_file_location("cp_verify_proof_by_id", script_path)
    mod = importlib.util.module_from_spec(spec)
    # Register in sys.modules BEFORE exec so @dataclass can resolve
    # cls.__module__ back to the live module dict.
    sys.modules["cp_verify_proof_by_id"] = mod
    spec.loader.exec_module(mod)  # type: ignore[union-attr]
    return mod


@pytest.fixture(scope="module")
def script():
    return _load_script()


class _FakeResp:
    def __init__(self, status: int, body: dict):
        self.status = status
        self._body = json.dumps(body).encode()

    def read(self) -> bytes:
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


def _make_urlopen(cases: dict):
    """cases: {url_substr: (status, body_dict)}.

    First matching key wins.
    """
    def fake(req, timeout=None):
        url = req.full_url
        for k, v in cases.items():
            if k in url:
                if isinstance(v, Exception):
                    raise v
                return _FakeResp(*v)
        raise urllib.error.URLError(f"no stub for URL {url}")

    return fake


class TestStages:
    def test_health_pass(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        with patch("urllib.request.urlopen", _make_urlopen({"/health": (200, {"status": "ok", "version": "1.0"})})):
            script.stage_01_health("http://x", log)
        assert log.stages[0].name == "api_health"
        assert log.stages[0].status == "PASS"
        assert "version=1.0" in log.stages[0].detail

    def test_health_fail_on_network_error(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        with patch("urllib.request.urlopen", _make_urlopen({"/health": urllib.error.URLError("dns")})):
            script.stage_01_health("http://x", log)
        assert log.stages[0].status == "FAIL"
        assert "network error" in log.stages[0].detail

    def test_fetch_proof_pass(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        with patch(
            "urllib.request.urlopen",
            _make_urlopen({"/proofs/p1": (200, {"PH": "abcdef1234567890", "Root": "rootABC", "agent": "a1"})}),
        ):
            proof = script.stage_02_fetch_proof("http://x", "p1", log)
        assert proof is not None
        assert log.stages[0].status == "PASS"
        assert "agent=a1" in log.stages[0].detail

    def test_merkle_shape_pass(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        script.stage_03_merkle_consistency({"Root": "r", "Path": ["h1", "h2", "h3"], "Idx": 2}, log)
        assert log.stages[0].status == "PASS"
        assert "depth=3" in log.stages[0].detail

    def test_merkle_shape_fail_when_idx_too_large(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        # depth=2 → covers idx 0..3. idx=99 must fail.
        script.stage_03_merkle_consistency({"Root": "r", "Path": ["a", "b"], "Idx": 99}, log)
        assert log.stages[0].status == "FAIL"

    def test_merkle_shape_skip_when_no_proof(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        script.stage_03_merkle_consistency(None, log)
        assert log.stages[0].status == "SKIP"

    def test_on_chain_registry_pass(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        with patch(
            "urllib.request.urlopen",
            _make_urlopen({"casper.network/rpc": (200, {"jsonrpc": "2.0", "id": 1, "result": {"api_version": "2.0"}})}),
        ):
            script.stage_04_on_chain_registry(log)
        assert log.stages[0].status == "PASS"
        assert "queryable" in log.stages[0].detail

    def test_full_verify_skip_without_inputs(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        script.stage_05_full_verify("http://x", "p1", b"", b"", "", log)
        assert log.stages[0].status == "SKIP"

    def test_full_verify_pass_when_server_verified(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        server_response = {
            "verified": True,
            "checks": {
                "input_hash_match": True,
                "output_hash_match": True,
                "model_hash_match": True,
                "commit_valid": True,
                "merkle_valid": True,
            },
        }
        with patch("urllib.request.urlopen", _make_urlopen({"/verify": (200, server_response)})):
            script.stage_05_full_verify("http://x", "p1", b"i", b"o", "m", log)
        assert log.stages[0].status == "PASS"
        assert "merkle_valid=True" in log.stages[0].detail

    def test_full_verify_fail_when_hash_mismatch(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        server_response = {
            "verified": False,
            "error": "input hash mismatch",
            "checks": {
                "input_hash_match": False,
                "output_hash_match": True,
                "model_hash_match": True,
                "commit_valid": False,
                "merkle_valid": True,
            },
        }
        with patch("urllib.request.urlopen", _make_urlopen({"/verify": (200, server_response)})):
            script.stage_05_full_verify("http://x", "p1", b"i", b"o", "m", log)
        assert log.stages[0].status == "FAIL"

    def test_signature_shape_skip_when_unsigned(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        script.stage_06_signature({"PH": "abc"}, log)
        assert log.stages[0].status == "SKIP"

    def test_signature_shape_pass_when_both_fields_set(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        script.stage_06_signature({"PubKey": "01aabbcc", "Sig": "deadbeef"}, log)
        assert log.stages[0].status == "PASS"

    def test_signature_shape_fail_when_asymmetric(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        script.stage_06_signature({"PubKey": "01aabbcc"}, log)  # sig missing
        assert log.stages[0].status == "FAIL"


class TestRendering:
    def test_render_includes_all_stages(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        log.add(script.StageResult(1, "api_health", "PASS", "ok", 10))
        log.add(script.StageResult(2, "fetch_proof", "FAIL", "boom", 20))
        rendered = script.render_log(log)
        assert "proof_id: p1" in rendered
        assert "api_health" in rendered
        assert "fetch_proof" in rendered
        assert "OVERALL:  FAIL" in rendered

    def test_overall_pass_when_no_fail(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        log.add(script.StageResult(1, "s", "PASS", "d", 0))
        log.add(script.StageResult(2, "s2", "SKIP", "d", 0))
        assert log.overall() == "PASS"

    def test_overall_skip_when_all_skipped(self, script):
        log = script.RunLog(proof_id="p1", started_at="now", api="http://x")
        log.add(script.StageResult(1, "s", "SKIP", "d", 0))
        assert log.overall() == "SKIP"
