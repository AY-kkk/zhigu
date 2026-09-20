from __future__ import annotations

import os
from typing import Any

ALLOWED_TOOLS = ("get_financials", "search_filings", "calculate_metric")
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
