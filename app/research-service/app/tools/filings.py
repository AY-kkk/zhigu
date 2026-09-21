from __future__ import annotations

from typing import Any

from app.tools.provider import GoClient


def search_filings(client: GoClient, query: str, limit: int = 5) -> dict[str, Any]:
    if limit > 5:
        limit = 5
    args = {"query": query, "limit": limit}
    grant_id = client.grant("search_filings", args)
    payload = client.data_query(grant_id, "search_filings", args)
    records = payload.get("records") or []
    ids = client.register_evidence(grant_id, [r["record_id"] for r in records if r.get("record_id")])
    first = records[0].get("record") if records else {}
    client.complete(grant_id, "succeeded", records[0].get("record_hash", "") if records else "")
    return {
        "data": first,
        "evidence_ids": ids,
        "as_of": first.get("available_at") if isinstance(first, dict) else None,
        "data_version": first.get("data_version") if isinstance(first, dict) else None,
        "quality_status": payload.get("quality_status") or "insufficient",
        "warnings": payload.get("warnings") or [],
        "unknowns": [] if records else ["正文不足：公告检索未返回可定位原文。"],
    }
