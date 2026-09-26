from __future__ import annotations

import os
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
