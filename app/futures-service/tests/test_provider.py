import asyncio
import unittest

from futures_worker.providers.base import FetchRequest, ProviderError
from futures_worker.providers.registry import ProviderRegistry, SourceManifest


class SequenceProvider:
    def __init__(self, pages):
        self.pages = pages
        self.calls = 0

    async def fetch(self, request):
        page = self.pages[self.calls]
        self.calls += 1
        if isinstance(page, BaseException):
            raise page
        return page


class ProviderTests(unittest.TestCase):
    @staticmethod
    def record(record_id):
        return {
            "id": record_id, "revision": 1, "supersedes_id": None, "scope": "public", "owner_id": None, "mode": None,
            "contract_id": None, "source_cluster": "cluster-v1", "caliber_id": "standard", "value": "100",
            "unit": "tonne", "currency": None, "period_start": "2026-10-01", "period_end": "2026-10-01",
            "trading_day": "2026-10-01", "published_at": "2026-10-01T08:00:00Z", "published_precision": "date",
            "version_available_at": "2026-10-01T08:00:00Z", "first_observed_at": "2026-10-01T08:00:00Z",
            "retrieved_at": "2026-10-01T08:00:00Z", "usable_at": "2026-10-01T08:00:00Z",
            "quality": {"acquisition": "available", "freshness": "current", "verification": "source_recorded"},
            "locator": {"source_url": "https://source.example/data", "page": None, "row_key": record_id, "quote": None},
            "content_hash": f"hash-{record_id}",
        }

    def test_partial_pages_do_not_advance_complete_watermark(self):
        request = FetchRequest("licensed", "SHFE.CU", "inventory", "2026-01-01", "2026-10-05", None)
        provider = SequenceProvider([
            {"records": [{"id": "r1"}], "next_cursor": "2", "complete": False, "coverage_start": "2026-01-01", "coverage_end": "2026-02-01"},
            TimeoutError("timeout"),
        ])
        registry = ProviderRegistry({"test": provider})
        result = asyncio.run(registry.fetch_manifested("test", request, expected_manifest="source-v1"))
        self.assertFalse(result["complete"])
        self.assertIsNone(result["watermark"])
        self.assertEqual(result["coverage_end"], "2026-02-01")

    def test_live_registry_rejects_fixture_source_type(self):
        manifest = SourceManifest("s1", "v1", "publisher", "https://example.invalid", "fixture", {"export": True}, ("inventory",))
        with self.assertRaises(ProviderError):
            ProviderRegistry.validate_live_manifest(manifest)

    def test_complete_pages_advance_watermark_only_after_all_validations(self):
        request = FetchRequest("licensed", "SHFE.CU", "inventory", "2026-01-01", "2026-10-05", None)
        provider = SequenceProvider([
            {"records": [self.record("r1")], "next_cursor": "2", "complete": False, "coverage_start": "2026-01-01", "coverage_end": "2026-02-01"},
            {"records": [self.record("r2")], "next_cursor": None, "complete": True, "coverage_start": "2026-01-01", "coverage_end": "2026-10-05"},
        ])
        manifest = SourceManifest("test", "1", "publisher", "https://source.example/data", "industry_data", {"display": True, "model_use": True, "retain_until": "2030-01-01T00:00:00Z"}, ("inventory",))
        ingested = []
        result = asyncio.run(ProviderRegistry({"test": provider}).fetch_and_ingest(
            "test", request, manifest, lambda records: ingested.extend(records)
        ))
        self.assertTrue(result["complete"])
        self.assertEqual(result["watermark"], "2026-10-05")
        self.assertEqual([record["id"] for record in ingested], ["r1", "r2"])
        self.assertEqual(ingested[0]["source_id"], "test")
        self.assertEqual(ingested[0]["source_version"], "1")

    def test_partial_fetch_never_ingests_or_advances_watermark(self):
        provider = SequenceProvider([
            {"records": [self.record("r1")], "next_cursor": "2", "complete": False, "coverage_start": "2026-01-01", "coverage_end": "2026-02-01"},
            TimeoutError("timeout"),
        ])
        manifest = SourceManifest("test", "1", "publisher", "https://source.example/data", "industry_data", {"display": True, "model_use": True, "retain_until": "2030-01-01T00:00:00Z"}, ("inventory",))
        ingested = []
        result = asyncio.run(ProviderRegistry({"test": provider}).fetch_and_ingest(
            "test", FetchRequest("licensed", "SHFE.CU", "inventory", "2026-01-01", "2026-10-05", None),
            manifest, lambda records: ingested.extend(records)
        ))
        self.assertFalse(result["complete"])
        self.assertIsNone(result["watermark"])
        self.assertEqual(ingested, [])
