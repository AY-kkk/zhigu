from __future__ import annotations

from typing import Any

from app.tools.provider import GoClient


def get_financials(client: GoClient, metrics: list[str], periods: list[str]) -> dict[str, Any]:
    args = {"metrics": metrics, "periods": periods}
    grant_id = client.grant("get_financials", args)
    payload = client.data_query(grant_id, "get_financials", args)
    ids = client.register_evidence(grant_id, payload)
    client.complete(grant_id, "succeeded", payload.get("content_hash", ""))
    return {
        "data": payload,
        "evidence_ids": ids,
        "as_of": payload.get("available_at"),
        "data_version": payload.get("data_version"),
        "quality_status": "verified",
        "warnings": ["fixture"],
    }
