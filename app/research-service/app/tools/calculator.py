from __future__ import annotations

from decimal import Decimal, InvalidOperation, ROUND_HALF_UP
from typing import Any

from app.tools.provider import GoClient

_KEYS = {
    "growth_rate": ("current", "previous"),
    "ratio": ("numerator", "denominator"),
    "difference": ("left", "right"),
}


def calculate_metric(client: GoClient | None, operation: str, inputs: dict[str, Any] | list[dict[str, Any]]) -> dict[str, Any]:
    if operation not in _KEYS:
        raise ValueError("INVALID_OPERATION")
    named = _as_named(operation, inputs)
    if client and client.enabled():
        args = {"operation": operation, "inputs": named}
        grant_id = client.grant("calculate_metric", args)
        out = client.calculate(grant_id, operation, named)
        client.complete(grant_id, "succeeded", str(out.get("value") or out.get("result") or ""))
        return {
            "data": out,
            "evidence_ids": out.get("evidence_ids") or [],
            "as_of": None,
            "data_version": "calc_v1",
            "quality_status": "verified",
            "warnings": [],
        }
    left_key, right_key = _KEYS[operation]
    left = Decimal(str(named[left_key].get("value")))
    right = Decimal(str(named[right_key].get("value")))
    try:
        if operation == "growth_rate":
            if right <= 0:
                return {"error": "INSUFFICIENT_DENOMINATOR"}
            result = ((left - right) / right * Decimal("100")).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)
            return {"data": {"value": str(result), "unit": "percent", "formula": "(current-previous)/previous×100"}, "evidence_ids": [], "quality_status": "verified", "warnings": []}
        if operation == "ratio":
            if right == 0:
                return {"error": "INSUFFICIENT_DENOMINATOR"}
            result = left / right
        else:
            result = left - right
    except (InvalidOperation, ZeroDivisionError):
        return {"error": "INSUFFICIENT_DENOMINATOR"}
    return {"data": {"result": str(result), "value": str(result)}, "evidence_ids": [], "quality_status": "verified", "warnings": []}


def _as_named(operation: str, inputs: dict[str, Any] | list[dict[str, Any]]) -> dict[str, Any]:
    left_key, right_key = _KEYS[operation]
    if isinstance(inputs, dict):
        if left_key not in inputs or right_key not in inputs or len(inputs) != 2:
            raise ValueError("INVALID_INPUTS")
        return inputs
    raise ValueError("INVALID_INPUTS")
