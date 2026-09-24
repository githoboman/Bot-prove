import importlib.util, pathlib, unittest
from unittest.mock import patch
P = pathlib.Path(__file__).parents[1] / "scripts" / "judge_demo.py"
spec = importlib.util.spec_from_file_location("judge_demo", P); demo = importlib.util.module_from_spec(spec); import sys; sys.modules[spec.name] = demo; spec.loader.exec_module(demo)

class FakeClient:
    def request(self, url, **kwargs):
        if url.endswith('/health'): return 200, {'status':'ok','version':'1'}
        if url.endswith('/proofs'): return 200, [{'id':'p1'}]
        if url.endswith('/prove'): return 200, {'hash':'h','proof_hex':'p'}
        if url.endswith('/verify'): return 200, {'valid':True}
        return 200, {'result':{}}

class DemoTests(unittest.TestCase):
    def test_contract_checks(self): self.assertTrue(all(x.ok for x in demo.contract_checks(FakeClient())))
    def test_crypto_skips_without_key(self):
        result = demo.crypto_checks(FakeClient(), 'https://api', '')[0]
        self.assertTrue(result.ok); self.assertIn('SKIP', result.detail)
    def test_crypto_roundtrip_with_key(self): self.assertTrue(demo.crypto_checks(FakeClient(), 'https://api', 'secret')[0].ok)
    def test_http_error_is_redacted(self):
        import urllib.error
        text = demo.friendly_error(urllib.error.HTTPError('https://x', 401, 'Unauthorized', {}, None))
        self.assertEqual(text, 'HTTP 401 Unauthorized')

if __name__ == '__main__': unittest.main()
