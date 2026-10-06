from __future__ import annotations

import json
import os
import re
from pathlib import Path
from typing import Any

ALLOWED_TOOLS = ("get_financials", "search_filings", "calculate_metric", "read_document_spans")
FORBIDDEN_TOOL_SUBSTRINGS = ("bash", "shell", "read_file", "write_file", "python", "mcp", "browser")


def assert_exact_tool_allowlist(names: set[str]) -> None:
    if names != set(ALLOWED_TOOLS):
        raise RuntimeError(f"tool allowlist mismatch: {sorted(names)}")


def inspect_middleware_names(middlewares: list[Any]) -> list[str]:
    names = []
    for item in middlewares:
        names.append(type(item).__name__)
    return names


def forbidden_middleware(names: list[str]) -> list[str]:
    bad = []
    joined = " ".join(names).lower()
    for needle in ("memory", "bash", "filesystem", "sandbox", "skill"):
        if needle in joined:
            bad.append(needle)
    return bad


def _extract_json_object(text: str) -> dict[str, Any]:
    stripped = re.sub(r"^```(?:json)?\s*|\s*```$", "", text.strip(), flags=re.IGNORECASE)
    try:
        value = json.loads(stripped)
    except json.JSONDecodeError:
        match = re.search(r"\{.*\}", text, flags=re.DOTALL)
        if not match:
            raise RuntimeError("MODEL_OUTPUT_INVALID")
        try:
            value = json.loads(match.group(0))
        except json.JSONDecodeError as exc:
            raise RuntimeError("MODEL_OUTPUT_INVALID") from exc
    if not isinstance(value, dict):
        raise RuntimeError("MODEL_OUTPUT_INVALID")
    return value


class FinanceDeerFlowAdapter:
    """Wraps the real DeerFlow Harness client. Does not implement a substitute loop."""

    def __init__(self, tools: list[Any] | None = None):
        self._tools = tools or []
        self._client = None

    def import_harness(self):
        from deerflow.client import DeerFlowClient

        return DeerFlowClient

    def build_client(self, token: str, thread_id: str):
        DeerFlowClient = self.import_harness()
        client = DeerFlowClient(
            subagent_enabled=False,
            plan_mode=False,
            thinking_enabled=False,
            available_skills=set(),
        )
        tools = list(self._tools)

        def _get_tools(*, model_name=None, subagent_enabled=False):
            return tools

        client._get_tools = _get_tools  # type: ignore[method-assign]
        if os.environ.get("OPENAI_API_KEY") == token:
            raise RuntimeError("must not copy task token into global env")
        self._client = client
        return client

    def actual_tool_names(self, client) -> set[str]:
        names = {getattr(t, "name", "") for t in self._tools}
        extra = []
        agent = getattr(client, "_agent", None)
        if agent is not None:
            bound = getattr(agent, "tools", None) or getattr(agent, "nodes", None)
            extra = [getattr(t, "name", str(t)) for t in (bound or [])]
        if extra:
            names |= {n for n in extra if n}
        if names and names != set(ALLOWED_TOOLS) and self._tools:
            # Fail closed when harness injects extra tools after assembly.
            if names != set(ALLOWED_TOOLS):
                raise RuntimeError(f"unexpected tools after assembly: {sorted(names)}")
        if self._tools:
            names = {getattr(t, "name", "") for t in self._tools}
            assert_exact_tool_allowlist(set(names))
            return set(names)
        return set(names)

    def complete_research(self, client: Any, task: Any) -> Any:
        """Run the real DeerFlow model loop and validate its structured result."""
        from app.schemas import Argument, ResearchResult, ResultError, Usage

        prompt_path = Path(__file__).resolve().parent / "prompts" / f"{task.role}.md"
        role_prompt = prompt_path.read_text(encoding="utf-8")
        contract = {
            "run_id": task.run_id,
            "task_id": task.task_id,
            "role": task.role,
            "claim": task.claim.model_dump(mode="json"),
            "instrument_id": task.instrument_id,
            "as_of": task.as_of,
            "input_mode": task.input_mode,
            "document_id": task.document_id,
            "deadline_at": task.deadline_at,
        }
        prompt = (
            role_prompt
            + "\n\n只输出一个 JSON 对象，不要输出 Markdown。JSON 字段为 "
            + "status(succeeded|insufficient), arguments, evidence_ids, unknowns, counterevidence, errors。"
            + "arguments/counterevidence 的每项为 {claim_type,text,evidence_ids,verification_status}。"
            + "所有事实和推演必须引用工具返回的 evidence_ids；用户研报使用 reported_only。\n"
            + json.dumps(contract, ensure_ascii=False)
        )
        raw = client.chat(prompt, thread_id=task.task_id)
        if not isinstance(raw, str):
            raise RuntimeError("MODEL_OUTPUT_INVALID")
        payload = _extract_json_object(raw)
        status = payload.get("status")
        if status not in {"succeeded", "insufficient"}:
            raise RuntimeError("MODEL_OUTPUT_INVALID")

        def parse_arguments(key: str) -> list[Argument]:
            rows = payload.get(key) or []
            if not isinstance(rows, list):
                raise RuntimeError("MODEL_OUTPUT_INVALID")
            out = []
            for row in rows:
                if not isinstance(row, dict):
                    raise RuntimeError("MODEL_OUTPUT_INVALID")
                out.append(Argument(
                    claim_type=row.get("claim_type", "fact"),
                    text=str(row.get("text") or ""),
                    evidence_ids=[str(v) for v in (row.get("evidence_ids") or [])],
                    verification_status=row.get("verification_status", "independent_verified"),
                ))
            return out

        errors = []
        for row in payload.get("errors") or []:
            if isinstance(row, dict):
                errors.append(ResultError(
                    code=str(row.get("code") or "MODEL_ERROR"),
                    message=str(row.get("message") or ""),
                    retryable=bool(row.get("retryable", False)),
                ))
        return ResearchResult(
            run_id=task.run_id,
            task_id=task.task_id,
            status=status,
            arguments=parse_arguments("arguments"),
            evidence_ids=[str(v) for v in (payload.get("evidence_ids") or [])],
            unknowns=[str(v) for v in (payload.get("unknowns") or [])],
            counterevidence=parse_arguments("counterevidence"),
            usage=Usage(
                model_calls=1,
                tool_calls=max(0, int(payload.get("tool_calls") or 0)),
                input_tokens=max(0, int(payload.get("input_tokens") or 0)),
                output_tokens=max(0, int(payload.get("output_tokens") or 0)),
                usage_unknown=True,
                simulated=False,
            ),
            errors=errors,
        )

    def complete_tool_roundtrip(self, client, tool_name: str = "get_financials", request: dict[str, Any] | None = None) -> dict[str, Any]:
        """Invoke one harness-bound finance tool and keep the request plus response.

        A successful import without this trace is not a research result.
        """
        if type(client).__name__ != "DeerFlowClient":
            raise RuntimeError("tool roundtrip requires DeerFlowClient")
        names = self.actual_tool_names(client)
        assert_exact_tool_allowlist(names)
        tools = list(client._get_tools())
        tool = next(t for t in tools if getattr(t, "name", "") == tool_name)
        if request is None:
            if tool_name == "read_document_spans":
                request = {"document_id": "doc_unavailable", "query": "", "limit": 5, "span_ids": []}
            else:
                request = {"metrics": ["revenue"], "periods": ["2024-12-31"]}
        response = tool.invoke(request)
        if not isinstance(response, dict):
            raise RuntimeError("tool roundtrip returned no object")
        evidence_ids = [i for i in (response.get("evidence_ids") or []) if i]
        text = ""
        data = response.get("data")
        if isinstance(data, dict):
            text = str(data.get("text") or "")
        self._last_trace = {"tool": tool_name, "request": request, "response": response}
        return {"evidence_ids": evidence_ids, "text": text, "response": response}
