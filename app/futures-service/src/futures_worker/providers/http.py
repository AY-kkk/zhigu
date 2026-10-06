from typing import Any, Mapping

import httpx

from .base import FetchRequest, ProviderError, SourceManifest


class HTTPProvider:
    """Fetches only the URL frozen in a validated source manifest."""

    def __init__(self, manifest: SourceManifest, client: httpx.AsyncClient | None = None):
        self.manifest = manifest
        self.client = client or httpx.AsyncClient(timeout=httpx.Timeout(15))

    async def fetch(self, request: FetchRequest) -> Mapping[str, Any]:
        if request.scope != "licensed":
            raise ProviderError("FUTURES_PROVIDER_SCOPE_DENIED")
        if request.metric not in self.manifest.metrics:
            raise ProviderError("FUTURES_PROVIDER_METRIC_DENIED")
        params = {
            "product_id": request.product_id, "metric": request.metric,
            "start": request.start, "end": request.end,
        }
        if request.cursor:
            params["cursor"] = request.cursor
        response = await self.client.get(self.manifest.url, params=params)
        if response.status_code != 200:
            raise ProviderError("FUTURES_PROVIDER_HTTP_ERROR")
        payload = response.json()
        records = payload.get("records")
        if not isinstance(records, list):
            raise ProviderError("FUTURES_PROVIDER_PAGE_INVALID")
        for record in records:
            if not isinstance(record, dict) or not record.get("id"):
                raise ProviderError("FUTURES_PROVIDER_RECORD_INVALID")
            if record.get("source_type") == "fixture":
                raise ProviderError("FUTURES_FIXTURE_SOURCE_IN_LIVE")
        return {
            "records": records,
            "next_cursor": payload.get("next_cursor"),
            "complete": payload.get("complete") is True,
            "coverage_start": payload.get("coverage_start"),
            "coverage_end": payload.get("coverage_end"),
        }
