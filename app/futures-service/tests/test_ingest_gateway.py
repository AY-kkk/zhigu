import asyncio
import json
import unittest

import httpx

from futures_worker.providers.base import ProviderError
from futures_worker.providers.ingest import HTTPIngestGateway


class IngestGatewayTests(unittest.TestCase):
    def test_complete_records_are_posted_to_the_go_ingestion_port(self):
        calls = []

        def handler(request):
            calls.append((str(request.url), request.headers.get("x-futures-service-token"), json.loads(request.content)))
            return httpx.Response(200, json={"data": {"inserted_ids": ["r1"], "evaluated_conditions": 1}})

        async def run():
            async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
                gateway = HTTPIngestGateway("http://offline.invalid", "service-token", 7, client=client)
                return await gateway.ingest([{"id": "r1", "scope": "public"}])

        result = asyncio.run(run())
        self.assertEqual(result["inserted_ids"], ["r1"])
        self.assertEqual(calls[0][0], "http://offline.invalid/internal/futures/v1/observations")
        self.assertEqual(calls[0][1], "service-token")
        self.assertEqual(calls[0][2]["owner_id"], 7)
        self.assertEqual(calls[0][2]["records"][0]["id"], "r1")

    def test_ingest_rejection_is_not_treated_as_success(self):
        async def run():
            async with httpx.AsyncClient(transport=httpx.MockTransport(lambda request: httpx.Response(403))) as client:
                await HTTPIngestGateway("http://offline.invalid", "service-token", 7, client=client).ingest([])

        with self.assertRaises(ProviderError):
            asyncio.run(run())
