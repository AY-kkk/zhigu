from __future__ import annotations

from typing import Any

from app.tools.provider import GoClient


def search_filings(client: GoClient, query: str, limit: int = 5) -> dict[str, Any]:
    if limit > 5:
        limit = 5
    args = {"query": query, "limit": limit}
    grant_id = client.grant("search_filings", args)
    payload = client.data_query(grant_id, "search_filings", args)
    ids = client.register_evidence(grant_id, payload)
    client.complete(grant_id, "succeeded", payload.get("content_hash", ""))
    return {
        "data": payload,
        "evidence_ids": ids,
        "as_of": payload.get("available_at"),
        "data_version": payload.get("data_version"),
        "quality_status": "insufficient",
        "warnings": ["fixture"],
    }
