import asyncio
import unittest

import httpx

from futures_worker.providers.base import FetchRequest, ProviderError, SourceManifest
from futures_worker.providers.http import HTTPProvider


class HTTPProviderTests(unittest.TestCase):
    def test_manifest_url_is_authoritative_and_metrics_are_whitelisted(self):
        calls = []
        def handler(request):
            calls.append(str(request.url))
            return httpx.Response(200, json={"records": [{"id": "r1", "source_type": "exchange"}], "next_cursor": None, "complete": True, "coverage_start": "2026-01-01", "coverage_end": "2026-01-02"})
        manifest = SourceManifest("s1", "v1", "publisher", "https://source.example/data", "exchange", {"display": True, "model_use": True}, ("inventory",))
        provider = HTTPProvider(manifest, httpx.AsyncClient(transport=httpx.MockTransport(handler)))
        result = asyncio.run(provider.fetch(FetchRequest("licensed", "SHFE.CU", "inventory", "2026-01-01", "2026-01-02", None)))
        self.assertTrue(result["complete"])
        self.assertEqual(calls, ["https://source.example/data?product_id=SHFE.CU&metric=inventory&start=2026-01-01&end=2026-01-02"])
        with self.assertRaises(ProviderError):
            asyncio.run(provider.fetch(FetchRequest("licensed", "SHFE.CU", "unapproved", "2026-01-01", "2026-01-02", None)))
