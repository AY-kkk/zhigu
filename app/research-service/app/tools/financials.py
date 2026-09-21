from __future__ import annotations

from typing import Any

from app.tools.provider import GoClient


def get_financials(client: GoClient, metrics: list[str], periods: list[str]) -> dict[str, Any]:
    args = {"metrics": metrics, "periods": periods}
    grant_id = client.grant("get_financials", args)
    payload = client.data_query(grant_id, "get_financials", args)
    records = payload.get("records") or []
    ids = client.register_evidence(grant_id, [r["record_id"] for r in records if r.get("record_id")])
    first = records[0].get("record") if records else {}
    client.complete(grant_id, "succeeded", records[0].get("record_hash", "") if records else "")
    return {
        "data": first,
        "evidence_ids": ids,
        "as_of": first.get("available_at") if isinstance(first, dict) else None,
        "data_version": first.get("data_version") if isinstance(first, dict) else None,
        "quality_status": payload.get("quality_status") or "verified",
        "warnings": payload.get("warnings") or [],
        "unknowns": [] if records else ["无法取数：财务查询未返回已披露记录。"],
    }
