import copy
import inspect
from typing import Any, Awaitable, Callable, Mapping

from .base import FetchRequest, Provider, ProviderError, SourceManifest

RECORD_FIELDS = {
    "schema_version", "id", "revision", "supersedes_id", "scope", "owner_id", "mode",
    "product_id", "contract_id", "source_id", "source_version", "source_cluster",
    "source_type", "metric", "value", "unit", "currency", "caliber_id", "period_start",
    "period_end", "trading_day", "published_at", "published_precision",
    "version_available_at", "first_observed_at", "retrieved_at", "usable_at", "quality",
    "rights", "locator", "content_hash",
}


class ProviderRegistry:
    def __init__(self, providers: dict[str, Provider]):
        self.providers = dict(providers)

    @staticmethod
    def validate_live_manifest(manifest: SourceManifest) -> None:
        if manifest.source_type == "fixture":
            raise ProviderError("FUTURES_FIXTURE_SOURCE_IN_LIVE")
        if not manifest.publisher or not manifest.url.startswith("https://"):
            raise ProviderError("FUTURES_SOURCE_MANIFEST_INVALID")
        if not manifest.version.isdigit():
            raise ProviderError("FUTURES_SOURCE_MANIFEST_INVALID")
        if not manifest.rights.get("model_use") or not manifest.rights.get("display") or not manifest.rights.get("retain_until"):
            raise ProviderError("FUTURES_SOURCE_RIGHTS_MISSING")
        if not manifest.metrics:
            raise ProviderError("FUTURES_SOURCE_METRICS_MISSING")

    @staticmethod
    def normalize_record(record: Mapping[str, Any], request: FetchRequest, manifest: SourceManifest) -> dict[str, Any]:
        if not isinstance(record, dict):
            raise ProviderError("FUTURES_PROVIDER_RECORD_INVALID")
        missing = RECORD_FIELDS - set(record) - {
            "schema_version", "product_id", "source_id", "source_version", "source_type", "metric", "rights",
        }
        if missing:
            raise ProviderError("FUTURES_PROVIDER_RECORD_INVALID")
        value = copy.deepcopy(record)
        value.update({
            "schema_version": "futures.record.v1",
            "product_id": request.product_id,
            "source_id": manifest.source_id,
            "source_version": manifest.version,
            "source_type": manifest.source_type,
            "metric": request.metric,
            "rights": copy.deepcopy(dict(manifest.rights)),
        })
        if set(value) != RECORD_FIELDS:
            raise ProviderError("FUTURES_PROVIDER_RECORD_INVALID")
        if value.get("scope") not in {"public", "private"}:
            raise ProviderError("FUTURES_PROVIDER_SCOPE_DENIED")
        if value["scope"] == "private" and (not value.get("owner_id") or not value.get("mode")):
            raise ProviderError("FUTURES_PROVIDER_SCOPE_DENIED")
        return value

    async def fetch_manifested(self, source_id: str, request: FetchRequest, expected_manifest: str) -> dict:
        provider = self.providers.get(source_id)
        if provider is None:
            raise ProviderError("FUTURES_PROVIDER_NOT_REGISTERED")
        records = []
        next_cursor = request.cursor
        coverage_start = None
        coverage_end = None
        try:
            while True:
                page = await provider.fetch(FetchRequest(
                    request.scope, request.product_id, request.metric,
                    request.start, request.end, next_cursor,
                ))
                page_records = page.get("records")
                if not isinstance(page_records, list):
                    raise ProviderError("FUTURES_PROVIDER_PAGE_INVALID")
                for record in page_records:
                    if not isinstance(record, dict) or not record.get("id"):
                        raise ProviderError("FUTURES_PROVIDER_RECORD_INVALID")
                records.extend(page_records)
                page_start = page.get("coverage_start")
                page_end = page.get("coverage_end")
                if page_start and (coverage_start is None or page_start < coverage_start):
                    coverage_start = page_start
                if page_end and (coverage_end is None or page_end > coverage_end):
                    coverage_end = page_end
                next_cursor = page.get("next_cursor")
                if page.get("complete") is True:
                    if next_cursor is not None:
                        raise ProviderError("FUTURES_PROVIDER_COMPLETE_WITH_CURSOR")
                    if coverage_start is None or coverage_end is None:
                        raise ProviderError("FUTURES_PROVIDER_COVERAGE_MISSING")
                    return {
                        "manifest_id": expected_manifest,
                        "records": records,
                        "next_cursor": None,
                        "complete": True,
                        "coverage_start": coverage_start,
                        "coverage_end": coverage_end,
                        "watermark": coverage_end,
                    }
                if next_cursor is None:
                    raise ProviderError("FUTURES_PROVIDER_INCOMPLETE_WITHOUT_CURSOR")
        except Exception as exc:
            return {
                "manifest_id": expected_manifest,
                "records": records,
                "next_cursor": next_cursor,
                "complete": False,
                "coverage_start": coverage_start,
                "coverage_end": coverage_end,
                "watermark": None,
                "error": type(exc).__name__,
            }

    async def fetch_and_ingest(
        self,
        source_id: str,
        request: FetchRequest,
        manifest: SourceManifest,
        ingest: Callable[[list[dict[str, Any]]], Awaitable[Any] | Any],
    ) -> dict:
        self.validate_live_manifest(manifest)
        result = await self.fetch_manifested(source_id, request, manifest.version)
        if not result.get("complete") or result.get("watermark") is None:
            return result
        normalized = [self.normalize_record(record, request, manifest) for record in result["records"]]
        ingested = ingest(normalized)
        if inspect.isawaitable(ingested):
            await ingested
        result["records"] = normalized
        return result
