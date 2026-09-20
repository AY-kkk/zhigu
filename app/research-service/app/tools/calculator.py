from __future__ import annotations

from decimal import Decimal, DivisionByZero, InvalidOperation
from typing import Any

from app.tools.provider import GoClient


def calculate_metric(client: GoClient | None, operation: str, inputs: list[dict[str, Any]]) -> dict[str, Any]:
    if operation not in {"growth_rate", "ratio", "difference"}:
        raise ValueError("INVALID_OPERATION")
    if client and client.enabled():
        args = {"operation": operation, "inputs": inputs}
        grant_id = client.grant("calculate_metric", args)
        out = client.calculate(grant_id, operation, inputs)
        client.complete(grant_id, "succeeded", str(out.get("result", "")))
        return {
            "data": out,
            "evidence_ids": out.get("evidence_ids") or [],
            "as_of": None,
            "data_version": "calc_v1",
            "quality_status": "verified",
            "warnings": [],
        }
    if len(inputs) < 2:
        raise ValueError("INVALID_INPUTS")
    left = Decimal(str(inputs[0]["value"]))
    right = Decimal(str(inputs[1]["value"]))
    try:
        if operation == "growth_rate":
            if right == 0:
                return {"error": "INSUFFICIENT_DENOMINATOR"}
            result = (left - right) / right
        elif operation == "ratio":
            if right == 0:
                return {"error": "INSUFFICIENT_DENOMINATOR"}
            result = left / right
        else:
            result = left - right
    except (DivisionByZero, InvalidOperation):
        return {"error": "INSUFFICIENT_DENOMINATOR"}
    return {"data": {"result": str(result)}, "evidence_ids": [], "quality_status": "verified", "warnings": []}
