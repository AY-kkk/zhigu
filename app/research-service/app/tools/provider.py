from __future__ import annotations

import hashlib
import json
import os
import uuid
from typing import Any

import httpx


class GoClient:
    def __init__(self, run_id: str, task_id: str, task_token: str = ""):
        self.base = os.environ.get("ZHIGU_GO_INTERNAL_URL", "").rstrip("/")
        self.service_token = os.environ.get("ZHIGU_INTERNAL_TOKEN", "zhigu-internal-dev")
        self.run_id = run_id
        self.task_id = task_id
        self.task_token = task_token

    def enabled(self) -> bool:
        return bool(self.base)

    def _headers(self) -> dict[str, str]:
        h = {
            "Authorization": f"Bearer {self.service_token}",
            "Content-Type": "application/json",
            "X-Request-ID": str(uuid.uuid4()),
        }
        if self.task_token:
            h["X-Zhigu-Task-Token"] = self.task_token
        return h

    def unwrap(self, res: httpx.Response) -> Any:
        res.raise_for_status()
        body = res.json()
        if isinstance(body, dict) and body.get("error"):
            raise RuntimeError(body["error"].get("code", "GO_ERROR"))
        if isinstance(body, dict) and "data" in body:
            return body["data"]
        return body

    def args_hash(self, payload: dict[str, Any]) -> str:
        return hashlib.sha256(
            json.dumps(payload, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()
        ).hexdigest()

    def grant(self, tool_name: str, args: dict[str, Any]) -> str:
        req_id = str(uuid.uuid4())
        data = self.unwrap(
            httpx.post(
                f"{self.base}/internal/finance/tool-grants",
                headers=self._headers(),
                json={
                    "request_id": req_id,
                    "tool_name": tool_name,
                    "args_hash": self.args_hash(args),
                    "run_id": self.run_id,
                    "task_id": self.task_id,
                },
                timeout=15,
            )
        )
        return data["grant_id"]

    def data_query(self, grant_id: str, operation: str, params: dict[str, Any]) -> dict[str, Any]:
        return self.unwrap(
            httpx.post(
                f"{self.base}/internal/finance/data-query",
                headers=self._headers(),
                json={"grant_id": grant_id, "operation": operation, "params": params},
                timeout=15,
            )
        )

    def register_evidence(self, grant_id: str, record: dict[str, Any]) -> list[str]:
        data = self.unwrap(
            httpx.post(
                f"{self.base}/internal/finance/evidence",
                headers=self._headers(),
                json={"grant_id": grant_id, "records": [record]},
                timeout=15,
            )
        )
        return data.get("evidence_ids") or []

    def complete(self, grant_id: str, status: str, output_hash: str) -> None:
        self.unwrap(
            httpx.post(
                f"{self.base}/internal/finance/tool-grants/{grant_id}/complete",
                headers=self._headers(),
                json={"status": status, "output_hash": output_hash},
                timeout=15,
            )
        )

    def calculate(self, grant_id: str, operation: str, inputs: list[dict[str, Any]]) -> dict[str, Any]:
        return self.unwrap(
            httpx.post(
                f"{self.base}/internal/finance/calculate",
                headers=self._headers(),
                json={"grant_id": grant_id, "operation": operation, "inputs": inputs},
                timeout=15,
            )
        )
