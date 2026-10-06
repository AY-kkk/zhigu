from __future__ import annotations

import hashlib
import json
import re
from typing import Any

from app.tools.provider import GoClient

_SPAN_ID = re.compile(r"span_[0-9a-f-]+")


def read_document_spans(
    client: GoClient,
    document_id: str,
    query: str = "",
    limit: int = 5,
    span_ids: list[str] | None = None,
) -> dict[str, Any]:
    if not document_id:
        return {
            "spans": [],
            "evidence_ids": [],
            "verification_status": "reported_only",
            "unknowns": ["正文不足：缺少研报 document_id。"],
        }
    limit = max(1, min(int(limit), 20))
    args = {
        "document_id": document_id,
        "query": query or "",
        "limit": limit,
        "span_ids": span_ids or [],
    }
    grant_id = client.grant("read_document_spans", args)
    try:
        result = client.data_query(grant_id, "read_document_spans", args)
        record_ids = [row["record_id"] for row in result.get("records", []) if row.get("record_id")]
        evidence_ids = client.register_evidence(grant_id, record_ids)
        spans = []
        for row in result.get("records", []):
            record = row.get("record") or {}
            locator = str(record.get("locator") or "")
            match = _SPAN_ID.search(locator)
            spans.append(
                {
                    "span_id": match.group(0) if match else row.get("record_id", ""),
                    "title": record.get("title", ""),
                    "locator": locator,
                    "text": record.get("text", ""),
                    "verification_status": "reported_only",
                }
            )
        output = {
            "spans": spans,
            "evidence_ids": evidence_ids,
            "verification_status": "reported_only",
            "unknowns": [],
        }
        if not spans:
            output["unknowns"] = ["正文不足：未找到匹配的研报段落。"]
        return output
    finally:
        output_hash = hashlib.sha256(
            json.dumps(args, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()
        ).hexdigest()
        client.complete(grant_id, "succeeded", output_hash)
